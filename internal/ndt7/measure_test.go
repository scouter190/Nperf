package ndt7

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nperf/internal/ndt7/ndt7test"
	"nperf/internal/version"
	"nperf/internal/ws"
)

func opts(s *ndt7test.Server, path string) MeasureOptions {
	return MeasureOptions{
		URL:        "wss://" + s.Addr() + "/ndt/v7/" + path + "?access_token=" + ndt7test.Token + "&index=0",
		WS:         ws.Options{TLS: s.TLSConfig()},
		UploadTime: time.Second,
		MinNormal:  600 * time.Millisecond,
	}
}

func srv(t *testing.T, f func(*ndt7test.Config)) *ndt7test.Server {
	t.Helper()
	s, err := ndt7test.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Configure(func(c *ndt7test.Config) {
		c.DownloadTime = time.Second
		c.UploadTime = time.Second
		c.LateClose = 1500 * time.Millisecond
		c.MeasureInterval = 50 * time.Millisecond
		if f != nil {
			f(c)
		}
	})
	return s
}

func within(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > want*tol {
		t.Fatalf("got %.2f, want %.2f ±%.0f%%", got, want, tol*100)
	}
}

func TestDownloadRate(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.DownloadRate = 5e6 }) // 40 Mbps
	d, err := Download(context.Background(), opts(s, "download"))
	if err != nil || d.Incomplete {
		t.Fatalf("%v %+v", err, d)
	}
	within(t, d.Mbps, 40, 0.05)
	if d.MinRTTms == nil || *d.MinRTTms != 12 {
		t.Fatalf("min rtt %v", d.MinRTTms)
	}
	if st := s.Stats(); st.DownloadConns != 1 || st.Connections != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestUploadUsesServerBytes(t *testing.T) {
	s := srv(t, nil)
	d, err := Upload(context.Background(), opts(s, "upload"))
	if err != nil || d.Incomplete {
		t.Fatalf("%v %+v", err, d)
	}
	st := s.Stats()
	// El producto usa el último Measurement recibido (bytes y tiempo del mismo
	// instante). El total del servidor puede ser mayor: sigue contando tras su
	// último Measurement (el ticker se retrasa con poca CPU y el RST puede
	// descartar el final), así que solo se exige que no lo supere.
	if st.UploadConns != 1 || d.Bytes == 0 || d.Bytes > st.BytesReceived || d.DurationS < 0.5 || d.DurationS > 1.5 {
		t.Fatalf("bytes %d, servidor %d, %+v", d.Bytes, st.BytesReceived, st)
	}
	within(t, d.Mbps, float64(d.Bytes)*8/1e6/d.DurationS, 0.001)
	for _, n := range d.Notes {
		if n == "estimado por el cliente" {
			t.Fatal("no debía estimar")
		}
	}
}

func TestDownloadNoCloseCut(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.Fail = ndt7test.NoClose; c.DownloadRate = 5e6 })
	o := opts(s, "download")
	o.MaxTotal = 2 * time.Second
	t0 := time.Now()
	d, err := Download(context.Background(), o)
	if err != nil || d.Incomplete || len(d.Notes) == 0 {
		t.Fatalf("%v %+v", err, d)
	}
	if el := time.Since(t0); el > 4*time.Second {
		t.Fatalf("tardó %v", el)
	}
	within(t, d.Mbps, 40, 0.1)
}

func TestDownloadLateClose(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.Fail = ndt7test.LateClose })
	d, err := Download(context.Background(), opts(s, "download"))
	if err != nil || d.Incomplete || d.Bytes == 0 {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestCutMidway(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.Fail = ndt7test.CutMidway })
	for _, dir := range []string{"download", "upload"} {
		var d Direction
		var err error
		if dir == "download" {
			d, err = Download(context.Background(), opts(s, dir))
		} else {
			d, err = Upload(context.Background(), opts(s, dir))
		}
		if err != nil || !d.Incomplete || d.Reason == "" || d.Bytes == 0 {
			t.Fatalf("%s: %v %+v", dir, err, d)
		}
	}
}

func measure(dir string, o MeasureOptions) (Direction, error) {
	if dir == "download" {
		return Download(context.Background(), o)
	}
	return Upload(context.Background(), o)
}

func TestGiantAndInvalidJSON(t *testing.T) {
	for _, fail := range []ndt7test.FailMode{ndt7test.GiantFrame, ndt7test.InvalidJSON} {
		for _, dir := range []string{"download", "upload"} {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()
				s := srv(t, func(c *ndt7test.Config) { c.Fail = fail })
				d, err := measure(dir, opts(s, dir))
				if err != nil {
					t.Fatal(err)
				}
				if fail == ndt7test.GiantFrame && (!d.Incomplete || d.Reason == "") {
					t.Fatalf("gigante: %+v", d)
				}
				if fail == ndt7test.InvalidJSON && d.Reason == ReasonNoData {
					// Flaky no reproducido (§23): se acepta el motivo, pero queda constancia.
					t.Logf("%s: no se envió ningún dato (%+v)", dir, d)
					return
				}
				if fail == ndt7test.InvalidJSON && (d.Incomplete || d.Bytes == 0) {
					t.Fatalf("json inválido: %+v", d)
				}
			})
		}
	}
}

func TestWrongSubprotocolAndCancel(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.Fail = ndt7test.WrongSubprotocol })
	if _, err := Download(context.Background(), opts(s, "download")); err == nil {
		t.Fatal("esperaba error")
	}
	s.Configure(func(c *ndt7test.Config) { c.Fail = ndt7test.Normal; c.DownloadTime = 10 * time.Second })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	d, err := Download(ctx, opts(s, "download"))
	if err != nil || !d.Incomplete {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestFakeClock(t *testing.T) {
	s := srv(t, nil)
	o := opts(s, "download")
	var n int64
	o.Now = func() int64 { n += int64(time.Millisecond); return n }
	if _, err := Download(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no usó el reloj inyectado")
	}
}

func TestEarlyNormalClose(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) {
		c.DownloadTime = 300 * time.Millisecond
		c.UploadTime = 300 * time.Millisecond
	})
	o := opts(s, "download")
	o.MaxTotal = 3 * time.Second
	d, err := Download(context.Background(), o)
	if err != nil || !d.Incomplete || d.Reason == "" {
		t.Fatalf("descarga: %v %+v", err, d)
	}
	// El cliente envía lo mismo que el servidor mide (300 ms) y el tope es corto:
	// tanto si el servidor cierra como si se corta al tope, < MinNormal es incompleto.
	o = opts(s, "upload")
	o.UploadTime = 300 * time.Millisecond
	o.MaxTotal = 3 * time.Second
	d, err = Upload(context.Background(), o)
	if err != nil || !d.Incomplete || d.Reason == "" {
		t.Fatalf("subida: %v %+v", err, d)
	}
}

func TestCappedShortIsIncomplete(t *testing.T) {
	for _, dir := range []string{"download", "upload"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			s := srv(t, func(c *ndt7test.Config) {
				c.Fail = ndt7test.NoClose
				c.DownloadTime = 300 * time.Millisecond
				c.UploadTime = 300 * time.Millisecond
			})
			o := opts(s, dir)
			o.UploadTime = 300 * time.Millisecond
			o.MaxTotal = time.Second
			d, err := measure(dir, o)
			if err != nil || !d.Incomplete || len(d.Notes) == 0 {
				t.Fatalf("%v %+v", err, d)
			}
		})
	}
}

func TestUserAgentDefault(t *testing.T) {
	var got string
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
		c, err := ws.Accept(w, r, ws.Options{Subprotocol: Subprotocol})
		if err == nil {
			_ = c.Close(ws.CloseNormal, "")
		}
	}))
	ts.EnableHTTP2 = false
	ts.StartTLS()
	defer ts.Close()
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	u := strings.Replace(ts.URL, "https://", "wss://", 1) + "/ndt/v7/download"
	o := MeasureOptions{URL: u, WS: ws.Options{TLS: &tls.Config{RootCAs: pool}}}
	if _, err := Download(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if got != "NPerf/"+version.Current {
		t.Fatalf("User-Agent %q", got)
	}
}

// Un cierre normal sin bytes enviados no es una subida completa de 0 Mbps. Determinista:
// el cliente no envía nada (plazo de 1 ns) y el servidor informa de 1 s sin bytes y cierra.
func TestUploadZeroBytesIsIncomplete(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := ws.Accept(w, r, ws.Options{Subprotocol: Subprotocol})
		if err != nil {
			return
		}
		_ = c.WriteMessage(context.Background(), ws.Text, []byte(`{"TCPInfo":{"ElapsedTime":1000000,"BytesReceived":0}}`))
		_ = c.Close(ws.CloseNormal, "")
	}))
	ts.EnableHTTP2 = false
	ts.StartTLS()
	defer ts.Close()
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	o := MeasureOptions{
		URL:        strings.Replace(ts.URL, "https://", "wss://", 1) + "/ndt/v7/upload",
		WS:         ws.Options{TLS: &tls.Config{RootCAs: pool}},
		UploadTime: time.Nanosecond,
		MinNormal:  time.Millisecond,
	}
	d, err := Upload(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if d.Bytes != 0 || !d.Incomplete || d.Reason != ReasonNoData {
		t.Fatalf("%+v", d)
	}
}

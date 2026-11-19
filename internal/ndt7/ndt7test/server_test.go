package ndt7test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"nperf/internal/ws"
)

func newSrv(t *testing.T, f func(*Config)) *Server {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Configure(func(c *Config) {
		c.DownloadTime = 600 * time.Millisecond
		c.UploadTime = 600 * time.Millisecond
		c.LateClose = 900 * time.Millisecond
		c.MeasureInterval = 50 * time.Millisecond
		if f != nil {
			f(c)
		}
	})
	return s
}

func getLocate(t *testing.T, s *Server) (*http.Response, []byte) {
	t.Helper()
	cl := &http.Client{Transport: &http.Transport{TLSClientConfig: s.TLSConfig()}}
	resp, err := cl.Get(s.URL + "/v2/nearest/ndt/ndt7?client_name=nperf&client_version=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func urls(t *testing.T, s *Server) (dl, ul string) {
	t.Helper()
	_, b := getLocate(t, s)
	var v struct {
		Results []struct {
			URLs map[string]string `json:"urls"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &v); err != nil || len(v.Results) != 1 {
		t.Fatalf("locate: %v %s", err, b)
	}
	return v.Results[0].URLs["wss:///ndt/v7/download"], v.Results[0].URLs["wss:///ndt/v7/upload"]
}

func dial(s *Server, u string) (*ws.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return ws.Dial(ctx, u, ws.Options{TLS: s.TLSConfig(), Subprotocol: Subprotocol, UserAgent: "test"})
}

// readAll lee hasta error; devuelve bytes binarios, textos y el error final.
func readAll(c *ws.Conn, max time.Duration) (int64, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), max)
	defer cancel()
	var n int64
	var texts []string
	for {
		typ, d, err := c.ReadMessage(ctx)
		if err != nil {
			return n, texts, err
		}
		if typ == ws.Binary {
			n += int64(len(d))
		} else {
			texts = append(texts, string(d))
		}
	}
}

func TestLocateModes(t *testing.T) {
	s := newSrv(t, nil)
	resp, b := getLocate(t, s)
	if resp.StatusCode != 200 || !strings.Contains(string(b), "access_token="+Token) {
		t.Fatalf("ok: %d %s", resp.StatusCode, b)
	}
	q, ua, path := s.LastLocate()
	if q.Get("client_name") != "nperf" || ua == "" || path != "/v2/nearest/ndt/ndt7" {
		t.Fatalf("last: %v %q %q", q, ua, path)
	}
	cases := []struct {
		m    LocateMode
		code int
		has  string
	}{
		{LocateEmpty, 200, `"results":[]`}, {Locate500, 500, ""}, {Locate429, 429, ""},
		{LocateBadJSON, 200, "{no es"}, {LocateBadScheme, 200, "ws://"},
	}
	for _, c := range cases {
		s.Configure(func(cf *Config) { cf.Locate = c.m })
		resp, b := getLocate(t, s)
		if resp.StatusCode != c.code || !strings.Contains(string(b), c.has) {
			t.Errorf("modo %d: %d %s", c.m, resp.StatusCode, b)
		}
	}
	if st := s.Stats(); st.LocateRequests != 6 || st.Requests != 6 {
		t.Errorf("contadores: %+v", st)
	}
}

func TestDownloadAndUpload(t *testing.T) {
	s := newSrv(t, nil)
	dl, ul := urls(t, s)
	c, err := dial(s, dl)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n, texts, err := readAll(c, 5*time.Second)
	var ce *ws.CloseError
	if !errors.As(err, &ce) || ce.Code != ws.CloseNormal {
		t.Fatalf("fin: %v", err)
	}
	mbps := float64(n) * 8 / time.Since(start).Seconds() / 1e6
	if mbps < 30 || mbps > 50 { // 5 MB/s = 40 Mbps
		t.Errorf("descarga %.1f Mbps", mbps)
	}
	if len(texts) == 0 || !strings.Contains(texts[len(texts)-1], "ElapsedTime") {
		t.Errorf("sin Measurement: %v", texts)
	}

	u, err := dial(s, ul)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got := make(chan []string, 1)
	go func() { _, tx, _ := readAll(u, 5*time.Second); got <- tx }()
	buf := make([]byte, 8192)
	var sent int64
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); {
		if err := u.WriteMessage(ctx, ws.Binary, buf); err != nil {
			t.Fatal(err)
		}
		sent += int64(len(buf))
	}
	tx := <-got // el servidor cierra a UploadTime tras su último Measurement
	if len(tx) == 0 {
		t.Fatal("sin Measurement de subida")
	}
	var m measurement
	if err := json.Unmarshal([]byte(tx[len(tx)-1]), &m); err != nil || m.TCPInfo.BytesReceived == 0 || m.TCPInfo.BytesReceived > sent {
		t.Errorf("measurement: %v %+v sent=%d", err, m, sent)
	}
	waitFor(t, func() bool { return s.Stats().BytesReceived == sent })
	if st := s.Stats(); st.Connections != 2 || st.DownloadConns != 1 || st.UploadConns != 1 {
		t.Errorf("contadores: %+v", st)
	}
}

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condición no cumplida")
}

func TestNoToken(t *testing.T) {
	s := newSrv(t, nil)
	dl, _ := urls(t, s)
	if _, err := dial(s, strings.Split(dl, "?")[0]); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("sin token: %v", err)
	}
}

func TestFailModes(t *testing.T) {
	t.Run("WrongSubprotocol", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = WrongSubprotocol })
		dl, _ := urls(t, s)
		if _, err := dial(s, dl); err == nil || !strings.Contains(err.Error(), "subprotocolo") {
			t.Errorf("err: %v", err)
		}
	})
	t.Run("NoClose", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = NoClose })
		dl, _ := urls(t, s)
		c, _ := dial(s, dl)
		_, _, err := readAll(c, 1500*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("debía quedar abierto: %v", err)
		}
	})
	t.Run("LateClose", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = LateClose })
		dl, _ := urls(t, s)
		c, _ := dial(s, dl)
		start := time.Now()
		_, _, err := readAll(c, 5*time.Second)
		var ce *ws.CloseError
		if !errors.As(err, &ce) || time.Since(start) < 800*time.Millisecond {
			t.Errorf("err %v tras %v", err, time.Since(start))
		}
	})
	t.Run("CutMidway", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = CutMidway })
		dl, _ := urls(t, s)
		c, _ := dial(s, dl)
		n, _, err := readAll(c, 5*time.Second)
		var ce *ws.CloseError
		if err == nil || errors.As(err, &ce) || n == 0 {
			t.Errorf("corte: n=%d err=%v", n, err)
		}
	})
	t.Run("GiantFrame", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = GiantFrame })
		dl, _ := urls(t, s)
		c, _ := dial(s, dl)
		_, _, err := readAll(c, 5*time.Second)
		if !errors.Is(err, ws.ErrTooBig) {
			t.Errorf("err: %v", err)
		}
	})
	t.Run("InvalidJSON", func(t *testing.T) {
		s := newSrv(t, func(c *Config) { c.Fail = InvalidJSON })
		dl, _ := urls(t, s)
		c, _ := dial(s, dl)
		_, texts, _ := readAll(c, 5*time.Second)
		var v any
		if len(texts) == 0 || json.Unmarshal([]byte(texts[0]), &v) == nil {
			t.Errorf("textos: %v", texts)
		}
	})
}

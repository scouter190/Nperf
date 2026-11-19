package ndt7

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nperf/internal/ndt7/ndt7test"
	"nperf/internal/ws"
)

func runOpts(t *testing.T, s *ndt7test.Server) RunOptions {
	t.Helper()
	fixed := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	return RunOptions{
		Consent: true,
		Quota:   &Quota{Dir: t.TempDir()},
		Locate:  LocateOptions{BaseURL: s.URL, TLS: s.TLSConfig(), Version: "1.2.3"},
		WS:      ws.Options{TLS: s.TLSConfig()},
		Measure: MeasureOptions{UploadTime: time.Second, MinNormal: 900 * time.Millisecond},
		Now:     func() time.Time { return fixed },
	}
}

func TestRunComplete(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.DownloadRate = 5e6 })
	r, err := Run(context.Background(), runOpts(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if r.Download.Status != StatusOK || r.Upload.Status != StatusOK || r.TestsToday != 1 {
		t.Fatalf("%+v", r)
	}
	within(t, r.Download.Direction.Mbps, 40, 0.05)
	st := s.Stats()
	if st.LocateRequests != 1 || st.DownloadConns != 1 || st.UploadConns != 1 {
		t.Fatalf("%+v", st)
	}
	txt := r.Text()
	for _, w := range []string{"Descarga", "Subida", Attribution, NoticeData, "M-Lab publica la dirección IP"} {
		if !strings.Contains(txt, w) {
			t.Fatalf("falta %q en %s", w, txt)
		}
	}
}

func TestRunNoConsentNoTraffic(t *testing.T) {
	s := srv(t, nil)
	o := runOpts(t, s)
	o.Consent = false
	_, err := Run(context.Background(), o)
	if !errors.Is(err, ErrNoConsent) || s.Stats().Requests != 0 {
		t.Fatalf("%v %+v", err, s.Stats())
	}
	if n, _ := (&Quota{Dir: o.Quota.Dir}).Reserve(o.Now()); n != 1 {
		t.Fatalf("la cuota no debía consumirse: %d", n)
	}
}

func TestRunQuotaExhaustedNoLocate(t *testing.T) {
	s := srv(t, nil)
	o := runOpts(t, s)
	for i := 0; i < DailyLimit; i++ {
		if _, err := o.Quota.Reserve(o.Now()); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Run(context.Background(), o)
	if !errors.Is(err, ErrQuotaExceeded) || s.Stats().Requests != 0 {
		t.Fatalf("%v %+v", err, s.Stats())
	}
	if r.Download.Status != StatusUnavailable || r.Download.Direction != nil {
		t.Fatalf("%+v", r)
	}
}

func TestRunLocateFailureNoRetry(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.Locate = ndt7test.Locate429 })
	r, err := Run(context.Background(), runOpts(t, s))
	var le *LocateError
	if !errors.As(err, &le) || s.Stats().LocateRequests != 1 || s.Stats().Connections != 0 {
		t.Fatalf("%v %+v", err, s.Stats())
	}
	if !strings.Contains(r.Download.Reason, "limita") {
		t.Fatalf("%+v", r)
	}
}

func TestRunTLSRejected(t *testing.T) {
	s := srv(t, nil)
	o := runOpts(t, s)
	o.WS = ws.Options{} // CA no confiada en las medidas
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Download.Status != StatusUnavailable || r.Upload.Status != StatusUnavailable ||
		r.Download.Reason == "" || s.Stats().Connections != 0 {
		t.Fatalf("%+v %+v", r, s.Stats())
	}
	if !strings.Contains(r.Text(), "no disponible") {
		t.Fatal(r.Text())
	}
}

func TestRunLocateTLSRejected(t *testing.T) {
	s := srv(t, nil)
	o := runOpts(t, s)
	o.Locate.TLS = nil
	_, err := Run(context.Background(), o)
	var le *LocateError
	if !errors.As(err, &le) || s.Stats().Connections != 0 {
		t.Fatalf("%v", err)
	}
}

func TestRunConnectionRefused(t *testing.T) {
	s := srv(t, nil)
	o := runOpts(t, s)
	s.Close()
	_, err := Run(context.Background(), o)
	var le *LocateError
	if !errors.As(err, &le) || le.Reason == "" {
		t.Fatalf("%v", err)
	}
}

func TestRunCutMidwayKeepsOtherDirection(t *testing.T) {
	s := srv(t, func(c *ndt7test.Config) { c.DownloadRate = 5e6; c.Fail = ndt7test.CutMidway })
	r, err := Run(context.Background(), runOpts(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if r.Download.Status != StatusIncomplete || r.Download.Reason == "" || r.Download.Direction == nil {
		t.Fatalf("%+v", r.Download)
	}
	st := s.Stats()
	if st.DownloadConns != 1 || st.UploadConns != 1 || st.LocateRequests != 1 {
		t.Fatalf("sin reintentos: %+v", st)
	}
	if r.Upload.Direction == nil {
		t.Fatalf("la subida debía intentarse: %+v", r.Upload)
	}
}

func TestResultGoldenJSON(t *testing.T) {
	rtt := 12.5
	r := Result{
		Time: "2026-03-04T10:00:00Z", TestsToday: 3,
		Server:   &Server{Machine: "mlab1-x", City: "Madrid", Country: "ES"},
		Download: DirResult{Status: StatusOK, Direction: &Direction{Mbps: 100.5, Bytes: 125000000, DurationS: 10, MinRTTms: &rtt}},
		Upload:   DirResult{Status: StatusUnavailable, Reason: "sin conexión"},
	}
	b, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "format_version": 1,
  "command": "speed",
  "time": "2026-03-04T10:00:00Z",
  "provider": "M-Lab NDT7",
  "attribution": "` + Attribution + `",
  "server": {
    "machine": "mlab1-x",
    "city": "Madrid",
    "country": "ES"
  },
  "tests_today": 3,
  "download": {
    "mbps": 100.5,
    "bytes": 125000000,
    "duration_s": 10,
    "min_rtt_ms": 12.5,
    "incomplete": false
  },
  "download_status": "ok",
  "upload": null,
  "upload_status": "unavailable",
  "upload_reason": "sin conexión"
}`
	if string(b) != want {
		t.Fatalf("got:\n%s", b)
	}
}

func TestNoticeURLs(t *testing.T) {
	if MLabPrivacyURL != "https://www.measurementlab.net/privacy/" ||
		NPerfPrivacyURL != "https://github.com/scouter190/Nperf/blob/main/docs/privacidad.md" {
		t.Fatal("URLs")
	}
	n := Notice()
	for _, w := range []string{MLabPrivacyURL, NPerfPrivacyURL, "40 pruebas al día", "IP pública", NoticeData} {
		if !strings.Contains(n, w) {
			t.Fatalf("falta %q", w)
		}
	}
}

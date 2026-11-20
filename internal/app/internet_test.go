package app

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/history"
	"nperf/internal/ndt7"
	"nperf/internal/ndt7/ndt7test"
	"nperf/internal/probe"
	"nperf/internal/ws"
)

func speedServer(t *testing.T) *ndt7test.Server {
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
		c.DownloadRate = 5e6
	})
	return s
}

func newSpeedApp(t *testing.T, env probe.Env, s *ndt7test.Server) (*App, *ndt7.Quota) {
	t.Helper()
	a, _ := newProbeApp(t, env, direct(), nil)
	q := &ndt7.Quota{Dir: t.TempDir()}
	a.d.History = &history.Store{Dir: filepath.Join(t.TempDir(), "historial"), Enabled: true}
	a.d.Speed = &ndt7.RunOptions{
		Quota:   q,
		Locate:  ndt7.LocateOptions{BaseURL: s.URL, TLS: s.TLSConfig(), Version: "1.2.3"},
		WS:      ws.Options{TLS: s.TLSConfig()},
		Measure: ndt7.MeasureOptions{UploadTime: time.Second, MinNormal: 900 * time.Millisecond},
	}
	return a, q
}

func TestSpeedRequiresConsentEveryTime(t *testing.T) {
	s := speedServer(t)
	a, q := newSpeedApp(t, probe.Env{}, s)
	if err := a.StartSpeed(false); err == nil {
		t.Fatal("sin consentimiento debía fallar")
	}
	if st := s.Stats(); st.Requests != 0 || a.State().Probe.Running {
		t.Fatalf("hubo tráfico o estado: %+v", st)
	}
	if n, _ := q.Reserve(time.Now()); n != 1 {
		t.Fatalf("la cuota no debía consumirse: %d", n)
	}
	for i := 0; i < 2; i++ {
		if err := a.StartSpeed(true); err != nil {
			t.Fatal(err)
		}
		p := waitProbe(t, a, ProbeDone)
		if p.Kind != ProbeSpeed || len(p.Result) == 0 {
			t.Fatalf("%+v", p)
		}
		if err := a.StartSpeed(false); err == nil { // el consentimiento anterior no vale
			t.Fatal("el consentimiento no debe recordarse")
		}
	}
	if s.Stats().LocateRequests != 2 {
		t.Fatalf("%+v", s.Stats())
	}
	l, err := a.History(history.Filter{})
	if err != nil || len(l.Records) != 2 || l.Records[0].Kind != history.KindSpeed {
		t.Fatalf("historial: %+v %v", l, err)
	}
}

func TestSpeedQuotaExhaustedShowsLimit(t *testing.T) {
	s := speedServer(t)
	a, q := newSpeedApp(t, probe.Env{}, s)
	for i := 0; i < ndt7.DailyLimit; i++ {
		if _, err := q.Reserve(time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.StartSpeed(true); err != nil {
		t.Fatal(err)
	}
	p := waitProbe(t, a, ProbeError)
	if !strings.Contains(p.Error, "límite diario") || s.Stats().Requests != 0 {
		t.Fatalf("%+v %+v", p, s.Stats())
	}
	l, _ := a.History(history.Filter{})
	if len(l.Records) != 1 || l.Records[0].Status != history.StatusError {
		t.Fatalf("historial: %+v", l)
	}
}

func TestSpeedLocateFailureReported(t *testing.T) {
	s := speedServer(t)
	s.Configure(func(c *ndt7test.Config) { c.Locate = ndt7test.Locate429 })
	a, _ := newSpeedApp(t, probe.Env{}, s)
	if err := a.StartSpeed(true); err != nil {
		t.Fatal(err)
	}
	if p := waitProbe(t, a, ProbeError); !strings.Contains(p.Error, "M-Lab") {
		t.Fatalf("%+v", p)
	}
}

func TestSpeedExcludedWithTestAndProbes(t *testing.T) {
	s := speedServer(t)
	entered := make(chan struct{})
	a, _ := newSpeedApp(t, probe.Env{Echoer: blockEcho(entered)}, s)
	a.SetTest(TestState{Running: true, Status: TestRunning})
	if err := a.StartSpeed(true); err == nil || err.Error() != ProbeBlockedByTest {
		t.Fatalf("StartSpeed con TEST!: %v", err)
	}
	if err := a.StartInternet(); err == nil || err.Error() != ProbeBlockedByTest {
		t.Fatalf("StartInternet con TEST!: %v", err)
	}
	a.SetTest(TestState{})
	// Un diagnóstico en curso bloquea speed e internet, y al revés.
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := a.StartSpeed(true); err == nil || err.Error() != probeAlreadyRunning {
		t.Fatalf("StartSpeed con diagnóstico: %v", err)
	}
	if err := a.StartInternet(); err == nil || err.Error() != probeAlreadyRunning {
		t.Fatalf("StartInternet con diagnóstico: %v", err)
	}
	a.CancelProbe()
	waitProbe(t, a, ProbeCancelled)
	if s.Stats().Requests != 0 {
		t.Fatalf("tráfico no esperado: %+v", s.Stats())
	}
	// Una prueba de internet en curso bloquea diagnósticos y TEST!.
	if err := a.StartInternet(); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := a.StartProbe(ProbePing, hostRouter); err == nil || err.Error() != probeAlreadyRunning {
		t.Fatalf("StartProbe con internet: %v", err)
	}
	if err := a.StartSpeed(true); err == nil {
		t.Fatal("speed con internet en curso debía fallar")
	}
	if st := a.State(); st.CanTest || st.TestBlocked != TestBlockedByProbe {
		t.Fatalf("estado: %+v", st)
	}
	a.CancelProbe()
	waitProbe(t, a, ProbeCancelled)
}

func TestInternetFinishesAndSaves(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{Echoer: okEcho(false)}, direct(), nil)
	a.d.History = &history.Store{Dir: filepath.Join(t.TempDir(), "historial"), Enabled: true}
	a.d.Ping = probe.PingOptions{Count: 1, Interval: probe.MinInterval, Timeout: probe.MinTimeout}
	if err := a.SetInternetTargets([]string{"9.9.9.9"}); err != nil {
		t.Fatal(err)
	}
	if got := a.InternetTargets(); len(got) != 1 || got[0] != "9.9.9.9" {
		t.Fatalf("%v", got)
	}
	if err := a.StartInternet(); err != nil {
		t.Fatal(err)
	}
	p := waitProbe(t, a, ProbeDone)
	if p.Kind != ProbeInternet || p.Combined == nil || !p.Combined.Healthy || !strings.Contains(string(p.Result), `"internet"`) {
		t.Fatalf("%+v", p)
	}
	l, _ := a.History(history.Filter{})
	if len(l.Records) != 1 || l.Records[0].Kind != history.KindInternet {
		t.Fatalf("historial: %+v", l)
	}
	if err := a.SetInternetTargets([]string{"10.0.0.1"}); err == nil {
		t.Fatal("destino privado debía rechazarse")
	}
}

func TestInternetNoEchoer(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{}, direct(), nil)
	if err := a.StartInternet(); err == nil {
		t.Fatal("sin Echoer debía fallar")
	}
}

func TestOpenPrivacyOnlyFixedURLs(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{}, direct(), nil)
	var mu sync.Mutex
	var got []string
	a.d.OpenURL = func(u string) error { mu.Lock(); got = append(got, u); mu.Unlock(); return nil }
	for _, w := range []string{"mlab", "nperf", "https://evil.example", "", "../x", "MLAB"} {
		_ = a.OpenPrivacy(w)
	}
	if len(got) != 2 || got[0] != ndt7.MLabPrivacyURL || got[1] != ndt7.NPerfPrivacyURL {
		t.Fatalf("%v", got)
	}
	a.d.OpenURL = nil
	if err := a.OpenPrivacy("mlab"); err == nil {
		t.Fatal("sin abridor debía fallar")
	}
}

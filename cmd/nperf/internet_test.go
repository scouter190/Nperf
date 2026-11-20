package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nperf/internal/history"
	"nperf/internal/ndt7"
	"nperf/internal/ndt7/ndt7test"
	"nperf/internal/probe"
	"nperf/internal/ws"
)

type inetFix struct {
	out, err *bytes.Buffer
	srv      *ndt7test.Server
	echo     *fakeEcho
	cache    string
}

func useInternet(t *testing.T, stdin string, terminal bool) *inetFix {
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
	f := &inetFix{out: &bytes.Buffer{}, err: &bytes.Buffer{}, srv: s, cache: t.TempDir(), echo: okEcho()}
	cfgd := t.TempDir()
	oldE, oldO, oldR, oldC, oldCD := internetSystem, stdout, stderr, internetCacheDir, configDir
	stdout, stderr = f.out, f.err
	internetCacheDir = func() (string, error) { return f.cache, nil }
	configDir = func() (string, error) { return cfgd, nil }
	internetSystem = func() internetEnv {
		return internetEnv{
			Ping: probe.PingOptions{Count: 2, Interval: 200 * time.Millisecond, Timeout: 200 * time.Millisecond},
			Probe: func() (probe.Env, func()) {
				return probe.Env{Echoer: f.echo, Clock: probe.RealClock{}, Dialer: refuseDialer{}}, func() {}
			},
			Speed: func(string) ndt7.RunOptions {
				return ndt7.RunOptions{
					Locate:  ndt7.LocateOptions{BaseURL: s.URL, TLS: s.TLSConfig()},
					WS:      ws.Options{TLS: s.TLSConfig()},
					Measure: ndt7.MeasureOptions{UploadTime: time.Second, MinNormal: 900 * time.Millisecond},
				}
			},
			Stdin:    strings.NewReader(stdin),
			Terminal: func() bool { return terminal },
		}
	}
	t.Cleanup(func() { internetSystem, stdout, stderr, internetCacheDir, configDir = oldE, oldO, oldR, oldC, oldCD })
	_ = netip.Addr{}
	return f
}

func (f *inetFix) records(t *testing.T) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(f.cache, "NPerf", "historial", "*.json"))
	return m
}

func TestInternetLatencyOnly(t *testing.T) {
	f := useInternet(t, "", false)
	if c := runInternet(context.Background(), []string{}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	if !strings.Contains(f.out.String(), "Latencia a internet") {
		t.Fatalf("salida: %q", f.out)
	}
	if st := f.srv.Stats(); st.Requests != 0 {
		t.Fatalf("tráfico a M-Lab sin -speed: %+v", st)
	}
	if len(f.records(t)) != 0 {
		t.Fatal("guardó sin -save")
	}
}

func TestInternetSpeedConsentYes(t *testing.T) {
	f := useInternet(t, "s\n", true)
	if c := runInternet(context.Background(), []string{"-speed"}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	if st := f.srv.Stats(); st.DownloadConns != 1 || st.UploadConns != 1 {
		t.Fatalf("%+v", st)
	}
	if !strings.Contains(f.err.String(), "M-Lab publica") || strings.Contains(f.out.String(), "¿Acepta") {
		t.Fatalf("aviso mal ubicado: err=%q", f.err)
	}
	if !strings.Contains(f.out.String(), "Descarga") {
		t.Fatalf("salida: %q", f.out)
	}
}

func TestInternetSpeedRefused(t *testing.T) {
	for _, in := range []string{"n\n", "\n", ""} {
		f := useInternet(t, in, true)
		if c := runInternet(context.Background(), []string{"-speed"}); c != 1 {
			t.Fatalf("%q: código %d", in, c)
		}
		if f.srv.Stats().Requests != 0 || f.echo.calls.Load() != 0 {
			t.Fatalf("%q: hubo tráfico", in)
		}
	}
}

func TestInternetSpeedNoTerminal(t *testing.T) {
	f := useInternet(t, "s\n", false)
	if c := runInternet(context.Background(), []string{"-speed"}); c != 2 {
		t.Fatalf("código %d", c)
	}
	if f.srv.Stats().Requests != 0 || f.echo.calls.Load() != 0 {
		t.Fatal("hubo tráfico")
	}
}

func TestInternetAcceptWithoutSpeed(t *testing.T) {
	f := useInternet(t, "", true)
	if c := runInternet(context.Background(), []string{"-accept-mlab"}); c != 2 {
		t.Fatalf("código %d", c)
	}
	if f.echo.calls.Load() != 0 {
		t.Fatal("hubo tráfico")
	}
}

func TestInternetJSONStdoutOnlyJSON(t *testing.T) {
	f := useInternet(t, "", false)
	if c := runInternet(context.Background(), []string{"-speed", "-accept-mlab", "-json"}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(f.out.Bytes(), &m); err != nil {
		t.Fatalf("stdout no es JSON: %v: %q", err, f.out)
	}
	var sp struct {
		Command string `json:"command"`
	}
	if m["command"] == nil || m["format_version"] == nil || json.Unmarshal(m["speed"], &sp) != nil || sp.Command != "speed" {
		t.Fatalf("JSON: %s", f.out)
	}
	if !strings.Contains(f.err.String(), "M-Lab publica") || strings.Contains(f.out.String(), "M-Lab publica") {
		t.Fatalf("aviso: err=%q", f.err)
	}
}

func TestInternetJSONNoSpeedUsageError(t *testing.T) {
	f := useInternet(t, "", false)
	if c := runInternet(context.Background(), []string{"-accept-mlab", "-json"}); c != 2 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(f.out.String(), `"error"`) {
		t.Fatalf("stdout: %q", f.out)
	}
}

func TestInternetSave(t *testing.T) {
	f := useInternet(t, "", false)
	if c := runInternet(context.Background(), []string{"-speed", "-accept-mlab", "-save"}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	recs := f.records(t)
	if len(recs) != 2 {
		t.Fatalf("registros: %v", recs)
	}
	kinds := map[string]bool{}
	for _, p := range recs {
		b, _ := os.ReadFile(p)
		r, err := history.Decode(b)
		if err != nil {
			t.Fatal(err)
		}
		kinds[r.Kind] = true
	}
	if !kinds["internet"] || !kinds["speed"] {
		t.Fatalf("kinds: %v", kinds)
	}
}

func TestInternetSaveHistoryDisabled(t *testing.T) {
	f := useInternet(t, "", false)
	dir, _ := configDir()
	os.MkdirAll(filepath.Join(dir, "NPerf"), 0o700)
	os.WriteFile(filepath.Join(dir, "NPerf", "config.json"), []byte(`{"history_enabled": false}`), 0o600)
	if c := runInternet(context.Background(), []string{"-save"}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	if !strings.Contains(f.err.String(), "desactivado") || len(f.records(t)) != 0 {
		t.Fatalf("err=%q recs=%v", f.err, f.records(t))
	}
}

var _ = io.Discard

func TestInternetSaveFailureKeepsExitCode(t *testing.T) {
	f := useInternet(t, "", false)
	os.WriteFile(filepath.Join(f.cache, "NPerf"), []byte("x"), 0o600) // impide crear el directorio
	if c := runInternet(context.Background(), []string{"-save"}); c != 0 {
		t.Fatalf("código %d: %s", c, f.err)
	}
	if !strings.Contains(f.err.String(), "no se pudo guardar") {
		t.Fatalf("err=%q", f.err)
	}
}

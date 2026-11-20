package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nperf/internal/history"
)

// useSave inyecta el directorio del historial y la configuración.
func useSave(t *testing.T) (cache string, records func() []string) {
	t.Helper()
	cache = t.TempDir()
	oldC, oldCD := internetCacheDir, configDir
	internetCacheDir = func() (string, error) { return cache, nil }
	cfg := t.TempDir()
	configDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { internetCacheDir, configDir = oldC, oldCD })
	return cache, func() []string {
		m, _ := filepath.Glob(filepath.Join(cache, "NPerf", "historial", "*.json"))
		return m
	}
}

func kindOf(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	r, err := history.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return r.Kind
}

func TestSaveProbesOneRecordEach(t *testing.T) {
	for _, c := range []struct {
		cmd, kind string
		args      []string
	}{
		{"ping", "ping", []string{"-count", "1"}},
		{"trace", "trace", []string{"-max-hops", "2"}},
		{"mtu", "mtu", nil},
		{"ports", "ports", []string{"-p", "80"}},
	} {
		t.Run(c.cmd, func(t *testing.T) {
			_, recs := useSave(t)
			out, errb := useProbe(t, okEcho(), google)
			// Sin -save: nada.
			run(append([]string{c.cmd, "8.8.8.8"}, c.args...))
			if len(recs()) != 0 {
				t.Fatal("guardó sin -save")
			}
			out.Reset()
			run(append([]string{c.cmd, "8.8.8.8", "-json", "-save"}, c.args...))
			if r := recs(); len(r) != 1 || kindOf(t, r[0]) != c.kind {
				t.Fatalf("registros %v, err=%q", r, errb)
			}
			if strings.Contains(out.String(), "guardado") || !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
				t.Fatalf("stdout con -json: %q", out)
			}
			if !strings.Contains(errb.String(), "guardado en el historial") {
				t.Fatalf("stderr: %q", errb)
			}
		})
	}
}

func TestSaveProbeHistoryDisabledWarns(t *testing.T) {
	_, recs := useSave(t)
	dir, _ := configDir()
	os.MkdirAll(filepath.Join(dir, "NPerf"), 0o700)
	os.WriteFile(filepath.Join(dir, "NPerf", "config.json"), []byte(`{"history_enabled": false}`), 0o600)
	out, errb := useProbe(t, okEcho(), google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "1", "-json", "-save"}); c != 0 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(errb.String(), "desactivado") || len(recs()) != 0 || strings.Contains(out.String(), "desactivado") {
		t.Fatalf("err=%q out=%q recs=%v", errb, out, recs())
	}
}

func TestSaveFailureKeepsExitCode(t *testing.T) {
	_, recs := useSave(t)
	base, _ := internetCacheDir()
	os.WriteFile(filepath.Join(base, "NPerf"), []byte("x"), 0o600) // un archivo impide crear el directorio
	_, errb := useProbe(t, okEcho(), google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "1", "-save"}); c != 0 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(errb.String(), "no se pudo guardar") || len(recs()) != 0 {
		t.Fatalf("err=%q", errb)
	}
}

func TestSaveClient(t *testing.T) {
	port := startServer(t, "")
	_, recs := useSave(t)
	out, errb := capture(t, func() string { d, _ := configDir(); return d }())
	args := []string{"client", "127.0.0.1", "-port", port, "-proto", "tcp", "-direction", "fwd", "-duration", "3"}
	if c := run(append(args, "-json")); c != 0 || len(recs()) != 0 {
		t.Fatalf("código %d, err=%q, recs=%v", c, errb, recs())
	}
	out.Reset()
	if c := run(append(args, "-json", "-save")); c != 0 {
		t.Fatalf("código %d, err=%q", c, errb)
	}
	if r := recs(); len(r) != 1 || kindOf(t, r[0]) != "test" {
		t.Fatalf("registros %v", r)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") || strings.Contains(out.String(), "guardado") {
		t.Fatalf("stdout: %q", out)
	}
}

func TestHistoryAndInternetRegistered(t *testing.T) {
	out, _ := capture(t, t.TempDir())
	if c := run([]string{"help"}); c != 0 || !strings.Contains(out.String(), "nperf history") || !strings.Contains(out.String(), "nperf internet") {
		t.Fatalf("ayuda: %q", out)
	}
	out.Reset()
	if c := run([]string{"history", "help"}); c != 0 || !strings.Contains(out.String(), "history list") {
		t.Fatalf("history: %d %q", c, out)
	}
	_ = context.Background
}

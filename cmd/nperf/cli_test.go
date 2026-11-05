package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nperf/internal/config"
	"nperf/internal/server"
	"nperf/internal/testutil"
)

// capture sustituye stdout/stderr y el directorio de configuración durante la prueba.
func capture(t *testing.T, cfgDir string) (out, errb *bytes.Buffer) {
	t.Helper()
	out, errb = new(bytes.Buffer), new(bytes.Buffer)
	so, se, sc := stdout, stderr, configDir
	stdout, stderr = out, errb
	configDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { stdout, stderr, configDir = so, se, sc })
	return
}

func TestDurationOutOfRangeFailsBeforeConnecting(t *testing.T) {
	for _, d := range []string{"2", "301", "0", "-1", "1000"} {
		_, errb := capture(t, t.TempDir())
		start := time.Now()
		// Puerto 1 cerrado: si intentara conectar el código sería 1.
		if code := run([]string{"client", "127.0.0.1", "-port", "1", "-duration", d}); code != 2 {
			t.Errorf("-duration %s: código %d, quería 2", d, code)
		}
		if !strings.Contains(errb.String(), "-duration debe estar entre 3 y 300") {
			t.Errorf("-duration %s: mensaje %q", d, errb.String())
		}
		if time.Since(start) > time.Second {
			t.Errorf("-duration %s tardó demasiado", d)
		}
	}
}

func startServer(t *testing.T, key string) string {
	t.Helper()
	ln, uc := testutil.Listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		testutil.ServeErr(t, ctx, server.New(server.WithKey(key), server.WithUDPConn(uc)).Serve(ctx, ln))
	}()
	t.Cleanup(func() { cancel(); <-done })
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port
}

func TestDuration3TakesAboutThreeSeconds(t *testing.T) {
	out, _ := capture(t, t.TempDir())
	port := startServer(t, "")
	start := time.Now()
	code := run([]string{"client", "127.0.0.1", "-port", port, "-proto", "tcp", "-direction", "fwd", "-duration", "3"})
	el := time.Since(start)
	if code != 0 {
		t.Fatalf("código %d: %s", code, out)
	}
	if el < 2500*time.Millisecond || el > 3500*time.Millisecond {
		t.Fatalf("la fase duró %v, quería 3 s ±0,5 s", el)
	}
	if !strings.Contains(out.String(), "Midiendo TCP") {
		t.Errorf("salida sin 'Midiendo TCP': %s", out)
	}
}

func writeCfg(t *testing.T, dir, content string) {
	t.Helper()
	p := config.Path(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestKeyFromConfigFileAndFlagPriority(t *testing.T) {
	dir := t.TempDir()
	out, errb := capture(t, dir)
	port := startServer(t, "secreta")
	base := []string{"client", "127.0.0.1", "-port", port, "-proto", "tcp", "-direction", "fwd", "-duration", "3"}

	// Sin clave ni archivo: aviso y fallo de autenticación (código 1).
	if code := run(base); code != 1 || !strings.Contains(errb.String(), "sin clave") {
		t.Fatalf("sin clave: código %d, stderr %q", code, errb)
	}
	// Con archivo: autentica y no avisa.
	writeCfg(t, dir, `{"key": "secreta"}`)
	out.Reset()
	errb.Reset()
	if code := run(base); code != 0 || strings.Contains(errb.String(), "sin clave") {
		t.Fatalf("archivo: código %d, stderr %q, stdout %q", code, errb, out)
	}
	// El flag manda sobre el archivo: -key mala falla aunque el archivo sea buena.
	errb.Reset()
	if code := run(append(append([]string{}, base...), "-key", "otra")); code != 1 {
		t.Fatalf("flag distinto del archivo: código %d, quería 1", code)
	}
	// Y al revés: archivo mala, flag buena.
	writeCfg(t, dir, `{"key": "mala"}`)
	if code := run(append(append([]string{}, base...), "-key", "secreta")); code != 0 {
		t.Fatalf("flag bueno con archivo malo: código %d (%s)", code, errb)
	}
}

func TestInvalidConfigFileIsClearError(t *testing.T) {
	dir := t.TempDir()
	_, errb := capture(t, dir)
	writeCfg(t, dir, `{no es json`)
	if code := run([]string{"client", "127.0.0.1", "-port", "1", "-duration", "3"}); code != 2 {
		t.Fatalf("código %d, quería 2", code)
	}
	if !strings.Contains(errb.String(), "config.json") || !strings.Contains(errb.String(), "inválido") {
		t.Errorf("mensaje poco claro: %q", errb)
	}
}

func TestHelpIsSpanish(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"client", "-h"}, {"server", "-h"}, {"status", "-h"}} {
		out, _ := capture(t, t.TempDir())
		if code := run(args); code != 0 {
			t.Errorf("%v: código %d", args, code)
		}
		txt := out.String()
		if !strings.Contains(txt, "Uso") {
			t.Errorf("%v: ayuda sin 'Uso': %q", args, txt)
		}
		// Solo las líneas de descripción de flags (sangradas con una descripción) se revisan.
		for _, l := range strings.Split(txt, "\n") {
			if m := regexp.MustCompile(`(?i)\b(default|shared key|empty|both sides|the |listen|parallel|duration of|default [a-z])`).FindString(l); m != "" {
				t.Errorf("%v: texto en inglés %q en %q", args, m, l)
			}
		}
		general := args[0] == "help" || args[0] == "-h"
		if general && !strings.Contains(txt, "nperf status") {
			t.Errorf("%v: la ayuda general no menciona status", args)
		}
	}
	out, _ := capture(t, t.TempDir())
	run([]string{"client", "-h"})
	for _, f := range []string{"-port", "-duration", "-key", "-proto", "-udp-rate", "-streams", "-direction"} {
		if !strings.Contains(out.String(), f) {
			t.Errorf("client -h sin %s", f)
		}
	}
}

func TestNoArgsAndUnknownCommand(t *testing.T) {
	out, errb := capture(t, t.TempDir())
	if code := run(nil); code != 2 || !strings.Contains(out.String(), "Uso") {
		t.Errorf("sin args: código %d, salida %q", code, out)
	}
	if code := run([]string{"frobnicar"}); code != 2 || !strings.Contains(errb.String(), "desconocido") || !strings.Contains(errb.String(), "Uso") {
		t.Errorf("desconocido: código %d, stderr %q", code, errb)
	}
}

func TestModeText(t *testing.T) {
	for p, w := range map[string]string{"tcp": "Midiendo TCP", "udp": "Midiendo UDP", "both": "Medición completa (RTT, TCP y UDP)"} {
		if got := modeText(p); got != w {
			t.Errorf("%s: %q", p, got)
		}
	}
}

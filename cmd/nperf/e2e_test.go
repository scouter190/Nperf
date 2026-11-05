package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/testutil"
)

// syncBuf es un búfer seguro entre goroutines que avisa cuando aparece un texto.
type syncBuf struct {
	mu     sync.Mutex
	b      bytes.Buffer
	marker string
	seen   chan struct{}
	done   bool
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Write(p)
	if s.seen != nil && !s.done && strings.Contains(s.b.String(), s.marker) {
		s.done = true
		close(s.seen)
	}
	return len(p), nil
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// e2eEnv construye el entorno con el directorio de configuración aislado.
func e2eEnv(cfg string) []string {
	return append(os.Environ(), "APPDATA="+cfg, "XDG_CONFIG_HOME="+cfg, "HOME="+cfg, "USERPROFILE="+cfg)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	name := "nperf"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// startServer lanza `nperf server` en un puerto libre y espera a que anuncie que escucha.
func startE2EServer(t *testing.T, bin, cfg string, args ...string) string {
	t.Helper()
	var last string
	for attempt := 0; attempt < 5; attempt++ {
		ln, uc := testutil.Listen(t)
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		ln.Close()
		uc.Close()
		out := &syncBuf{marker: "Servidor escuchando", seen: make(chan struct{})}
		errBuf := &syncBuf{}
		cmd := exec.Command(bin, append([]string{"server", "-port", port}, args...)...)
		cmd.Env = e2eEnv(cfg)
		cmd.Stdout, cmd.Stderr = out, errBuf
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { cmd.Wait(); close(exited) }()
		t.Cleanup(func() {
			cmd.Process.Kill()
			<-exited
		})
		select {
		case <-out.seen:
			return port
		case <-exited: // puerto tomado entre medias: otro intento
		case <-time.After(15 * time.Second):
			cmd.Process.Kill()
		}
		last = fmt.Sprintf("puerto %s: stdout=%q stderr=%q", port, out.String(), errBuf.String())
	}
	t.Fatalf("el servidor no quedó listo tras 5 intentos; último: %s", last)
	return ""
}

type e2eRun struct {
	stdout, stderr string
	code           int
	elapsed        time.Duration
}

func startCmd(t *testing.T, bin, cfg string, args ...string) (wait func() e2eRun) {
	t.Helper()
	var so, se bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Env = e2eEnv(cfg)
	cmd.Stdout, cmd.Stderr = &so, &se
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	return func() e2eRun {
		err := cmd.Wait()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return e2eRun{so.String(), se.String(), code, time.Since(start)}
	}
}

func runCmd(t *testing.T, bin, cfg string, args ...string) e2eRun {
	t.Helper()
	return startCmd(t, bin, cfg, args...)()
}

func mustContain(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("falta %q en la salida:\n%s", sub, s)
		}
	}
}

func TestE2EAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba E2E omitida con -short")
	}
	bin := buildBinary(t)
	cfg := t.TempDir()
	port := startE2EServer(t, bin, cfg)
	addr := "127.0.0.1"

	t.Run("criterio_1_4_valores_por_defecto", func(t *testing.T) {
		r := runCmd(t, bin, cfg, "client", addr, "-port", port, "-duration", "3")
		if r.code != 0 {
			t.Fatalf("código %d\nstdout:%s\nstderr:%s", r.code, r.stdout, r.stderr)
		}
		mustContain(t, r.stdout, "RTT:", "TCP fwd", "TCP rev", "UDP fwd", "UDP rev")
		last := -1
		for _, m := range []string{"RTT:", "TCP fwd", "TCP rev", "UDP fwd", "UDP rev"} {
			i := strings.Index(r.stdout, m)
			if i <= last {
				t.Errorf("%q fuera de orden:\n%s", m, r.stdout)
			}
			last = i
		}
		if r.elapsed < 10*time.Second {
			t.Errorf("duración %v: se esperaban 4 fases de ~3 s", r.elapsed)
		}
	})

	t.Run("criterio_5_direction_fwd", func(t *testing.T) {
		r := runCmd(t, bin, cfg, "client", addr, "-port", port, "-duration", "3", "-proto", "tcp", "-direction", "fwd")
		if r.code != 0 {
			t.Fatalf("código %d\nstderr:%s", r.code, r.stderr)
		}
		mustContain(t, r.stdout, "TCP fwd", "TCP rev (servidor -> cliente): no medido")
	})

	t.Run("criterio_6_duracion", func(t *testing.T) {
		for _, d := range []string{"2", "301"} {
			r := runCmd(t, bin, cfg, "client", addr, "-port", "1", "-duration", d)
			if r.code != 2 {
				t.Errorf("-duration %s: código %d, se esperaba 2", d, r.code)
			}
			if !strings.Contains(r.stderr, "-duration") {
				t.Errorf("-duration %s: stderr sin mensaje de rango: %q", d, r.stderr)
			}
		}
		r := runCmd(t, bin, cfg, "client", addr, "-port", port, "-duration", "3", "-proto", "tcp", "-direction", "fwd")
		if r.code != 0 {
			t.Fatalf("código %d\nstderr:%s", r.code, r.stderr)
		}
		if r.elapsed < 2500*time.Millisecond || r.elapsed > 20*time.Second {
			t.Errorf("una fase de 3 s tardó %v", r.elapsed)
		}
	})

	t.Run("criterio_10_cola", func(t *testing.T) {
		args := []string{"client", addr, "-port", port, "-duration", "3", "-proto", "tcp", "-direction", "fwd"}
		w1 := startCmd(t, bin, cfg, args...)
		w2 := startCmd(t, bin, cfg, args...)
		r1, r2 := w1(), w2()
		for i, r := range []e2eRun{r1, r2} {
			if r.code != 0 {
				t.Errorf("cliente %d: código %d\nstdout:%s\nstderr:%s", i+1, r.code, r.stdout, r.stderr)
			}
		}
		if !strings.Contains(r1.stdout, "en cola (posición 1)") && !strings.Contains(r2.stdout, "en cola (posición 1)") {
			t.Errorf("ningún cliente mostró la cola:\n%s\n---\n%s", r1.stdout, r2.stdout)
		}
	})

	t.Run("criterio_11_json", func(t *testing.T) {
		r := runCmd(t, bin, cfg, "client", addr, "-port", port, "-duration", "3", "-proto", "tcp", "-direction", "fwd", "-json")
		if r.code != 0 {
			t.Fatalf("código %d\nstdout:%s\nstderr:%s", r.code, r.stdout, r.stderr)
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
			t.Fatalf("JSON inválido: %v\n%s", err, r.stdout)
		}
		if v["format_version"] != float64(1) {
			t.Errorf("format_version = %v", v["format_version"])
		}
		for _, k := range []string{"params", "rtt", "tcp", "udp", "notes"} {
			if _, ok := v[k]; !ok {
				t.Errorf("falta el campo %q", k)
			}
		}
		tcp, _ := v["tcp"].(map[string]any)
		fwd, _ := tcp["fwd"].(map[string]any)
		if _, ok := fwd["throughput_mbps"].(float64); !ok {
			t.Errorf("tcp.fwd.throughput_mbps ausente: %v", fwd)
		}
	})

	t.Run("ayuda_y_comando_desconocido", func(t *testing.T) {
		r := runCmd(t, bin, cfg, "-h")
		if r.code != 0 {
			t.Errorf("-h: código %d", r.code)
		}
		mustContain(t, r.stdout, "Uso:", "medidor de red")
		r = runCmd(t, bin, cfg, "bogus")
		if r.code != 2 {
			t.Errorf("comando desconocido: código %d, se esperaba 2", r.code)
		}
		mustContain(t, r.stderr, "comando desconocido")
	})
}

func TestE2EClaveIncorrecta(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba E2E omitida con -short")
	}
	bin := buildBinary(t)
	cfg := t.TempDir()
	port := startE2EServer(t, bin, cfg, "-key", "secreta")
	t.Run("criterio_2_clave_distinta", func(t *testing.T) {
		r := runCmd(t, bin, cfg, "client", "127.0.0.1", "-port", port, "-duration", "3", "-key", "otra")
		if r.code == 0 {
			t.Fatalf("se esperaba código != 0\nstdout:%s", r.stdout)
		}
		mustContain(t, r.stderr+r.stdout, "clave incorrecta")
	})
}

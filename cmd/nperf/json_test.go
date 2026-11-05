package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Con -json, stdout lleva solo un objeto JSON válido (avisos van por stderr).
func TestJSONStdoutOnlyJSON(t *testing.T) {
	out, errb := capture(t, t.TempDir())
	port := startServer(t, "") // sin clave: provoca el aviso por stderr
	code := run([]string{"client", "127.0.0.1", "-port", port, "-duration", "3", "-json"})
	if code != 0 {
		t.Fatalf("código %d: %s %s", code, out, errb)
	}
	dec := json.NewDecoder(strings.NewReader(out.String()))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stdout no es JSON: %v\n%s", err, out)
	}
	if dec.More() {
		t.Errorf("stdout trae más que un objeto JSON: %s", out)
	}
	if m["format_version"] != float64(1) {
		t.Errorf("format_version = %v", m["format_version"])
	}
	for _, k := range []string{"params", "rtt", "tcp", "udp", "notes"} {
		if _, ok := m[k]; !ok {
			t.Errorf("falta %s", k)
		}
	}
	if !strings.Contains(errb.String(), "aviso") {
		t.Errorf("el aviso debía ir por stderr: %q", errb)
	}
}

func TestJSONErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		code int
	}{
		"conexión": {[]string{"client", "127.0.0.1", "-port", "1", "-json"}, 1},
		"uso":      {[]string{"client", "127.0.0.1", "-duration", "1", "-json"}, 2},
	} {
		out, _ := capture(t, t.TempDir())
		if code := run(tc.args); code != tc.code {
			t.Errorf("%s: código %d, quería %d", name, code, tc.code)
		}
		var m map[string]string
		if err := json.Unmarshal(out.Bytes(), &m); err != nil || m["error"] == "" || len(m) != 1 {
			t.Errorf("%s: stdout %q (%v)", name, out, err)
		}
	}
}

func TestJSONFlagParseError(t *testing.T) {
	out, errb := capture(t, t.TempDir())
	if code := run([]string{"client", "127.0.0.1", "-json", "-duration", "x"}); code != 2 {
		t.Errorf("código %d, quería 2", code)
	}
	var m map[string]string
	if err := json.Unmarshal(out.Bytes(), &m); err != nil || m["error"] == "" {
		t.Errorf("stdout %q (%v)", out, err)
	}
	out, errb = capture(t, t.TempDir())
	if code := run([]string{"client", "127.0.0.1", "-json=false", "-duration", "x"}); code != 2 {
		t.Errorf("código %d, quería 2", code)
	}
	if strings.HasPrefix(out.String(), "{") || !strings.Contains(errb.String(), "error:") {
		t.Errorf("-json=false debía dar texto por stderr: out %q err %q", out, errb)
	}
}

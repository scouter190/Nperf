package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, content string) {
	t.Helper()
	p := Path(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if c, err := Load(dir); err != nil || c.Key != "" {
		t.Fatalf("sin archivo: %+v %v", c, err)
	}
	write(t, dir, `{"key": "abc"}`)
	if c, err := Load(dir); err != nil || c.Key != "abc" {
		t.Fatalf("válido: %+v %v", c, err)
	}
	for _, bad := range []string{`{`, `[]`, `{"clave":"x"}`, `{"key":1}`, `{"key":"a"} x`} {
		write(t, dir, bad)
		if _, err := Load(dir); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

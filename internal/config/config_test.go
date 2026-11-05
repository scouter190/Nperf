package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	for _, bad := range []string{`{`, `[]`, `{"key":1}`, `{"key":"a"} x`} {
		write(t, dir, bad)
		if _, err := Load(dir); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil || c.Duration != 10 || c.DiscoveryPort != 5301 || c.Alias != "" || len(c.ExtraSubnets) != 0 {
		t.Fatalf("sin archivo: %+v %v", c, err)
	}
	write(t, dir, `{"key":"k"}`)
	c, err = Load(dir)
	if err != nil || c.Key != "k" || c.Duration != 10 || c.DiscoveryPort != 5301 {
		t.Fatalf("parcial: %+v %v", c, err)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Config{Key: "k", Alias: "PC-sala", Duration: 30, DiscoveryPort: 6000, ExtraSubnets: []string{"10.1.0.0/24", "192.168.5.7"}}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.Key != want.Key || got.Alias != want.Alias || got.Duration != 30 ||
		got.DiscoveryPort != 6000 || len(got.ExtraSubnets) != 2 || got.ExtraSubnets[1] != "192.168.5.7" {
		t.Fatalf("ida y vuelta: %+v %v", got, err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(Path(dir))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("permisos: %v %v", st, err)
		}
	}
}

func TestUpdateKeepsKey(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"key":"secreta","duration":20}`)
	if err := Update(dir, func(c *Config) { c.Alias = "nuevo" }); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil || c.Key != "secreta" || c.Alias != "nuevo" || c.Duration != 20 {
		t.Fatalf("%+v %v", c, err)
	}
	// sobrescribir un archivo existente (Windows incluido)
	if err := Update(dir, func(c *Config) { c.Alias = "otro" }); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidValues(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{`{"duration":2}`, `{"duration":301}`, `{"discovery_port":0}`, `{"discovery_port":70000}`, `{"extra_subnets":"x"}`} {
		write(t, dir, bad)
		if _, err := LoadFull(dir); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
	write(t, dir, `{`)
	if err := Update(dir, func(c *Config) { c.Alias = "x" }); err == nil {
		t.Error("Update sobre archivo inválido debería fallar")
	}
	if b, _ := os.ReadFile(Path(dir)); string(b) != `{` {
		t.Errorf("archivo inválido modificado: %q", b)
	}
	if err := Save(dir, Config{Duration: 1, DiscoveryPort: 5301}); err == nil {
		t.Error("Save con duración inválida debería fallar")
	}
}

func TestSaveAtomicOnFailure(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Config{Key: "orig", Duration: 10, DiscoveryPort: 5301}); err != nil {
		t.Fatal(err)
	}
	old := renameFn
	renameFn = func(string, string) error { return errors.New("fallo inyectado") }
	defer func() { renameFn = old }()
	if err := Save(dir, Config{Key: "nueva", Duration: 10, DiscoveryPort: 5301}); err == nil {
		t.Fatal("debería fallar")
	}
	c, err := Load(dir)
	if err != nil || c.Key != "orig" {
		t.Fatalf("archivo original alterado: %+v %v", c, err)
	}
	ents, _ := os.ReadDir(filepath.Dir(Path(dir)))
	if len(ents) != 1 {
		t.Fatalf("quedaron temporales: %v", ents)
	}
}

// Compatibilidad entre versiones: un campo desconocido no es error y se conserva.
func TestUnknownFieldPreserved(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"key":"k","futuro":{"a":[1,2]}}`)
	if c, err := Load(dir); err != nil || c.Key != "k" {
		t.Fatalf("Load: %+v %v", c, err)
	}
	if err := Update(dir, func(c *Config) { c.Alias = "x" }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(Path(dir))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if strings.Join(strings.Fields(string(m["futuro"])), "") != `{"a":[1,2]}` || string(m["key"]) != `"k"` || string(m["alias"]) != `"x"` {
		t.Fatalf("no se conservó: %s", b)
	}
}

func TestLoadKeyIgnoresBadRanges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"key":"k","duration":999,"discovery_port":"x"}`)
	if c, err := Load(dir); err != nil || c.Key != "k" {
		t.Fatalf("CLI: %+v %v", c, err)
	}
	_, err := LoadFull(dir)
	if err == nil || !strings.Contains(err.Error(), "config.json") || !strings.Contains(err.Error(), "discovery_port") {
		t.Fatalf("LoadFull: %v", err)
	}
	write(t, dir, `{"duration":999}`)
	if _, err := LoadFull(dir); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("LoadFull rango: %v", err)
	}
}

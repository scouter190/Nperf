package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const changelog = "# Changelog\n\n## [Sin publicar]\n\n## [1.0.0] - pendiente\n\nTodo nuevo.\n\n- Algo\n\n## [0.9.0]\n\nViejo.\n"

func setup(t *testing.T, withLicense bool) (root, exe, out string) {
	t.Helper()
	d := t.TempDir()
	root, exe, out = filepath.Join(d, "r"), filepath.Join(d, "e"), filepath.Join(d, "o")
	w := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(exe, "nperf.exe"), "MZ1")
	w(filepath.Join(exe, "nperf-gui.exe"), "MZ2")
	w(filepath.Join(root, "README.md"), "readme")
	w(filepath.Join(root, "docs", "privacidad.md"), "priv")
	w(filepath.Join(root, "docs", "licencias-terceros.txt"), "lic")
	w(filepath.Join(root, "CHANGELOG.md"), changelog)
	w(filepath.Join(root, "cmd", "nperf", "winres", "winres.json"), `{"c":"ok"}`)
	w(filepath.Join(root, "cmd", "nperf-gui", "winres", "winres.json"), `{"c":"ok"}`)
	if withLicense {
		w(filepath.Join(root, "LICENSE"), "MIT")
	}
	return
}

func names(t *testing.T, p string) []string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var n []string
	for _, f := range zr.File {
		n = append(n, f.Name)
	}
	return n
}

func TestPackageContents(t *testing.T) {
	root, exe, out := setup(t, false)
	name, err := Package("1.0.0", root, exe, out)
	if err != nil {
		t.Fatal(err)
	}
	if name != "NPerf-1.0.0-windows-amd64.zip" {
		t.Fatal(name)
	}
	want := []string{"nperf.exe", "nperf-gui.exe", "README.md", "privacidad.md", "licencias-terceros.txt"}
	if got := names(t, filepath.Join(out, name)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	// con LICENSE
	root, exe, out = setup(t, true)
	name, _ = Package("1.0.0", root, exe, out)
	got := names(t, filepath.Join(out, name))
	if len(got) != 6 || got[5] != "LICENSE" {
		t.Fatalf("got %v", got)
	}
}

func TestDeterministicAndSums(t *testing.T) {
	root, exe, out := setup(t, true)
	name, err := Package("1.0.0", root, exe, out)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(out, name))
	out2 := filepath.Join(t.TempDir(), "o2")
	if _, err := Package("1.0.0", root, exe, out2); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out2, name))
	if !bytes.Equal(a, b) {
		t.Fatal("zip no determinista")
	}
	h := sha256.Sum256(a)
	sums, _ := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if string(sums) != hex.EncodeToString(h[:])+"  "+name+"\n" {
		t.Fatalf("SHA256SUMS: %q", sums)
	}
	zr, _ := zip.OpenReader(filepath.Join(out, name))
	defer zr.Close()
	rc, _ := zr.File[0].Open()
	c, _ := io.ReadAll(rc)
	if string(c) != "MZ1" {
		t.Fatal("contenido")
	}
}

func TestMissingRequired(t *testing.T) {
	root, exe, out := setup(t, false)
	os.Remove(filepath.Join(exe, "nperf-gui.exe"))
	if _, err := Package("1.0.0", root, exe, out); err == nil || !strings.Contains(err.Error(), "nperf-gui.exe") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotes(t *testing.T) {
	n, err := Notes(changelog, "1.0.0")
	if err != nil || n != "Todo nuevo.\n\n- Algo\n" {
		t.Fatalf("%q %v", n, err)
	}
	n, err = Notes(changelog, "0.9.0") // última sección
	if err != nil || n != "Viejo.\n" {
		t.Fatalf("%q %v", n, err)
	}
	if _, err := Notes(changelog, "2.0.0"); err == nil {
		t.Fatal("debía fallar sin sección")
	}
	if _, err := Notes(changelog, "1.0"); err == nil {
		t.Fatal("versión parcial")
	}
	if _, err := Notes("## [1.0.0]\n\n## [0.9.0]\nx", "1.0.0"); err == nil {
		t.Fatal("sección vacía")
	}
}

func TestPackageWritesNotes(t *testing.T) {
	root, exe, out := setup(t, false)
	if _, err := Package("1.0.0", root, exe, out); err != nil {
		t.Fatal(err)
	}
	n, _ := os.ReadFile(filepath.Join(out, "notes.md"))
	if !strings.HasPrefix(string(n), "Todo nuevo.") {
		t.Fatalf("%q", n)
	}
	if _, err := Package("3.0.0", root, exe, out); err == nil {
		t.Fatal("sin sección debe fallar")
	}
	if _, err := Package("v1.0.0", root, exe, out); err == nil {
		t.Fatal("versión inválida")
	}
}

func TestCheckCopyright(t *testing.T) {
	root, exe, out := setup(t, false)
	p := filepath.Join(root, "cmd", "nperf-gui", "winres", "winres.json")
	os.WriteFile(p, []byte(`{"c":"(c) 2026 TITULAR-PENDIENTE"}`), 0o644)
	var sb strings.Builder
	base := []string{"-version", "1.0.0", "-root", root, "-exe-dir", exe, "-out", out}
	if err := run(base, &sb); err != nil {
		t.Fatalf("sin opción no debe fallar: %v", err)
	}
	if err := run(append(base, "-check-copyright"), &sb); err == nil {
		t.Fatal("con opción debe fallar")
	}
	os.WriteFile(p, []byte(`{"c":"(c) 2026 Alguien"}`), 0o644)
	if err := run(append(base, "-check-copyright"), &sb); err != nil {
		t.Fatal(err)
	}
}

func TestNotesOnly(t *testing.T) {
	root, _, _ := setup(t, false)
	var sb strings.Builder
	if err := run([]string{"-version", "1.0.0", "-root", root}, &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sb.String(), "Todo nuevo.") {
		t.Fatal(sb.String())
	}
}

package main

import (
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildWindows compila un paquete para Windows y devuelve el subsistema del PE.
func buildWindows(t *testing.T, pkg string, ldflags string) uint16 {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.exe")
	args := []string{"build", "-o", out}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	f, err := pe.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		t.Fatalf("cabecera opcional inesperada: %T", f.OptionalHeader)
	}
	return oh.Subsystem
}

func TestSubsistemaPE(t *testing.T) {
	if testing.Short() {
		t.Skip("compila con GOOS=windows; se salta con -short")
	}
	if got := buildWindows(t, "./cmd/nperf-gui", "-H=windowsgui"); got != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
		t.Errorf("nperf-gui: subsistema %d, se esperaba 2 (GUI)", got)
	}
	if got := buildWindows(t, "./cmd/nperf", ""); got != pe.IMAGE_SUBSYSTEM_WINDOWS_CUI {
		t.Errorf("nperf: subsistema %d, se esperaba 3 (consola)", got)
	}
}

package gui_test

import (
	"path/filepath"
	"testing"

	"nperf/internal/config"
	"nperf/internal/gui"
)

func TestPathsSharedWithCLI(t *testing.T) {
	root := filepath.Join("base", "AppData")
	cfg, wv := gui.Paths(root)
	if got, want := config.Path(cfg), filepath.Join(root, "NPerf", "config.json"); got != want {
		t.Errorf("config.Path = %q, want %q", got, want)
	}
	if want := filepath.Join(root, "NPerf", "webview2"); wv != want {
		t.Errorf("webviewDir = %q, want %q", wv, want)
	}
}

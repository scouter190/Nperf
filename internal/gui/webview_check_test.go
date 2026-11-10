package gui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestVersionInstalled(t *testing.T) {
	for pv, want := range map[string]bool{"": false, " ": false, "0.0.0.0": false, "120.0.2210.91": true} {
		if got := VersionInstalled(pv); got != want {
			t.Errorf("VersionInstalled(%q)=%v", pv, got)
		}
	}
}

func TestStartupText(t *testing.T) {
	if txt, _ := StartupText(nil); txt != "" {
		t.Error("nil debe dar texto vacío")
	}
	txt, offer := StartupText(fmt.Errorf("x: %w", ErrWebViewMissing))
	if !offer || !strings.Contains(txt, WebView2DownloadURL) {
		t.Errorf("falta enlace/oferta: %q", txt)
	}
	txt, offer = StartupText(fmt.Errorf("puerto ocupado"))
	if offer || !strings.Contains(txt, "puerto ocupado") {
		t.Errorf("error genérico mal: %q", txt)
	}
}

func TestCombineAndWindowError(t *testing.T) {
	I, M, U := RuntimeInstalled, RuntimeMissing, RuntimeUnknown
	if CombineStatus(M, I, U) != I || CombineStatus(M, U) != U || CombineStatus(M, M) != M || CombineStatus() != M {
		t.Error("CombineStatus incorrecto")
	}
	if !errors.Is(WindowCreateError(M), ErrWebViewMissing) {
		t.Error("Missing debe envolver ErrWebViewMissing")
	}
	for _, st := range []RuntimeStatus{U, I} {
		if errors.Is(WindowCreateError(st), ErrWebViewMissing) {
			t.Error("no debe afirmar que falta")
		}
	}
}

//go:build windows

package history

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultDirUnderLocalAppData(t *testing.T) {
	la := os.Getenv("LocalAppData")
	if la == "" {
		t.Skip("sin LocalAppData")
	}
	d := DefaultDir(la)
	if !strings.HasPrefix(d, la) || !strings.Contains(d, `\NPerf\`) {
		t.Fatalf("ruta %q", d)
	}
}

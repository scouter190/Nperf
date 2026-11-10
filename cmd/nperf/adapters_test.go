package main

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"nperf/internal/netinfo"
)

type fakeSrc struct {
	raw []netinfo.RawAdapter
	err error
}

func (f fakeSrc) Adapters() ([]netinfo.RawAdapter, error) { return f.raw, f.err }
func (f fakeSrc) WiFi(string) (netinfo.RawWiFi, error)    { return netinfo.RawWiFi{}, nil }

func useSource(t *testing.T, s netinfo.Source) {
	t.Helper()
	old := adaptersSource
	adaptersSource = func() netinfo.Source { return s }
	t.Cleanup(func() { adaptersSource = old })
}

var okSrc = fakeSrc{raw: []netinfo.RawAdapter{{
	Name: "Ethernet", GUID: "{A}", Description: "Intel", MAC: "AA-BB-CC-DD-EE-FF",
	IPv4: []netip.Addr{netip.MustParseAddr("192.168.1.10")}, IfType: 6, OperStatus: 1,
	TxBps: 1_000_000_000, RxBps: 1_000_000_000,
}}}

func TestAdaptersText(t *testing.T) {
	useSource(t, okSrc)
	out, _ := capture(t, t.TempDir())
	if code := run([]string{"adapters"}); code != 0 {
		t.Fatalf("código %d", code)
	}
	if !strings.Contains(out.String(), "Ethernet") {
		t.Errorf("salida: %q", out.String())
	}
}

func TestAdaptersJSON(t *testing.T) {
	useSource(t, okSrc)
	out, _ := capture(t, t.TempDir())
	if code := run([]string{"adapters", "-json"}); code != 0 {
		t.Fatalf("código %d", code)
	}
	var d map[string]any
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("JSON inválido: %v: %q", err, out.String())
	}
	if d["format_version"] == nil || d["adapters"] == nil {
		t.Errorf("faltan campos: %v", d)
	}
}

func TestAdaptersFailure(t *testing.T) {
	useSource(t, fakeSrc{err: errors.New("boom")})
	out, errb := capture(t, t.TempDir())
	if code := run([]string{"adapters"}); code != 1 {
		t.Errorf("código %d, quería 1", code)
	}
	if !strings.Contains(errb.String(), "boom") {
		t.Errorf("stderr: %q", errb.String())
	}
	out.Reset()
	if code := run([]string{"adapters", "-json"}); code != 1 {
		t.Errorf("json: código %d", code)
	}
	var d map[string]string
	if json.Unmarshal(out.Bytes(), &d) != nil || d["error"] == "" {
		t.Errorf("stdout: %q", out.String())
	}
}

func TestAdaptersUsageErrors(t *testing.T) {
	useSource(t, okSrc)
	for _, a := range [][]string{{"adapters", "-nope"}, {"adapters", "extra"}} {
		capture(t, t.TempDir())
		if code := run(a); code != 2 {
			t.Errorf("%v: código %d, quería 2", a, code)
		}
	}
}

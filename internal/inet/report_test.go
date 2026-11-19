package inet

import (
	"bytes"
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nperf/internal/probe"
)

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

var update = flag.Bool("update", false, "reescribe los golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s distinto del golden:\n%s", name, got)
	}
}

func reportCases() map[string]Outcome {
	gw, a, b := addr("192.168.1.1"), addr("1.1.1.1"), addr("8.8.8.8")
	healthy := Outcome{Gateway: gw, GatewayStats: st(0, 2, 0.5),
		Targets: []Target{{Addr: a, Stats: st(0, 12, 1)}, {Addr: b, Stats: st(0, 15, 2)}}}
	healthy.Verdict = Verdict(healthy.GatewayStats, []probe.Stats{healthy.Targets[0].Stats, healthy.Targets[1].Stats}, nil)

	hop := func(ttl int, from string, s probe.Stats) probe.Hop {
		h := probe.Hop{TTL: ttl, Stats: s}
		if from != "" {
			h.From = addr(from)
		}
		return h
	}
	prov := Outcome{Gateway: gw, GatewayStats: st(0, 2, 0.5),
		Targets: []Target{{Addr: a, Stats: st(5, 150, 40)}, {Addr: b, Stats: st(0, 15, 2)}}}
	prov.Verdict = Verdict(prov.GatewayStats, []probe.Stats{prov.Targets[0].Stats, prov.Targets[1].Stats}, nil)
	prov.Trace = &probe.TraceResult{Dst: a, Reached: true, Hops: []probe.Hop{
		hop(1, "192.168.1.1", st(0, 2, 0.5)), hop(2, "", dead), hop(3, "1.1.1.1", st(5, 150, 40))}}
	prov.TraceNotes = []string{"el salto 3 pierde paquetes"}

	none := Outcome{Gateway: gw, GatewayStats: st(0, 2, 0.5),
		Targets: []Target{{Addr: a, Stats: dead, TCPTried: true}, {Addr: b, Stats: dead, TCPTried: true}}}
	none.Verdict = Verdict(none.GatewayStats, []probe.Stats{dead, dead}, []bool{false, false})

	mute := Outcome{Gateway: gw, GatewayStats: dead,
		Targets: []Target{{Addr: a, Stats: st(0, 12, 1)}, {Addr: b, Stats: st(0, 15, 2)}}}
	mute.Verdict = Verdict(dead, []probe.Stats{mute.Targets[0].Stats, mute.Targets[1].Stats}, nil)

	filtered := Outcome{Targets: []Target{{Addr: a, Stats: dead, TCPTried: true, TCP443: true}}}
	filtered.Verdict = Verdict(dead, []probe.Stats{dead}, []bool{true})

	return map[string]Outcome{"healthy": healthy, "provider": prov, "nointernet": none, "gwmute": mute, "icmpfiltered": filtered}
}

func TestReportGolden(t *testing.T) {
	for name, o := range reportCases() {
		var j, x bytes.Buffer
		if err := ReportJSON(&j, o); err != nil {
			t.Fatal(err)
		}
		ReportText(&x, o)
		golden(t, name+".golden.json", j.String())
		golden(t, name+".golden.txt", x.String())
	}
}

func TestReportContent(t *testing.T) {
	var b bytes.Buffer
	ReportText(&b, reportCases()["gwmute"])
	if !strings.Contains(b.String(), GatewayNote) || !strings.Contains(b.String(), "sin respuesta") {
		t.Errorf("falta nota de gateway muda:\n%s", b.String())
	}
}

func TestReportJSONError(t *testing.T) {
	var b bytes.Buffer
	if err := JSONError(&b, errors.New("fallo")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"error": "fallo"`) {
		t.Errorf("salida: %s", b.String())
	}
}

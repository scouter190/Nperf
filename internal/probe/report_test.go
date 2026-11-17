package probe

import (
	"bytes"
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

var tgt = Dest{Name: "servidor.lan", IP: mustAddr("192.168.1.10")}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

func pingData() (Stats, []Reply) {
	rs := []Reply{
		{Status: StatusOK, From: tgt.IP, RTTms: 0.4},
		{Status: StatusTimeout, Reason: "tiempo de espera agotado"},
		{Status: StatusOK, From: tgt.IP, RTTms: 12.34},
	}
	return NewStats(rs), rs
}

func traceData() TraceResult {
	h1 := []Reply{{Status: StatusTTLExpired, From: mustAddr("192.168.1.1"), RTTms: 0.5}, {Status: StatusTTLExpired, From: mustAddr("192.168.1.1"), RTTms: 0.7}, {Status: StatusTTLExpired, From: mustAddr("192.168.1.1"), RTTms: 0.6}}
	h2 := []Reply{{Status: StatusTimeout}, {Status: StatusTimeout}, {Status: StatusTimeout}}
	h2 = []Reply{{Status: StatusTTLExpired, From: mustAddr("10.0.0.1"), RTTms: 3}, {Status: StatusTimeout}, {Status: StatusTTLExpired, From: mustAddr("10.0.0.1"), RTTms: 4}}
	h3 := []Reply{{Status: StatusOK, From: tgt.IP, RTTms: 5}, {Status: StatusOK, From: tgt.IP, RTTms: 6}, {Status: StatusOK, From: tgt.IP, RTTms: 7}}
	mk := func(ttl int, from netip.Addr, p []Reply) Hop {
		c := append([]Reply(nil), p...)
		for i := range c {
			if c[i].Status == StatusTTLExpired {
				c[i].Status = StatusOK
			}
		}
		return Hop{TTL: ttl, From: from, Probes: p, Stats: NewStats(c)}
	}
	return TraceResult{Dst: tgt.IP, Reached: true, Hops: []Hop{
		mk(1, mustAddr("192.168.1.1"), h1), mk(2, mustAddr("10.0.0.1"), h2), mk(3, tgt.IP, h3)}}
}

var tr1 = []Reply{{Status: StatusTTLExpired, From: netip.MustParseAddr("192.168.1.1"), RTTms: 1}, {Status: StatusTTLExpired, From: netip.MustParseAddr("192.168.1.1"), RTTms: 2}, {Status: StatusTTLExpired, From: netip.MustParseAddr("192.168.1.1"), RTTms: 1}}
var tr2 = []Reply{{Status: StatusTimeout}, {Status: StatusTimeout}, {Status: StatusTimeout}}

func okAll(rs []Reply) []Reply {
	c := append([]Reply(nil), rs...)
	for i := range c {
		if c[i].Status == StatusTTLExpired {
			c[i].Status = StatusOK
		}
	}
	return c
}

func TestReportGolden(t *testing.T) {
	s, rs := pingData()
	var b bytes.Buffer
	PingText(&b, tgt, s, rs)
	golden(t, "ping.golden.txt", b.String())
	b.Reset()
	if err := PingJSON(&b, tgt, s, rs); err != nil {
		t.Fatal(err)
	}
	golden(t, "ping.golden.json", b.String())

	b.Reset()
	TraceText(&b, tgt, traceData())
	golden(t, "trace.golden.txt", b.String())
	b.Reset()
	if err := TraceJSON(&b, tgt, traceData()); err != nil {
		t.Fatal(err)
	}
	golden(t, "trace.golden.json", b.String())

	unreached := TraceResult{Dst: tgt.IP, Hops: []Hop{
		{TTL: 1, From: mustAddr("192.168.1.1"), Probes: tr1, Stats: NewStats(okAll(tr1))},
		{TTL: 2, Probes: tr2, Stats: NewStats(tr2)}}}
	b.Reset()
	TraceText(&b, tgt, unreached)
	_ = TraceJSON(&b, tgt, unreached)
	golden(t, "trace_unreached.golden.txt", b.String())

	lost := []Reply{{Status: StatusTimeout}, {Status: StatusTimeout}}
	b.Reset()
	PingText(&b, tgt, NewStats(lost), lost)
	_ = PingJSON(&b, tgt, NewStats(lost), lost)
	golden(t, "ping_lost.golden.txt", b.String())

	pr2 := []PortResult{{Port: 22, State: PortCanceled}, {Port: 80, State: PortError, Detail: "red inaccesible"}}
	b.Reset()
	PortsText(&b, tgt, pr2)
	_ = PortsJSON(&b, tgt, pr2)
	golden(t, "ports_cancel_error.golden.txt", b.String())

	ok := MTUResult{Available: true, MTU: 1400, MaxPayload: 1372, Probes: 9, InterfaceMTU: 1500, Notes: []string{mtuNoteBlack}}
	no := MTUResult{Reason: mtuReasonMute, Probes: 2, InterfaceMTU: 1500}
	b.Reset()
	MTUText(&b, tgt, ok)
	MTUText(&b, tgt, no)
	golden(t, "mtu.golden.txt", b.String())
	b.Reset()
	_ = MTUJSON(&b, tgt, ok)
	_ = MTUJSON(&b, tgt, no)
	golden(t, "mtu.golden.json", b.String())

	pr := []PortResult{{Port: 22, State: PortOpen}, {Port: 80, State: PortClosed}, {Port: 443, State: PortFiltered, Detail: "sin respuesta (timeout)"}}
	b.Reset()
	PortsText(&b, tgt, pr)
	golden(t, "ports.golden.txt", b.String())
	b.Reset()
	_ = PortsJSON(&b, tgt, pr)
	golden(t, "ports.golden.json", b.String())
}

func TestReportUnavailableAndSubMs(t *testing.T) {
	s := NewStats([]Reply{{Status: StatusTimeout}})
	var b bytes.Buffer
	PingText(&b, tgt, s, []Reply{{Status: StatusTimeout}})
	if !strings.Contains(b.String(), "no disponible") {
		t.Errorf("falta 'no disponible': %s", b.String())
	}
	b.Reset()
	_ = PingJSON(&b, tgt, s, []Reply{{Status: StatusTimeout}})
	if !strings.Contains(b.String(), `"min_ms": null`) || !strings.Contains(b.String(), `"rtt_status": "unavailable"`) {
		t.Errorf("JSON sin null+status: %s", b.String())
	}
	s2, rs2 := pingData()
	b.Reset()
	PingText(&b, tgt, s2, rs2)
	if !strings.Contains(b.String(), "< 1 ms") {
		t.Errorf("falta '< 1 ms'")
	}
}

func TestJSONError(t *testing.T) {
	var b bytes.Buffer
	_ = JSONError(&b, errors.New("no resuelve"))
	if strings.Contains(b.String(), "command") || strings.Contains(b.String(), "format_version") || !strings.Contains(b.String(), `"error": "no resuelve"`) {
		t.Errorf("%s", b.String())
	}
}

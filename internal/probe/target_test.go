package probe

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type spyResolver struct {
	calls int
	ips   []netip.Addr
	err   error
}

func (s *spyResolver) LookupIP(context.Context, string) ([]netip.Addr, error) {
	s.calls++
	return s.ips, s.err
}

func TestParseTargetRejects(t *testing.T) {
	bad := []string{"", " ", "http://a.com", "a.com:80", "10.0.0.1:80", "10.0.0.0/24", "10.0.0.1-5",
		"10.0.0.1-10.0.0.5", "a b", "a,b", "::1", "999.1.1.1", "1.2.3", "-a.com", "a..com", "a/b",
		strings.Repeat("a", 254), "10.0.0.*", "a\x01b"}
	for _, s := range bad {
		spy := &spyResolver{}
		tg, err := ParseTarget(s)
		if err == nil {
			t.Errorf("%q aceptado: %+v", s, tg)
			continue
		}
		if spy.calls != 0 {
			t.Errorf("%q: resolver llamado", s)
		}
	}
}

func TestParseTargetOK(t *testing.T) {
	for _, s := range []string{"192.168.1.1", "router", "mi-router.local", "a.b.c.", strings.Repeat("a", 63) + ".com"} {
		if _, err := ParseTarget(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	if tg, _ := ParseTarget("10.1.2.3"); !tg.IP.IsValid() || tg.Name != "" {
		t.Errorf("IP literal: %+v", tg)
	}
	if _, err := ParseTarget(strings.Repeat("a", 253)); err == nil {
		// etiqueta > 63: debe rechazarse por nombre inválido
		t.Error("etiqueta de 253 aceptada")
	}
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	spy := &spyResolver{ips: []netip.Addr{netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("10.0.0.9"), netip.MustParseAddr("10.0.0.10")}}
	tg, _ := ParseTarget("host")
	a, err := Resolve(ctx, tg, spy)
	if err != nil || a.String() != "10.0.0.9" {
		t.Fatalf("%v %v", a, err)
	}
	lit, _ := ParseTarget("10.0.0.5")
	spy.calls = 0
	if a, _ := Resolve(ctx, lit, spy); a.String() != "10.0.0.5" || spy.calls != 0 {
		t.Error("IP literal no debe resolver")
	}
	spy.ips = []netip.Addr{netip.MustParseAddr("2001:db8::1")}
	if _, err := Resolve(ctx, tg, spy); err == nil {
		t.Error("sin IPv4 debe fallar")
	}
	spy.err = errors.New("nxdomain")
	if _, err := Resolve(ctx, tg, spy); err == nil {
		t.Error("error del resolver")
	}
}

func TestCheckAddr(t *testing.T) {
	local := []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24"), netip.MustParsePrefix("10.0.0.0/22")}
	cases := map[string]bool{
		"0.0.0.0": false, "224.0.0.1": false, "239.255.255.250": false, "255.255.255.255": false,
		"192.168.1.0": false, "192.168.1.255": false, "10.0.3.255": false, "10.0.0.0": false,
		"192.168.1.1": true, "192.168.1.254": true, "10.0.1.255": true, "8.8.8.8": true, "192.168.2.255": true,
	}
	for s, ok := range cases {
		err := CheckAddr(netip.MustParseAddr(s), local)
		if (err == nil) != ok {
			t.Errorf("%s: ok=%v err=%v", s, ok, err)
		}
	}
	if CheckAddr(netip.MustParseAddr("2001:db8::1"), nil) == nil {
		t.Error("IPv6 aceptada")
	}
}

func TestOptionsLimits(t *testing.T) {
	ms := time.Millisecond
	p := func(c int, i, to time.Duration) error { return PingOptions{c, i, to}.Validate() }
	if p(10, 500*ms, time.Second) != nil || p(1, 200*ms, 200*ms) != nil || p(100, time.Second, 5*time.Second) != nil {
		t.Error("límites válidos rechazados")
	}
	for _, e := range []error{p(0, 500*ms, time.Second), p(101, 500*ms, time.Second), p(10, 199*ms, time.Second),
		p(10, 500*ms, 199*ms), p(10, 500*ms, 5001*ms)} {
		if e == nil {
			t.Error("ping inválido aceptado")
		}
	}
	tr := func(h int, to time.Duration) error { return TraceOptions{h, to}.Validate() }
	if tr(1, time.Second) != nil || tr(64, 5*time.Second) != nil || tr(0, time.Second) == nil || tr(65, time.Second) == nil ||
		tr(30, 100*ms) == nil || tr(30, 6*time.Second) == nil {
		t.Error("límites de trace")
	}
	if err := DefaultPortsOptions().Validate(); err != nil {
		t.Error(err)
	}
	if err := (PortsOptions{Ports: []int{80}, Timeout: 0}).Validate(); err == nil {
		t.Error("timeout 0 aceptado")
	}
	if err := (PortsOptions{Ports: []int{0}, Timeout: time.Second}).Validate(); err == nil {
		t.Error("puerto 0 aceptado")
	}
}

func TestParsePorts(t *testing.T) {
	got, err := ParsePorts("22,80,8000-8003")
	if err != nil || len(got) != 6 || got[5] != 8003 {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := ParsePorts("1-32"); err != nil || len(got) != 32 {
		t.Errorf("32 puertos: %v", err)
	}
	if got, err := ParsePorts("65535"); err != nil || got[0] != 65535 {
		t.Error("65535")
	}
	for _, s := range []string{"", "0", "65536", "1-33", "22,22", "a", "80-70", "0-5", "1-65536", "22,,80", "1-20,21-33", "-5", "80-"} {
		if _, err := ParsePorts(s); err == nil {
			t.Errorf("%q aceptado", s)
		}
	}
}

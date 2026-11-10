package discovery

import (
	"context"
	"errors"
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"

	"nperf/internal/proto"
)

func TestBroadcastAddr(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"192.168.1.10/24", "192.168.1.255", true},
		{"10.1.2.3/16", "10.1.255.255", true},
		{"10.0.0.5/30", "10.0.0.7", true},
		{"10.0.0.5/31", "", false},
		{"10.0.0.5/32", "", false},
		{"fe80::1/64", "", false},
	}
	for _, c := range cases {
		got, ok := BroadcastAddr(netip.MustParsePrefix(c.in))
		if ok != c.ok || (ok && got.String() != c.want) {
			t.Errorf("%s: got %v,%v", c.in, got, ok)
		}
	}
}

type sent struct {
	local, dst netip.AddrPort
	data       []byte
}

func TestBeaconPerInterface(t *testing.T) {
	var mu sync.Mutex
	var got []sent
	cur := []Iface{
		{Name: "a", IP: netip.MustParseAddr("192.168.1.10"), Broadcast: netip.MustParseAddr("192.168.1.255")},
		{Name: "b", IP: netip.MustParseAddr("10.0.0.5"), Broadcast: netip.MustParseAddr("10.0.255.255")},
	}
	ticks := make(chan time.Time)
	b := &Beacon{
		Alias: "pc", InstanceID: "0123456789abcdef", MeasurePort: 5300, Ticks: ticks,
		Ifaces: func() ([]Iface, error) { mu.Lock(); defer mu.Unlock(); return cur, nil },
		Status: func() (string, int) { return StateBusy, 2 },
		Send: func(l netip.Addr, d netip.AddrPort, data []byte) error {
			mu.Lock()
			got = append(got, sent{netip.AddrPortFrom(l, 0), d, data})
			mu.Unlock()
			return nil
		},
	}
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.Run(ctx); close(done) }()

	// Ciclo inmediato: dos anuncios.
	waitLen := func(n int) {
		t.Helper()
		for i := 0; i < 2000; i++ {
			mu.Lock()
			l := len(got)
			mu.Unlock()
			if l >= n {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("esperaba %d envíos", n)
	}
	waitLen(2)
	mu.Lock()
	for i, c := range cur {
		s := got[i]
		a, err := Parse(s.data)
		if err != nil {
			t.Fatal(err)
		}
		if a.IP != c.IP || s.local.Addr() != c.IP || s.dst.Addr() != c.Broadcast || s.dst.Port() != DefaultDiscoveryPort {
			t.Errorf("iface %s: %+v %v", c.Name, a, s.dst)
		}
		if a.State != StateBusy || a.Queue != 2 || a.Port != 5300 || a.Alias != "pc" {
			t.Errorf("contenido: %+v", a)
		}
	}
	// Una interfaz desaparece entre ciclos.
	cur = cur[:1]
	mu.Unlock()
	ticks <- time.Time{}
	waitLen(3)
	ticks <- time.Time{} // el segundo tick garantiza que el ciclo anterior terminó
	waitLen(4)
	mu.Lock()
	if n := len(got); n != 4 {
		t.Errorf("envíos = %d, quiero 4", n)
	}
	mu.Unlock()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run no terminó")
	}
	time.Sleep(10 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines colgadas: %d -> %d", before, after)
	}
}

func TestSystemIfacesLoopback(t *testing.T) {
	// No debe fallar; con loopback no incluido no aparece 127.0.0.1.
	ifs, err := SystemIfaces(false)()
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range ifs {
		if i.IP.IsLoopback() {
			t.Errorf("loopback incluido: %v", i)
		}
	}
}

func TestBeaconLogRateLimited(t *testing.T) {
	var logs, okSends int
	now := time.Unix(1000, 0)
	ifs := []Iface{
		{Name: "bad", IP: netip.MustParseAddr("192.168.1.10"), Broadcast: netip.MustParseAddr("192.168.1.255")},
		{Name: "ok", IP: netip.MustParseAddr("10.0.0.5"), Broadcast: netip.MustParseAddr("10.0.0.255")},
	}
	b := &Beacon{
		InstanceID: "0123456789abcdef", MeasurePort: 5300,
		Ifaces: func() ([]Iface, error) { return ifs, nil },
		Status: func() (string, int) { return StateAvailable, 0 },
		Send: func(l netip.Addr, _ netip.AddrPort, _ []byte) error {
			if l == ifs[0].IP {
				return errors.New("boom")
			}
			okSends++
			return nil
		},
		Logf: func(string, ...any) { logs++ },
		Now:  func() time.Time { return now },
	}
	for i := 0; i < 3; i++ {
		b.cycle()
		now = now.Add(10 * time.Second)
	}
	if logs != 1 || okSends != 3 {
		t.Fatalf("logs=%d okSends=%d", logs, okSends)
	}
	now = now.Add(time.Minute)
	b.cycle()
	if logs != 2 {
		t.Fatalf("tras un minuto logs=%d", logs)
	}
}

func TestBeaconLinkFor(t *testing.T) {
	var mu sync.Mutex
	got := map[string]Announce{}
	ifs := []Iface{
		{Name: "a", IP: netip.MustParseAddr("192.168.1.10"), Broadcast: netip.MustParseAddr("192.168.1.255")},
		{Name: "b", IP: netip.MustParseAddr("10.0.0.5"), Broadcast: netip.MustParseAddr("10.0.255.255")},
		{Name: "c", IP: netip.MustParseAddr("172.16.0.5"), Broadcast: netip.MustParseAddr("172.16.255.255")},
	}
	ticks := make(chan time.Time)
	b := &Beacon{
		Alias: "pc", InstanceID: "0123456789abcdef", MeasurePort: 5300, Ticks: ticks,
		Ifaces: func() ([]Iface, error) { return ifs, nil },
		Status: func() (string, int) { return StateAvailable, 0 },
		LinkFor: func(ip netip.Addr) *proto.Link {
			switch ip.String() {
			case "192.168.1.10":
				return &proto.Link{Kind: "wifi", TxBps: 866e6, RxBps: 780e6, SSID: "Casa"}
			case "10.0.0.5":
				return &proto.Link{Kind: "eth", TxBps: 1e9, RxBps: 1e9}
			}
			return nil
		},
		Send: func(l netip.Addr, d netip.AddrPort, data []byte) error {
			a, err := Parse(data)
			if err != nil {
				t.Errorf("parse: %v", err)
			}
			mu.Lock()
			got[l.String()] = a
			mu.Unlock()
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.Run(ctx); close(done) }()
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if a := got["192.168.1.10"]; a.Link != "wifi" || a.TxBps != 866e6 || a.RxBps != 780e6 || a.SSID != "Casa" {
		t.Errorf("wifi: %+v", a)
	}
	if a := got["10.0.0.5"]; a.Link != "eth" || a.TxBps != 1e9 || a.SSID != "" {
		t.Errorf("eth: %+v", a)
	}
	if a := got["172.16.0.5"]; a.Link != "" || a.TxBps != 0 || a.SSID != "" {
		t.Errorf("sin enlace: %+v", a)
	}
	// El listener entrega los campos.
	l := &Listener{OnAnnounce: func(a Announce, _ netip.AddrPort) {
		if a.Link != "wifi" || a.SSID != "Casa" {
			t.Errorf("listener: %+v", a)
		}
	}}
	data, _ := got["192.168.1.10"].Marshal()
	l.handle(data, netip.MustParseAddrPort("192.168.1.10:5301"), time.Now())
}

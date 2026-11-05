package discovery

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func ann(id, ip, alias, state string) Announce {
	return Announce{Alias: alias, IP: netip.MustParseAddr(ip), Port: 5300, State: state, InstanceID: id}
}

func id(n int) string { return fmt.Sprintf("%016x", n) }

func newTable() (*Table, *fakeClock) {
	c := &fakeClock{t: time.Unix(1000, 0)}
	return &Table{SelfID: id(0), Now: c.Now}, c
}

func TestTableUnreachableAndBack(t *testing.T) {
	tb, c := newTable()
	tb.Update(ann(id(1), "10.0.0.2", "A", StateBusy), netip.AddrPort{})
	c.Add(9 * time.Second)
	if p := tb.Snapshot(); len(p) != 1 || !p[0].Reachable {
		t.Fatalf("a los 9 s: %+v", p)
	}
	c.Add(time.Second)
	if p := tb.Snapshot(); len(p) != 1 || p[0].Reachable {
		t.Fatalf("a los 10 s debía ser inalcanzable: %+v", p)
	}
	tb.Update(ann(id(1), "10.0.0.2", "A", StateAvailable), netip.AddrPort{})
	p := tb.Snapshot()
	if !p[0].Reachable || p[0].State != StateAvailable {
		t.Fatalf("debía volver: %+v", p)
	}
}

func TestTableDedupAndIPChange(t *testing.T) {
	tb, _ := newTable()
	tb.Update(ann(id(1), "10.0.0.2", "A", StateAvailable), netip.AddrPort{})
	tb.Update(ann(id(1), "192.168.1.2", "A", StateAvailable), netip.AddrPort{})
	p := tb.Snapshot()
	if len(p) != 1 || p[0].IP != netip.MustParseAddr("192.168.1.2") {
		t.Fatalf("%+v", p)
	}
}

func TestTableExcludesSelf(t *testing.T) {
	tb, _ := newTable()
	tb.Update(ann(id(0), "10.0.0.2", "yo", StateAvailable), netip.AddrPort{})
	if len(tb.Snapshot()) != 0 {
		t.Fatal("la propia instancia debía excluirse")
	}
}

func TestTableExpiryAndCap(t *testing.T) {
	tb, c := newTable()
	tb.MaxPeers = 3
	for i := 1; i <= 5; i++ {
		tb.Update(ann(id(i), "10.0.0.2", fmt.Sprint("n", i), StateAvailable), netip.AddrPort{})
	}
	p := tb.Snapshot()
	if len(p) != 3 || p[2].Alias != "n3" {
		t.Fatalf("tope: %+v", p)
	}
	// Los conocidos siguen actualizándose con la tabla llena.
	tb.Update(ann(id(1), "10.0.0.9", "n1", StateBusy), netip.AddrPort{})
	if p := tb.Snapshot(); p[0].State != StateBusy {
		t.Fatalf("%+v", p)
	}
	c.Add(DefaultExpireAfter)
	if len(tb.Snapshot()) != 0 {
		t.Fatal("debían caducar")
	}
	tb.Update(ann(id(9), "10.0.0.2", "nuevo", StateAvailable), netip.AddrPort{})
	if len(tb.Snapshot()) != 1 {
		t.Fatal("tras caducar debe admitir nuevos")
	}
}

func TestTableEvictsUnreachable(t *testing.T) {
	tb, c := newTable()
	tb.MaxPeers = 3
	for i := 1; i <= 3; i++ {
		tb.Update(ann(id(i), "10.0.0.2", fmt.Sprint("n", i), StateAvailable), netip.AddrPort{})
		c.Add(time.Second)
	}
	// Todas alcanzables: el nuevo se descarta.
	tb.Update(ann(id(9), "10.0.0.2", "nuevo", StateAvailable), netip.AddrPort{})
	if p := tb.Snapshot(); len(p) != 3 || p[0].Alias != "n1" || p[2].Alias != "n3" {
		t.Fatalf("%+v", p)
	}
	c.Add(11 * time.Second)
	tb.Update(ann(id(9), "10.0.0.2", "nuevo", StateAvailable), netip.AddrPort{})
	p := tb.Snapshot()
	if len(p) != 3 || p[0].Alias != "n2" || p[1].Alias != "n3" || p[2].Alias != "nuevo" {
		t.Fatalf("debía desalojar a n1: %+v", p)
	}
}

func TestTableSorted(t *testing.T) {
	tb, _ := newTable()
	tb.Update(ann(id(2), "10.0.0.3", "B", StateAvailable), netip.AddrPort{})
	tb.Update(ann(id(1), "10.0.0.2", "A", StateAvailable), netip.AddrPort{})
	p := tb.Snapshot()
	if p[0].Alias != "A" || p[1].Alias != "B" {
		t.Fatalf("%+v", p)
	}
}

func TestTableLoopbackTwoInstances(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	lo := netip.MustParseAddr("127.0.0.1")
	tb := &Table{SelfID: id(0)}
	l := &Listener{Conn: conn, OnAnnounce: tb.Update}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); _ = l.Serve(ctx) }()
	const iv = 50 * time.Millisecond
	for i := 1; i <= 2; i++ {
		b := &Beacon{
			Alias: fmt.Sprint("eq", i), InstanceID: id(i), MeasurePort: 5300,
			DiscoveryPort: port, Interval: iv,
			Ifaces: func() ([]Iface, error) { return []Iface{{Name: "lo", IP: lo, Broadcast: lo}}, nil },
			Status: func() (string, int) { return StateAvailable, 0 },
		}
		go func() { defer wg.Done(); b.Run(ctx) }()
	}
	deadline := time.Now().Add(2 * iv)
	// Margen para planificación; el límite funcional es ~2 ciclos, 5 s evita flakiness.
	for len(tb.Snapshot()) < 2 && time.Now().Before(deadline.Add(5*time.Second)) {
		time.Sleep(10 * time.Millisecond)
	}
	got := tb.Snapshot()
	cancel()
	wg.Wait()
	if len(got) != 2 || got[0].Alias != "eq1" || got[1].Alias != "eq2" {
		t.Fatalf("%+v", got)
	}
}

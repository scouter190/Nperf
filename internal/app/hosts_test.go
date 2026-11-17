package app

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/discovery"
	"nperf/internal/probe"
)

type fakeRouter struct {
	mu    sync.Mutex
	gw    netip.Addr
	err   error
	calls int
}

func (f *fakeRouter) Route(netip.Addr) (probe.Route, error) { return probe.Route{}, errors.New("no") }
func (f *fakeRouter) Gateway() (netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.gw, f.err
}
func (f *fakeRouter) LocalPrefixes() ([]netip.Prefix, error) {
	return []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}, nil
}
func (f *fakeRouter) set(ip string) {
	f.mu.Lock()
	f.gw = netip.MustParseAddr(ip)
	f.mu.Unlock()
}

func newHostApp(t *testing.T, r probe.Router) (*App, *clock, string) {
	t.Helper()
	ck := &clock{t: time.Unix(1000, 0)}
	dir := t.TempDir()
	a, err := New(Deps{ConfigDir: dir, Table: &discovery.Table{SelfID: selfID, Now: ck.now}, SelfID: selfID, Now: ck.now,
		Hostname: func() (string, error) { return "PC", nil }, Router: r})
	if err != nil {
		t.Fatal(err)
	}
	return a, ck, dir
}

func TestHostsAddRemovePersist(t *testing.T) {
	a, ck, dir := newHostApp(t, &fakeRouter{err: probe.ErrNoGateway})
	if err := a.AddHost(" Example.com ", "  Mi\x01 web  "); err != nil {
		t.Fatal(err)
	}
	if err := a.AddHost("192.168.1.50", ""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"example.COM", "http://x", "192.168.1.255", "10.0.0.0/8", ""} {
		if a.AddHost(bad, "") == nil {
			t.Errorf("%q debe rechazarse", bad)
		}
	}
	s := a.State()
	if len(s.NoAgent) != 2 || s.NoAgent[0].Alias != "Mi web" || s.NoAgent[1].Alias != "192.168.1.50" || s.NoAgent[0].Label != "sin NPerf" {
		t.Fatalf("noagent: %+v", s.NoAgent)
	}
	// recarga
	b, err := New(Deps{ConfigDir: dir, Table: &discovery.Table{SelfID: selfID, Now: ck.now}, SelfID: selfID, Now: ck.now})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.State().NoAgent) != 2 {
		t.Fatalf("recarga: %+v", b.State().NoAgent)
	}
	if err := a.RemoveHost(s.NoAgent[0].ID); err != nil {
		t.Fatal(err)
	}
	if a.RemoveHost("host:nada") == nil || a.RemoveHost("xx") == nil {
		t.Fatal("quitar inexistente/ajeno debe fallar")
	}
	if n := len(a.State().NoAgent); n != 1 {
		t.Fatalf("tras quitar: %d", n)
	}
}

func TestHostsLimit64(t *testing.T) {
	a, _, _ := newHostApp(t, nil)
	for i := 0; i < 64; i++ {
		if err := a.AddHost("10.1.0."+strconv.Itoa(i+1), ""); err != nil {
			t.Fatal(i, err)
		}
	}
	if err := a.AddHost("10.2.0.1", ""); err == nil || !strings.Contains(err.Error(), "máximo") {
		t.Fatalf("65.º: %v", err)
	}
}

func TestGatewayAlwaysPresentCachedNotRemovable(t *testing.T) {
	r := &fakeRouter{}
	r.set("192.168.1.1")
	a, ck, _ := newHostApp(t, r)
	s := a.State()
	if len(s.NoAgent) != 1 || s.NoAgent[0].ID != GatewayID || s.NoAgent[0].Alias != "Puerta de enlace (sin NPerf)" || s.NoAgent[0].IPs[0] != "192.168.1.1" {
		t.Fatalf("gateway: %+v", s.NoAgent)
	}
	r.set("192.168.1.254")
	if a.State().NoAgent[0].IPs[0] != "192.168.1.1" {
		t.Fatal("la caché de 5 s debe conservar el valor")
	}
	ck.add(6 * time.Second)
	if a.State().NoAgent[0].IPs[0] != "192.168.1.254" {
		t.Fatal("tras 5 s debe actualizarse")
	}
	if a.RemoveHost(GatewayID) == nil {
		t.Fatal("la puerta de enlace no se quita")
	}
	if a.AddHost("192.168.1.254", "") == nil {
		t.Fatal("duplicar la puerta de enlace debe fallar")
	}
	r.mu.Lock()
	r.err = probe.ErrNoGateway
	r.mu.Unlock()
	ck.add(6 * time.Second)
	if n := len(a.State().NoAgent); n != 0 {
		t.Fatalf("sin puerta de enlace: %d", n)
	}
}

func TestNoAgentCannotTestAndNoKeyInJSON(t *testing.T) {
	r := &fakeRouter{}
	r.set("192.168.1.1")
	a, _, _ := newHostApp(t, r)
	if err := a.SetKey("secreto-super"); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectPeer(GatewayID); err != nil {
		t.Fatal(err)
	}
	s := a.State()
	if s.CanTest || s.SelectedID != GatewayID || s.TestBlocked == "" {
		t.Fatalf("can_test: %+v", s)
	}
	if _, err := a.Target(); err == nil {
		t.Fatal("Target de sin NPerf debe fallar")
	}
	if a.SelectPeer("host:nope") == nil {
		t.Fatal("seleccionar inexistente debe fallar")
	}
	if err := a.AddHost("example.org", "x"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(a.State())
	if strings.Contains(string(b), "secreto-super") || !strings.Contains(string(b), `"no_agent"`) {
		t.Fatalf("json: %s", b)
	}
}

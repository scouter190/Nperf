package app

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/config"
	"nperf/internal/discovery"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const (
	selfID = "0000000000000000"
	idA    = "aaaaaaaaaaaaaaaa"
	idB    = "bbbbbbbbbbbbbbbb"
)

func newApp(t *testing.T) (*App, *discovery.Table, *clock, string) {
	t.Helper()
	ck := &clock{t: time.Unix(1000, 0)}
	tb := &discovery.Table{SelfID: selfID, Now: ck.now}
	dir := t.TempDir()
	a, err := New(Deps{ConfigDir: dir, Table: tb, SelfID: selfID, Now: ck.now,
		SelfIPs:  func() []string { return []string{"10.0.0.1"} },
		Hostname: func() (string, error) { return "PC-LOCAL", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return a, tb, ck, dir
}

func announce(tb *discovery.Table, id, alias, ip, state string, q int) {
	tb.Update(discovery.Announce{Alias: alias, IP: netip.MustParseAddr(ip), Port: 5300, State: state, Queue: q, InstanceID: id}, netip.AddrPort{})
}

func TestListSelfApartAndOrder(t *testing.T) {
	a, tb, ck, _ := newApp(t)
	announce(tb, idB, "Zeta", "10.0.0.3", discovery.StateBusy, 2)
	announce(tb, idA, "Alfa", "10.0.0.2", discovery.StateAvailable, 0)
	s := a.State()
	if s.Self.Alias != "PC-LOCAL" || s.Self.IPs[0] != "10.0.0.1" || s.Self.Status != StatusAvailable {
		t.Fatalf("self: %+v", s.Self)
	}
	if len(s.Peers) != 2 || s.Peers[0].Alias != "Alfa" || s.Peers[1].Alias != "Zeta" {
		t.Fatalf("peers: %+v", s.Peers)
	}
	if s.Peers[1].Status != StatusQueued || s.Peers[1].Label != "en cola 2" || s.Peers[0].Label != "disponible" {
		t.Fatalf("estados: %+v", s.Peers)
	}
	ck.add(11 * time.Second)
	if s = a.State(); s.Peers[0].Status != StatusUnreachable || s.Peers[0].Label != "inalcanzable" {
		t.Fatalf("inalcanzable: %+v", s.Peers[0])
	}
}

func TestSelfStateInjected(t *testing.T) {
	a, _, _, _ := newApp(t)
	a.d.SelfState = func() (string, int) { return discovery.StateBusy, 0 }
	if s := a.State(); s.Self.Status != StatusBusy || s.Self.Label != "ocupado" {
		t.Fatalf("%+v", s.Self)
	}
}

func TestSelectPeer(t *testing.T) {
	a, tb, ck, _ := newApp(t)
	announce(tb, idA, "Alfa", "10.0.0.2", discovery.StateAvailable, 0)
	for _, id := range []string{"", selfID, "ffffffffffffffff"} {
		if err := a.SelectPeer(id); err == nil {
			t.Fatalf("%q debía rechazarse", id)
		}
	}
	if _, err := a.Target(); err == nil {
		t.Fatal("sin destino debía fallar")
	}
	if err := a.SelectPeer(idA); err != nil {
		t.Fatal(err)
	}
	p, err := a.Target()
	if err != nil || p.Alias != "Alfa" {
		t.Fatal(p, err)
	}
	if s := a.State(); s.SelectedID != idA || !strings.Contains(s.Directions[1].Label, "PC-LOCAL → Alfa") ||
		s.Directions[2].Label != "Alfa → PC-LOCAL" {
		t.Fatalf("%+v", s.Directions)
	}
	ck.add(11 * time.Second)
	if err := a.SelectPeer(idA); err == nil || !strings.Contains(err.Error(), "inalcanzable") {
		t.Fatal(err)
	}
	if _, err := a.Target(); err == nil {
		t.Fatal("inalcanzable debía fallar")
	}
}

func TestDurationAndDirection(t *testing.T) {
	a, _, _, dir := newApp(t)
	s := a.State()
	if s.Direction != DirBoth || s.DurationSeconds != 10 || s.TotalSeconds != 20 || !strings.Contains(s.TotalLabel, "20 s en total") {
		t.Fatalf("%+v", s)
	}
	for _, d := range []int{2, 301, 0, -1} {
		if a.SetDuration(d) == nil {
			t.Fatalf("%d debía rechazarse", d)
		}
	}
	if err := a.SetDuration(30); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDirection("rev"); err != nil {
		t.Fatal(err)
	}
	if s = a.State(); s.TotalSeconds != 30 || s.TotalLabel != "30 s" {
		t.Fatalf("%+v", s)
	}
	if a.SetDirection("up") == nil {
		t.Fatal("sentido inválido")
	}
	// la duración se recuerda
	a2, err := New(Deps{ConfigDir: dir, Table: &discovery.Table{}})
	if err != nil || a2.State().DurationSeconds != 30 {
		t.Fatal(err)
	}
}

func TestAliasDefaultAndPersist(t *testing.T) {
	a, _, _, dir := newApp(t)
	if a.Alias() != "PC-LOCAL" {
		t.Fatal(a.Alias())
	}
	if err := a.SetKey("secreto"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAlias("  Mi PC "); err != nil {
		t.Fatal(err)
	}
	if err := a.SetDuration(45); err != nil {
		t.Fatal(err)
	}
	c, err := config.LoadFull(dir)
	if err != nil || c.Alias != "Mi PC" || c.Key != "secreto" || c.Duration != 45 {
		t.Fatalf("%+v %v", c, err)
	}
	if !a.HasKey() || a.State().Alias != "Mi PC" {
		t.Fatal("alias/clave")
	}
	if err := a.SetAlias(""); err != nil || a.Alias() != "PC-LOCAL" {
		t.Fatal("vacío debe volver al hostname", err)
	}
	if c, _ = config.LoadFull(dir); c.Key != "secreto" {
		t.Fatal("la clave se perdió")
	}
}

func TestSubnets(t *testing.T) {
	a, _, _, dir := newApp(t)
	for _, bad := range []string{"", "10.0.0.0/8", "::1", "abc"} {
		if a.AddSubnet(bad) == nil {
			t.Fatalf("%q debía rechazarse", bad)
		}
	}
	if err := a.SetKey("k"); err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{"192.168.5.0/24", "10.1.1.7", "192.168.5.0/24"} {
		if err := a.AddSubnet(ok); err != nil {
			t.Fatal(err)
		}
	}
	if c, _ := config.LoadFull(dir); len(c.ExtraSubnets) != 2 || c.Key != "k" {
		t.Fatalf("%+v", c)
	}
	if err := a.RemoveSubnet("10.1.1.7"); err != nil {
		t.Fatal(err)
	}
	if a.RemoveSubnet("10.1.1.7") == nil {
		t.Fatal("quitar inexistente debía fallar")
	}
	if got := a.State().ExtraSubnets; len(got) != 1 || got[0] != "192.168.5.0/24" {
		t.Fatal(got)
	}
}

func TestFirewallHint(t *testing.T) {
	a, tb, ck, _ := newApp(t)
	if a.State().FirewallHint != "" {
		t.Fatal("pista prematura")
	}
	ck.add(14 * time.Second)
	if a.State().FirewallHint != "" {
		t.Fatal("pista a los 14 s")
	}
	ck.add(1 * time.Second)
	h := a.State().FirewallHint
	if !strings.Contains(h, "5300") || !strings.Contains(h, "5301") {
		t.Fatal(h)
	}
	announce(tb, idA, "Alfa", "10.0.0.2", discovery.StateAvailable, 0)
	if a.State().FirewallHint != "" {
		t.Fatal("con equipos no hay pista")
	}
}

func TestStateJSONAndConcurrency(t *testing.T) {
	a, tb, _, _ := newApp(t)
	announce(tb, idA, "Alfa", "10.0.0.2", discovery.StateAvailable, 0)
	b, err := json.Marshal(a.State())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"self", "peers", "selected_id", "direction", "directions", "duration_seconds", "firewall_hint", "test", "extra_subnets"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("falta %s en %s", k, b)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				a.State()
				_ = a.SelectPeer(idA)
				_ = a.SetDirection(DirFwd)
				_ = a.SetDuration(10 + j%5)
				a.SetTest(TestState{Running: j%2 == 0})
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentConfigUpdates(t *testing.T) {
	a, _, _, dir := newApp(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = a.AddSubnet(fmt.Sprintf("10.2.0.%d", i+1)) }()
		go func() { defer wg.Done(); _ = a.SetAlias("X") }()
	}
	wg.Wait()
	c, err := config.LoadFull(dir)
	if err != nil || c.Alias != "X" || len(c.ExtraSubnets) != 10 {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestSetKeyValidationAndPrivacy(t *testing.T) {
	a, _, _, dir := newApp(t)
	for _, k := range []string{"", "   ", strings.Repeat("a", 257)} {
		if a.SetKey(k) == nil {
			t.Fatalf("%q debía rechazarse", k)
		}
	}
	_ = a.SetAlias("Mio")
	_ = a.AddSubnet("10.9.9.9")
	if err := a.SetKey("supersecreta"); err != nil {
		t.Fatal(err)
	}
	c, _ := config.LoadFull(dir)
	if c.Alias != "Mio" || len(c.ExtraSubnets) != 1 || c.Key != "supersecreta" {
		t.Fatalf("%+v", c)
	}
	b, _ := json.Marshal(a.State())
	if strings.Contains(string(b), "supersecreta") {
		t.Fatal("la clave aparece en State")
	}
}

func TestCanTest(t *testing.T) {
	a, tb, ck, _ := newApp(t)
	if s := a.State(); s.CanTest || s.TestBlocked == "" {
		t.Fatalf("sin destino: %+v", s)
	}
	announce(tb, idA, "Alfa", "10.0.0.2", discovery.StateAvailable, 0)
	_ = a.SelectPeer(idA)
	if s := a.State(); !s.CanTest || s.TestBlocked != "" {
		t.Fatalf("%+v", s)
	}
	a.SetTest(TestState{Running: true})
	if a.State().CanTest {
		t.Fatal("en curso")
	}
	a.SetTest(TestState{})
	ck.add(11 * time.Second)
	if s := a.State(); s.CanTest || !strings.Contains(s.TestBlocked, "inalcanzable") {
		t.Fatalf("%+v", s)
	}
}

func TestConfigError(t *testing.T) {
	a, _, _, dir := newApp(t)
	if err := os.MkdirAll(filepath.Dir(config.Path(dir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(dir), []byte("{no json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := a.State()
	if s.ConfigError == "" || s.Alias != "PC-LOCAL" {
		t.Fatalf("%+v", s)
	}
}

func TestStateHasKey(t *testing.T) {
	a, _, _, _ := newApp(t)
	if a.State().HasKey {
		t.Fatal("sin clave guardada has_key debe ser false")
	}
	if err := a.SetKey("secreta"); err != nil {
		t.Fatal(err)
	}
	s := a.State()
	if !s.HasKey {
		t.Fatal("has_key debe ser true tras guardar la clave")
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"has_key":true`) || strings.Contains(string(b), "secreta") {
		t.Fatalf("json: has_key ausente o la clave se filtra: %s", b)
	}
}

func TestRuntimeErrorInState(t *testing.T) {
	a, _, _, _ := newApp(t)
	a.SetRuntimeError("puerto ocupado")
	if got := a.State().ConfigError; got != "puerto ocupado" {
		t.Fatalf("config_error = %q", got)
	}
	a.SetRuntimeError("")
	if a.State().ConfigError != "" {
		t.Fatal("el error debe limpiarse")
	}
}

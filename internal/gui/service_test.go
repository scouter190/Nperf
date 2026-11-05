package gui

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"nperf/internal/server"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/app"
	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/testutil"
)

func loIfaces() ([]discovery.Iface, error) {
	lo := netip.MustParseAddr("127.0.0.1")
	return []discovery.Iface{{Name: "lo", IP: lo, Broadcast: lo}}, nil
}

// base entrega sockets nuevos en cada arranque, como un Start real.
func base(t *testing.T) func() app.RuntimeOptions {
	return func() app.RuntimeOptions {
		ln, uc := testutil.Listen(t)
		pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return app.RuntimeOptions{MeasureListener: ln, MeasureUDP: uc, DiscoveryConn: pc, Ifaces: loIfaces,
			Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil }}
	}
}

func TestSaveRestartsRuntime(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var starts []app.RuntimeOptions
	var tables []*discovery.Table
	st := func(ctx context.Context, o app.RuntimeOptions) (*app.Runtime, error) {
		mu.Lock()
		starts = append(starts, o)
		tables = append(tables, o.Table)
		mu.Unlock()
		return app.Start(ctx, o)
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(starts) }
	var done <-chan struct{}
	svc, err := NewService(context.Background(), dir, base(t), st)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	b := &Bridge{Secret: sec, B: svc.Backend(), OnConfigChange: func() { done = svc.RestartAsync() }}
	if count() != 1 {
		t.Fatalf("arranques iniciales: %d", count())
	}
	for _, c := range []struct{ name, arg string }{{"setAlias", `"Taller"`}, {"setKey", `"clave"`}, {"addSubnet", `"192.168.5.0/24"`}, {"removeSubnet", `"192.168.5.0/24"`}} {
		n := count()
		if _, err := b.Call(sec, c.name, []byte(c.arg)); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		<-done
		if count() <= n {
			t.Fatalf("%s no reinició el runtime", c.name)
		}
	}
	last := starts[len(starts)-1]
	if last.Alias != "Taller" || last.ConfigDir != dir {
		t.Fatalf("el reinicio no usa la configuración nueva: %+v", last)
	}
	if tables[0] != tables[len(tables)-1] || tables[0].SelfID == "" {
		t.Fatal("tabla e id de instancia deben conservarse")
	}
	s := svc.App.State()
	if !s.HasKey || s.Alias != "Taller" || s.ConfigError != "" {
		t.Fatalf("estado: %+v", s)
	}
	// una opción inválida no reinicia ni toca el runtime
	n := count()
	if _, err := b.Call(sec, "addSubnet", []byte(`"10.0.0.0/8"`)); err == nil {
		t.Fatal("subred /8 debe rechazarse")
	}
	if count() != n {
		t.Fatal("un guardado fallido no debe reiniciar")
	}
}

func TestRestartFailureReachesUI(t *testing.T) {
	dir := t.TempDir()
	if err := config.Save(dir, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	fail := false
	st := func(ctx context.Context, o app.RuntimeOptions) (*app.Runtime, error) {
		if fail {
			return nil, errBusy
		}
		return app.Start(ctx, o)
	}
	svc, err := NewService(context.Background(), dir, base(t), st)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	fail = true
	if err := svc.App.SetAlias("X"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(); err == nil {
		t.Fatal("el reinicio debía fallar")
	}
	if ce := svc.App.State().ConfigError; !strings.Contains(ce, "puerto ocupado") {
		t.Fatalf("config_error = %q", ce)
	}
	fail = false
	if err := svc.Restart(); err != nil {
		t.Fatal(err)
	}
	if ce := svc.App.State().ConfigError; ce != "" {
		t.Fatalf("el error debe limpiarse: %q", ce)
	}
}

func TestInitialStartFailure(t *testing.T) {
	st := func(context.Context, app.RuntimeOptions) (*app.Runtime, error) { return nil, errBusy }
	if _, err := NewService(context.Background(), t.TempDir(), base(t), st); err == nil {
		t.Fatal("el arranque fallido debe devolver error")
	}
}

type strErr string

func (e strErr) Error() string { return string(e) }

const errBusy = strErr("puerto ocupado")

func TestStopPreventsRestart(t *testing.T) {
	n := 0
	st := func(ctx context.Context, o app.RuntimeOptions) (*app.Runtime, error) { n++; return app.Start(ctx, o) }
	svc, err := NewService(context.Background(), t.TempDir(), base(t), st)
	if err != nil {
		t.Fatal(err)
	}
	svc.Stop()
	svc.Stop()
	svc.Restart()
	if n != 1 {
		t.Fatalf("arranques tras Stop: %d", n)
	}
}

// Decisión: guardar la clave (u otra config) durante una prueba NO la aborta. El cliente
// toma su configuración al empezar y la medición va por sus propios sockets; el reinicio
// solo toca el runtime (server propio + descubrimiento).
func TestSetKeyDuringTestDoesNotAbort(t *testing.T) {
	ln, uc := testutil.Listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	sdone := make(chan struct{})
	go func() {
		testutil.ServeErr(t, ctx, server.New(server.WithUDPConn(uc), server.WithMaxPerIP(0)).Serve(ctx, ln))
		close(sdone)
	}()
	t.Cleanup(func() { cancel(); <-sdone })

	svc, err := NewService(context.Background(), t.TempDir(), base(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Stop()
	ap := netip.MustParseAddrPort(ln.Addr().String())
	svc.table.Update(discovery.Announce{Alias: "Dest", IP: ap.Addr(), Port: int(ap.Port()), State: discovery.StateAvailable, InstanceID: "aaaaaaaaaaaaaaaa"}, netip.AddrPort{})
	var done <-chan struct{}
	b := &Bridge{Secret: sec, B: svc.Backend(), OnConfigChange: func() { done = svc.RestartAsync() }}
	for _, c := range []struct{ n, a string }{{"selectPeer", `"aaaaaaaaaaaaaaaa"`}, {"setDirection", `"fwd"`}, {"setDuration", `3`}, {"startTest", ``}} {
		var args []json.RawMessage
		if c.a != "" {
			args = []json.RawMessage{json.RawMessage(c.a)}
		}
		if _, err := b.Call(sec, c.n, args...); err != nil {
			t.Fatalf("%s: %v", c.n, err)
		}
	}
	if !svc.App.State().Test.Running {
		t.Fatal("la prueba debía estar en curso")
	}
	if _, err := b.Call(sec, "setKey", json.RawMessage(`"otra"`)); err != nil {
		t.Fatal(err)
	}
	<-done // el runtime se reinició con la prueba en curso
	if !svc.App.State().Test.Running && svc.App.State().Test.Status != app.TestDone {
		t.Fatalf("la prueba no debía abortarse: %+v", svc.App.State().Test)
	}
	if !svc.App.WaitTest(60 * time.Second) {
		t.Fatal("la prueba no terminó")
	}
	if st := svc.App.State().Test; st.Status != app.TestDone || st.Error != "" {
		t.Fatalf("la prueba debía terminar bien: %+v", st)
	}
}

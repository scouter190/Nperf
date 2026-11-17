package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/discovery"
	"nperf/internal/netinfo"
	"nperf/internal/probe"
)

// echoFn es un Echoer falso.
type echoFn func(ctx context.Context, r probe.EchoRequest) (probe.Reply, error)

func (f echoFn) Echo(ctx context.Context, r probe.EchoRequest) (probe.Reply, error) { return f(ctx, r) }

type refuseDialer struct{}

func (refuseDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("sin conexión")
}

type routeFake struct {
	rt  probe.Route
	err error
}

func (r routeFake) Route(netip.Addr) (probe.Route, error) { return r.rt, r.err }
func (r routeFake) Gateway() (netip.Addr, error)          { return netip.Addr{}, probe.ErrNoGateway }
func (r routeFake) LocalPrefixes() ([]netip.Prefix, error) {
	return []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}, nil
}

// okEcho responde a todo con eco correcto (o pérdida si lossy) y TTL agotado desde 10.9.9.9.
func okEcho(lossy bool) echoFn {
	return func(_ context.Context, r probe.EchoRequest) (probe.Reply, error) {
		if lossy {
			return probe.Reply{Status: probe.StatusTimeout}, nil
		}
		return probe.Reply{Status: probe.StatusOK, From: r.Dst, RTTms: 2}, nil
	}
}

func newProbeApp(t *testing.T, env probe.Env, rt probe.Router, net InventorySource) (*App, *discovery.Table) {
	t.Helper()
	tb := &discovery.Table{SelfID: selfID, Now: time.Now}
	if env.Clock == nil {
		env.Clock = probe.NewFakeClock(time.Unix(0, 0))
	}
	a, err := New(Deps{ConfigDir: t.TempDir(), Table: tb, SelfID: selfID, Probe: env, Router: rt, Net: net,
		SelfIPs:  func() []string { return []string{"192.168.1.5"} },
		Hostname: func() (string, error) { return "PC", nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AddHost("192.168.1.1", "Router"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.CancelProbe(); a.WaitProbe(10 * time.Second) })
	return a, tb
}

const hostRouter = "host:192.168.1.1"

func waitProbe(t *testing.T, a *App, want string) ProbeState {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if p := a.State().Probe; p.Status == want {
			return p
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no se llegó a %q: %+v", want, a.State().Probe)
	return ProbeState{}
}

func wifiNet(sig int) fakeNet {
	ch := 36
	return fakeNet{infos: []netinfo.Info{{
		Adapter: netinfo.Adapter{Name: "Wi-Fi", Kind: netinfo.KindWiFi, Connected: true,
			IPv4: []netip.Addr{netip.MustParseAddr("192.168.1.5")}},
		WiFi: &netinfo.WiFiInfo{Available: true, SSID: "Casa", Channel: &ch, SignalPct: &sig},
	}}}
}

func ethNet() fakeNet {
	return fakeNet{infos: []netinfo.Info{{Adapter: netinfo.Adapter{Name: "Ethernet", Kind: netinfo.KindEthernet,
		Connected: true, IPv4: []netip.Addr{netip.MustParseAddr("192.168.1.5")}}}}}
}

func direct() routeFake {
	return routeFake{rt: probe.Route{IfIndex: 1, Source: netip.MustParseAddr("192.168.1.5"), MTU: 1500}}
}

func TestProbeEveryKindFinishes(t *testing.T) {
	env := probe.Env{Echoer: okEcho(false), Dialer: refuseDialer{}}
	for _, kind := range []string{ProbePing, ProbeTrace, ProbeMTU, ProbePorts} {
		a, _ := newProbeApp(t, env, direct(), ethNet())
		if err := a.StartProbe(kind, hostRouter); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		p := waitProbe(t, a, ProbeDone)
		var m map[string]any
		if err := json.Unmarshal(p.Result, &m); err != nil {
			t.Fatalf("%s: JSON: %v", kind, err)
		}
		if m["command"] != kind || m["format_version"] != float64(1) || p.Running || p.Kind != kind {
			t.Fatalf("%s: %+v %v", kind, p, m)
		}
		if _, err := json.Marshal(a.State()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProbeCombinedWiFiCableHealthy(t *testing.T) {
	cases := []struct {
		name  string
		env   probe.Env
		net   InventorySource
		want  string
		heal  bool
		nilCb bool
	}{
		{"wifi", probe.Env{Echoer: okEcho(true)}, wifiNet(42), "tramo WiFi de este equipo (señal 42 %)", false, false},
		{"cable", probe.Env{Echoer: okEcho(true)}, ethNet(), "problema en el primer tramo (cable", false, false},
		{"sano", probe.Env{Echoer: okEcho(false)}, ethNet(), "enlace sano hasta 192.168.1.1", true, false},
	}
	for _, c := range cases {
		a, _ := newProbeApp(t, c.env, direct(), c.net)
		if err := a.StartProbe(ProbePing, hostRouter); err != nil {
			t.Fatal(err)
		}
		p := waitProbe(t, a, ProbeDone)
		if p.Combined == nil || p.Combined.Healthy != c.heal || !strings.Contains(strings.Join(p.Combined.Messages, "|"), c.want) {
			t.Fatalf("%s: %+v", c.name, p.Combined)
		}
	}
}

func TestProbeCombinedMTUAndTrace(t *testing.T) {
	// Interfaz 1500; el eco con DF solo pasa hasta 1400 bytes IP (carga 1372).
	echo := func(_ context.Context, r probe.EchoRequest) (probe.Reply, error) {
		if r.DF && r.Size > 1400-28 {
			return probe.Reply{Status: probe.StatusFragNeeded}, nil
		}
		return probe.Reply{Status: probe.StatusOK, From: r.Dst, RTTms: 2}, nil
	}
	a, _ := newProbeApp(t, probe.Env{Echoer: echoFn(echo)}, direct(), ethNet())
	if err := a.StartProbe(ProbeMTU, hostRouter); err != nil {
		t.Fatal(err)
	}
	p := waitProbe(t, a, ProbeDone)
	if p.Combined == nil || p.Combined.Healthy || len(p.Combined.Messages) != 1 || !strings.Contains(p.Combined.Messages[0], "MTU reducido en la ruta (1400") {
		t.Fatalf("%+v", p.Combined)
	}
}

func TestProbePingBehindGatewayHasNoCombined(t *testing.T) {
	rt := direct()
	rt.rt.NextHop = netip.MustParseAddr("192.168.1.254")
	a, _ := newProbeApp(t, probe.Env{Echoer: okEcho(false)}, rt, ethNet())
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	if p := waitProbe(t, a, ProbeDone); p.Combined != nil {
		t.Fatalf("combinado inesperado: %+v", p.Combined)
	}
}

func TestProbeNoRouteNoCombined(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{Echoer: okEcho(false)}, routeFake{err: errors.New("no")}, ethNet())
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	if p := waitProbe(t, a, ProbeDone); p.Combined != nil || p.Result == nil {
		t.Fatalf("%+v", p)
	}
}

// blockEcho bloquea hasta que se cancela el contexto.
func blockEcho(entered chan<- struct{}) echoFn {
	var once sync.Once
	return func(ctx context.Context, _ probe.EchoRequest) (probe.Reply, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return probe.Reply{}, ctx.Err()
	}
}

func TestProbeSecondRejectedAndCancel(t *testing.T) {
	entered := make(chan struct{})
	a, _ := newProbeApp(t, probe.Env{Echoer: blockEcho(entered)}, direct(), ethNet())
	release := make(chan struct{})
	a.probeHook = func() { <-release }
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := a.StartProbe(ProbeTrace, hostRouter); err == nil || !strings.Contains(err.Error(), "ya hay un diagnóstico") {
		t.Fatalf("segundo: %v", err)
	}
	if s := a.State(); !s.Probe.Running || s.CanProbe || s.CanTest || s.ProbeBlocked == "" || s.TestBlocked != TestBlockedByProbe {
		t.Fatalf("estado en curso: %+v", s)
	}
	start := time.Now()
	a.CancelProbe()
	if s := a.State().Probe; s.Status != ProbeCancelling || s.Label != "Cancelando…" {
		t.Fatalf("cancelando: %+v", s)
	}
	close(release)
	waitProbe(t, a, ProbeCancelled)
	if !a.WaitProbe(5*time.Second) || time.Since(start) > 5*time.Second {
		t.Fatal("no terminó")
	}
	if s := a.State(); !s.CanProbe || s.Probe.Result != nil {
		t.Fatalf("tras cancelar: %+v", s)
	}
}

func TestProbeAndTestMutuallyExclusive(t *testing.T) {
	entered := make(chan struct{})
	a, tb := newProbeApp(t, probe.Env{Echoer: blockEcho(entered)}, direct(), ethNet())
	announce(tb, idA, "Dest", "10.0.0.2", discovery.StateAvailable, 0)
	if err := a.SelectPeer(idA); err != nil {
		t.Fatal(err)
	}
	// Con un diagnóstico en curso no se inicia un TEST!.
	if err := a.StartProbe(ProbePing, idA); err != nil { // también equipos con NPerf
		t.Fatal(err)
	}
	<-entered
	if err := a.StartTest(); err == nil || err.Error() != TestBlockedByProbe {
		t.Fatalf("StartTest: %v", err)
	}
	a.CancelProbe()
	waitProbe(t, a, ProbeCancelled)
	// Con un TEST! en curso no se inicia un diagnóstico.
	a.SetTest(TestState{Running: true, Status: TestRunning})
	if err := a.StartProbe(ProbePing, idA); err == nil || err.Error() != ProbeBlockedByTest {
		t.Fatalf("StartProbe: %v", err)
	}
	if s := a.State(); s.CanProbe || s.ProbeBlocked != ProbeBlockedByTest {
		t.Fatalf("estado: %+v", s)
	}
}

func TestProbeValidation(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{Echoer: okEcho(false)}, direct(), nil)
	for _, c := range []struct{ kind, id string }{
		{"nmap", hostRouter}, {ProbePing, ""}, {ProbePing, selfID}, {ProbePing, "host:9.9.9.9"}, {ProbePorts, hostRouter},
	} {
		if err := a.StartProbe(c.kind, c.id); err == nil {
			t.Errorf("%+v debía fallar", c)
		}
	}
	if s := a.State(); s.Probe.Status != ProbeIdle || !s.CanProbe {
		t.Fatalf("%+v", s.Probe)
	}
}

func TestProbeResolveErrorReported(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{Echoer: okEcho(false)}, direct(), nil)
	if err := a.AddHost("equipo.local", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.StartProbe(ProbePing, "host:equipo.local"); err != nil {
		t.Fatal(err)
	}
	if p := waitProbe(t, a, ProbeError); p.Error == "" || p.Result != nil {
		t.Fatalf("%+v", p)
	}
}

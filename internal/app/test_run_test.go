package app

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/proto"
	"nperf/internal/report"
	"nperf/internal/server"
	"nperf/internal/testutil"
	"nperf/internal/version"
)

// testEnv levanta un server de test en loopback y una App con ese equipo como destino.
type testEnv struct {
	a       *App
	addr    string
	entered chan struct{}
	free    chan struct{}
}

func newTestEnv(t *testing.T, serverKey, clientKey string, hold bool) *testEnv {
	t.Helper()
	e := &testEnv{entered: make(chan struct{}, 8), free: make(chan struct{}, 8)}
	opts := []server.Option{server.WithKey(serverKey), server.WithMaxPerIP(0)}
	ln, uc := testutil.Listen(t)
	opts = append(opts, server.WithUDPConn(uc))
	if hold {
		opts = append(opts, server.WithTurnHook(func(ctx context.Context) {
			e.entered <- struct{}{}
			select {
			case <-e.free:
			case <-ctx.Done():
			}
		}))
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		testutil.ServeErr(t, ctx, server.New(opts...).Serve(ctx, ln))
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	e.addr = ln.Addr().String()

	a, tb, _, dir := newApp(t)
	if err := config.Save(dir, func() config.Config { c := config.Defaults(); c.Key = clientKey; return c }()); err != nil {
		t.Fatal(err)
	}
	ap := netip.MustParseAddrPort(e.addr)
	tb.Update(discovery.Announce{Alias: "Dest", IP: ap.Addr(), Port: int(ap.Port()), State: discovery.StateAvailable, InstanceID: idA}, netip.AddrPort{})
	if err := a.SelectPeer(idA); err != nil {
		t.Fatal(err)
	}
	a.duration = 1
	e.a = a
	t.Cleanup(func() { a.CancelTest(); a.WaitTest(10 * time.Second) })
	return e
}

func (e *testEnv) waitStatus(t *testing.T, want string) TestState {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if s := e.a.State().Test; s.Status == want {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("estado %q no alcanzado; actual %+v", want, e.a.State().Test)
	return TestState{}
}

func TestStartTestMatchesClientRun(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	if s := e.a.State(); s.CanTest || s.TestBlocked == "" {
		t.Fatalf("durante la prueba can_test debe ser false: %+v", s)
	}
	if err := e.a.StartTest(); err == nil || !strings.Contains(err.Error(), "en curso") {
		t.Fatalf("segunda prueba debe fallar: %v", err)
	}
	got := e.waitStatus(t, TestDone)
	if got.Running || got.Report == nil {
		t.Fatalf("estado final: %+v", got)
	}
	res, err := client.Run(context.Background(), e.addr, client.Options{Duration: time.Second, Proto: proto.ProtoBoth,
		Direction: DirBoth, Key: "k", Streams: proto.DefaultStreams})
	if err != nil {
		t.Fatal(err)
	}
	ap := netip.MustParseAddrPort(e.addr)
	want := report.Build(report.Params{Host: ap.Addr().String(), Port: int(ap.Port()), Proto: proto.ProtoBoth,
		Direction: DirBoth, Duration: time.Second, Streams: proto.DefaultStreams, Version: version.Current}, res)
	r := *got.Report
	if r.Params != want.Params {
		t.Fatalf("params: %+v vs %+v", r.Params, want.Params)
	}
	if r.RTT.Status != want.RTT.Status || r.RTT.Samples != want.RTT.Samples {
		t.Fatalf("rtt: %+v vs %+v", r.RTT, want.RTT)
	}
	for _, c := range [][2]report.JSONDir{{r.TCP.Fwd, want.TCP.Fwd}, {r.TCP.Rev, want.TCP.Rev}, {r.UDP.Fwd, want.UDP.Fwd}, {r.UDP.Rev, want.UDP.Rev}} {
		if c[0].Status != c[1].Status || c[0].Status != report.StatusMeasured || c[0].ThroughputMbps == nil {
			t.Fatalf("direccion: %+v vs %+v", c[0], c[1])
		}
		if (c[0].Retransmission == nil) != (c[1].Retransmission == nil) || (c[0].UDP == nil) != (c[1].UDP == nil) {
			t.Fatalf("campos distintos: %+v vs %+v", c[0], c[1])
		}
	}
	// Terminada la prueba se puede iniciar otra.
	if s := e.a.State(); !s.CanTest {
		t.Fatalf("can_test tras terminar: %+v", s)
	}
}

func TestStartTestQueuedAndCancel(t *testing.T) {
	e := newTestEnv(t, "k", "k", true)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	occ := make(chan struct{})
	go func() {
		defer close(occ)
		client.Run(ctx, e.addr, client.Options{Duration: time.Second, Proto: proto.ProtoTCP, Key: "k", Streams: 1})
	}()
	select {
	case <-e.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("el ocupante no obtuvo turno")
	}
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	s := e.waitStatus(t, TestQueued)
	for s.Position == 0 {
		time.Sleep(10 * time.Millisecond)
		s = e.a.State().Test
	}
	if s.Position != 1 || !s.Running {
		t.Fatalf("cola: %+v", s)
	}
	e.a.CancelTest()
	if !e.a.WaitTest(10 * time.Second) {
		t.Fatal("la goroutine no terminó")
	}
	if s := e.a.State(); s.Test.Status != TestCancelled || s.Test.Running || !s.CanTest {
		t.Fatalf("tras cancelar: %+v", s)
	}
	// Libera al ocupante: el turno no debe quedar tomado por la prueba cancelada.
	e.free <- struct{}{}
	select {
	case <-occ:
	case <-time.After(15 * time.Second):
		t.Fatal("el ocupante no terminó")
	}
	time.Sleep(100 * time.Millisecond)
	if s := e.a.State().Test; s.Status != TestCancelled {
		t.Fatalf("el estado cambió tras cancelar: %+v", s)
	}
	select {
	case <-e.entered:
		t.Fatal("la prueba cancelada obtuvo turno")
	default:
	}
}

func TestStartTestCancelRunning(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	e.a.duration = 30
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	e.waitStatus(t, TestRunning)
	start := time.Now()
	e.a.CancelTest()
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("CancelTest bloqueó %v", d)
	}
	s := e.a.State()
	if s.Test.Status == TestCancelling {
		if !s.Test.Running || s.CanTest || !strings.Contains(s.TestBlocked, "cancelando") || s.Test.Label != "Cancelando…" {
			t.Fatalf("cancelando: %+v", s)
		}
		if err := e.a.StartTest(); err == nil {
			t.Fatal("no debe iniciar durante cancelando")
		}
	}
	if !e.a.WaitTest(10 * time.Second) {
		t.Fatal("goroutine colgada")
	}
	if s := e.a.State().Test; s.Status != TestCancelled || s.Running {
		t.Fatalf("estado: %+v", s)
	}
}

func TestStartTestWrongKey(t *testing.T) {
	e := newTestEnv(t, "k", "otra", false)
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	s := e.waitStatus(t, TestError)
	_, cliErr := client.Run(context.Background(), e.addr, client.Options{Duration: time.Second, Proto: proto.ProtoBoth, Key: "otra"})
	if cliErr == nil || s.Error != cliErr.Error() || s.Running {
		t.Fatalf("error app %q vs CLI %v", s.Error, cliErr)
	}
}

func TestStartTestNoTarget(t *testing.T) {
	a, _, _, _ := newApp(t)
	if err := a.StartTest(); err == nil {
		t.Fatal("sin destino debe fallar")
	}
	if s := a.State().Test; s.Status != TestIdle {
		t.Fatalf("estado: %+v", s)
	}
}

func TestOnPhaseOrder(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	var got []string
	_, err := client.Run(context.Background(), e.addr, client.Options{Duration: time.Second, Proto: proto.ProtoBoth,
		Direction: DirBoth, Key: "k", Streams: 1, OnPhase: func(p string) { got = append(got, p) }})
	if err != nil {
		t.Fatal(err)
	}
	want := "rtt,tcp_fwd,tcp_rev,udp_fwd,udp_rev"
	if strings.Join(got, ",") != want {
		t.Fatalf("fases %v, quiero %s", got, want)
	}
}

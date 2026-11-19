package inet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"

	"nperf/internal/probe"
)

var (
	gw = netip.MustParseAddr("192.168.1.1")
	t1 = netip.MustParseAddr("1.1.1.1")
	t2 = netip.MustParseAddr("8.8.8.8")
)

// fakeEcho responde según un guion por destino; TTL limitado = salto de traza.
type fakeEcho struct {
	mu     sync.Mutex
	rtt    map[netip.Addr]float64 // <0: sin respuesta
	calls  map[netip.Addr]int
	ttlReq int
	block  bool
}

func (f *fakeEcho) Echo(ctx context.Context, req probe.EchoRequest) (probe.Reply, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[netip.Addr]int{}
	}
	f.calls[req.Dst]++
	if req.TTL > 0 {
		f.ttlReq++
	}
	rtt, ok := f.rtt[req.Dst]
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return probe.Reply{}, ctx.Err()
	}
	if !ok || rtt < 0 {
		return probe.Reply{Status: probe.StatusTimeout, Reason: "tiempo de espera agotado"}, nil
	}
	return probe.Reply{Status: probe.StatusOK, RTTms: rtt, From: req.Dst}, nil
}

type fakeDial struct {
	mu    sync.Mutex
	calls int
	ok    bool
}

func (d *fakeDial) DialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	if !d.ok {
		return nil, errors.New("rechazado")
	}
	a, b := net.Pipe()
	b.Close()
	return a, nil
}

func env(e *fakeEcho, d *fakeDial) probe.Env {
	return probe.Env{Echoer: e, Dialer: d, Clock: probe.NewFakeClock(time.Unix(0, 0))}
}

var fast = probe.PingOptions{Count: 3, Interval: 200 * time.Millisecond, Timeout: time.Second}

func run(t *testing.T, e *fakeEcho, d *fakeDial, g netip.Addr) Outcome {
	t.Helper()
	o, err := RunWith(context.Background(), env(e, d), g, []netip.Addr{t1, t2}, fast)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestRunVerdicts(t *testing.T) {
	cases := []struct {
		name  string
		rtt   map[netip.Addr]float64
		gw    netip.Addr
		tcpOK bool
		want  Kind
		notes int
		tcp   bool
		trace bool
	}{
		{"sano", map[netip.Addr]float64{gw: 2, t1: 10, t2: 12}, gw, false, Healthy, 0, false, false},
		{"proveedor", map[netip.Addr]float64{gw: 2, t1: 150, t2: 12}, gw, false, Provider, 0, false, true},
		{"lan", map[netip.Addr]float64{gw: 50, t1: 10, t2: 12}, gw, false, LAN, 0, false, false},
		{"sin salida", map[netip.Addr]float64{gw: 2}, gw, false, NoInternet, 0, true, false},
		{"icmp filtrado", map[netip.Addr]float64{gw: 2}, gw, true, ICMPFiltered, 0, true, false},
		{"gateway mudo, internet sano", map[netip.Addr]float64{t1: 10, t2: 12}, gw, false, Healthy, 1, false, false},
		{"sin gateway, proveedor", map[netip.Addr]float64{t1: 150, t2: 150}, netip.Addr{}, false, Provider, 1, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &fakeEcho{rtt: c.rtt}
			d := &fakeDial{ok: c.tcpOK}
			o := run(t, e, d, c.gw)
			if o.Verdict.Kind != c.want {
				t.Fatalf("veredicto %s, quería %s", o.Verdict.Kind, c.want)
			}
			if len(o.Verdict.Notes) != c.notes {
				t.Errorf("notas %v", o.Verdict.Notes)
			}
			if (d.calls > 0) != c.tcp {
				t.Errorf("TCP 443: %d llamadas, esperado %v", d.calls, c.tcp)
			}
			if (o.Trace != nil) != c.trace || (e.ttlReq > 0) != c.trace {
				t.Errorf("traceroute: trace=%v ttlReq=%d, esperado %v", o.Trace != nil, e.ttlReq, c.trace)
			}
		})
	}
}

func TestRunTraceTargetsDegraded(t *testing.T) {
	e := &fakeEcho{rtt: map[netip.Addr]float64{gw: 2, t1: 10, t2: 150}}
	o := run(t, e, &fakeDial{}, gw)
	if o.Verdict.Kind != Provider || o.Trace == nil || o.Trace.Dst != t2 {
		t.Fatalf("%+v", o)
	}
}

func TestRunCancelFast(t *testing.T) {
	before := runtime.NumGoroutine()
	e := &fakeEcho{block: true}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := RunWith(ctx, env(e, &fakeDial{}), gw, []netip.Addr{t1, t2}, fast)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
			t.Fatalf("err=%v en %v", err, time.Since(start))
		}
	case <-time.After(time.Second):
		t.Fatal("no cancela en 1 s")
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("fuga de goroutines: %d > %d", n, before)
	}
}

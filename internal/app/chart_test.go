package app

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
)

func TestChartLiveAndAfterDone(t *testing.T) {
	e := newTestEnv(t, "secreto-k", "secreto-k", false)
	e.a.duration = 2
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	sawLive := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && !sawLive {
		s := e.a.State().Test
		if s.Running && s.Chart != nil {
			for _, p := range s.Chart.Phases {
				if len(p.Samples) > 0 {
					sawLive = true
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawLive {
		t.Fatal("sin muestras durante la prueba")
	}
	got := e.waitStatus(t, TestDone)
	if got.Chart == nil || len(got.Chart.Phases) != 4 || got.Chart.Cancelled {
		t.Fatalf("chart final: %+v", got.Chart)
	}
	for _, p := range got.Chart.Phases {
		if !p.Available || len(p.Samples) == 0 || p.RTTInitial == nil || p.NominalMbps != nil {
			t.Fatalf("fase %s: %+v", p.Phase, p)
		}
		if !strings.Contains(p.Label, "Dest") {
			t.Fatalf("etiqueta sin alias: %q", p.Label)
		}
		udp := strings.HasPrefix(p.Phase, "udp")
		if s := p.Samples[0]; s[0] == nil || s[1] == nil || s[2] == nil || (s[4] != nil) != udp || (s[5] != nil) != udp {
			t.Fatalf("muestra %s: %+v", p.Phase, s)
		}
	}
	b, err := json.Marshal(e.a.State())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secreto-k") {
		t.Fatal("la clave aparece en el estado")
	}
	// El siguiente TEST! descarta la gráfica anterior.
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	if c := e.a.State().Test.Chart; c != nil && len(c.Phases) > 0 && len(c.Phases[0].Samples) > 1 {
		t.Fatalf("chart anterior no descartado: %+v", c)
	}
}

func TestChartCancelledKept(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	e.a.duration = 5
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if c := e.a.State().Test.Chart; c != nil && len(c.Phases) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.a.CancelTest()
	got := e.waitStatus(t, TestCancelled)
	if got.Chart == nil || !got.Chart.Cancelled || len(got.Chart.Phases) == 0 {
		t.Fatalf("chart cancelado: %+v", got.Chart)
	}
}

func TestChartNominalOldPeerCapAndRace(t *testing.T) {
	c := &Chart{local: "A", peer: "B"}
	c.ready(&proto.Link{Kind: "eth", TxBps: 1e9, RxBps: 1e9}, &proto.Link{Kind: "eth", TxBps: 100e6, RxBps: 100e6})
	if n := c.get("tcp_fwd").NominalMbps; n == nil || *n != 100 {
		t.Fatalf("nominal fwd: %v", n)
	}
	if n := c.get("tcp_rev").NominalMbps; n == nil || *n != 100 {
		t.Fatalf("nominal rev: %v", n)
	}
	if l := c.get("udp_rev").Label; l != "UDP B → A" {
		t.Fatalf("etiqueta: %q", l)
	}
	for i := 0; i < maxChartSamples+50; i++ {
		c.addInterval("tcp_fwd", client.Interval{Index: i, Duration: time.Second, Mbps: 1})
	}
	if n := len(c.get("tcp_fwd").Samples); n != maxChartSamples {
		t.Fatalf("tope: %d", n)
	}
	// Par antiguo: medido sin Live -> no disponible.
	o := &Chart{local: "A", peer: "B"}
	o.finish(client.Result{Fwd: client.Dir{Measured: true}}, nil)
	if p := o.get("tcp_fwd"); p.Available || p.LoadedRTT != nil || p.NominalMbps != nil || p.RTTInitial != nil {
		t.Fatalf("par antiguo: %+v", p)
	}
	// Copia independiente y accesos concurrentes.
	cp := c.clone()
	cp.Phases[0].Samples[0] = ChartSample{}
	if c.Phases[0].Samples[0][2] == nil {
		t.Fatal("clone comparte muestras")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			mu.Lock()
			c.addInterval("udp_fwd", client.Interval{Index: i, UDP: true})
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			mu.Lock()
			_ = c.clone()
			mu.Unlock()
		}
	}()
	wg.Wait()
}

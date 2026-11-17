package app

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nperf/internal/discovery"
	"nperf/internal/netinfo"
)

type fakeCounters struct {
	calls atomic.Int32
	mu    sync.Mutex
	in    uint64
}

func (f *fakeCounters) Counters() ([]netinfo.IfCounter, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.in += 1000
	return []netinfo.IfCounter{{GUID: "{A}", In: f.in, Out: f.in, Up: true}}, nil
}

type fakeInv struct{}

func (fakeInv) Inventory() ([]netinfo.Info, error) {
	return []netinfo.Info{{GUID: "{A}", Name: "Ethernet", Kind: netinfo.KindEthernet}}, nil
}

func newMonApp(t *testing.T, c netinfo.CounterSource, clk *time.Time, mu *sync.Mutex) *App {
	t.Helper()
	a, err := New(Deps{Table: &discovery.Table{SelfID: "x", Now: time.Now}, Net: fakeInv{}, Counters: c,
		Now: func() time.Time { mu.Lock(); defer mu.Unlock(); return *clk }})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestMonitorViewAndStop(t *testing.T) {
	clk := time.Unix(1000, 0)
	var mu sync.Mutex
	f := &fakeCounters{}
	a := newMonApp(t, f, &clk, &mu)
	var v MonitorView
	for i := 0; i < 3; i++ {
		mu.Lock()
		clk = clk.Add(time.Second)
		mu.Unlock()
		var err error
		if v, err = a.Monitor(true); err != nil {
			t.Fatal(err)
		}
	}
	if !v.Available || len(v.Adapters) != 1 || len(v.Adapters[0].Points) != 2 || v.Adapters[0].Status != "up" {
		t.Fatalf("vista: %+v", v)
	}
	if v.Adapters[0].Points[0].InBps != 8000 {
		t.Fatalf("tasa: %v", v.Adapters[0].Points[0].InBps)
	}
	if _, err := json.Marshal(v); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Monitor(false); err != nil {
		t.Fatal(err)
	}
	n := f.calls.Load()
	mu.Lock()
	clk = clk.Add(time.Second)
	mu.Unlock()
	a.StopMonitor()
	if f.calls.Load() != n {
		t.Fatal("consultó tras parar")
	}
}

func TestMonitorUnavailable(t *testing.T) {
	a, err := New(Deps{Table: &discovery.Table{SelfID: "x", Now: time.Now}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := a.Monitor(true)
	if err != nil || v.Available || v.Message == "" {
		t.Fatalf("%+v %v", v, err)
	}
	a.StopMonitor()
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "key") {
		t.Fatal("clave")
	}
}

func TestMonitorConcurrent(t *testing.T) {
	clk := time.Unix(1000, 0)
	var mu sync.Mutex
	a := newMonApp(t, &fakeCounters{}, &clk, &mu)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				mu.Lock()
				clk = clk.Add(100 * time.Millisecond)
				mu.Unlock()
				a.Monitor(j%7 != 0)
				if i == 0 {
					a.StopMonitor()
				}
			}
		}(i)
	}
	wg.Wait()
}

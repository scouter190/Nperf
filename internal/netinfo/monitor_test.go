package netinfo

import (
	"errors"
	"testing"
	"time"
)

type fakeCounters struct {
	calls int
	cs    []IfCounter
	err   error
}

func (f *fakeCounters) Counters() ([]IfCounter, error) { f.calls++; return f.cs, f.err }

type monEnv struct {
	t    *testing.T
	now  time.Time
	fc   *fakeCounters
	invN int
	inv  []Info
	m    *Monitor
}

func newMonEnv(t *testing.T) *monEnv {
	e := &monEnv{t: t, now: time.Unix(1000, 0), fc: &fakeCounters{}}
	e.inv = []Info{
		{Adapter: Adapter{Name: "Ethernet", GUID: "{A}", Kind: KindEthernet}},
		{Adapter: Adapter{Name: "vEthernet", GUID: "{V}", Kind: KindOther, Virtual: true}},
	}
	e.m = NewMonitor(e.fc, func() ([]Info, error) { e.invN++; return e.inv, nil }, func() time.Time { return e.now })
	return e
}

// step avanza d, renueva el permiso y muestrea con los contadores dados.
func (e *monEnv) step(d time.Duration, cs ...IfCounter) {
	e.now = e.now.Add(d)
	e.fc.cs = cs
	e.m.Touch()
	e.m.Step(e.now)
}

func ctr(g string, in, out uint64) IfCounter { return IfCounter{GUID: g, In: in, Out: out, Up: true} }

func TestMonitorRate(t *testing.T) {
	e := newMonEnv(t)
	e.step(0, ctr("{A}", 0, 0), ctr("{V}", 5, 5))
	e.step(time.Second, ctr("{A}", 125000, 250000), ctr("{V}", 9, 9))
	s := e.m.Snapshot()
	if len(s) != 1 || s[0].GUID != "{A}" {
		t.Fatalf("virtual no excluido: %+v", s)
	}
	if len(s[0].Points) != 1 || s[0].Points[0].InBps != 1e6 || s[0].Points[0].OutBps != 2e6 || s[0].Status != StatusUp {
		t.Fatalf("tasa: %+v", s[0])
	}
	e.step(2*time.Second, ctr("{A}", 125000+250000, 250000))
	if p := e.m.Snapshot()[0].Points; len(p) != 2 || p[1].InBps != 1e6 || p[1].OutBps != 0 || p[1].Gap {
		t.Fatalf("dt 2 s: %+v", p)
	}
}

func TestMonitorDiscards(t *testing.T) {
	cases := []struct {
		name  string
		d     time.Duration
		next  []IfCounter
		wantN int
	}{
		{"retroceso entrada", time.Second, []IfCounter{ctr("{A}", 50, 200)}, 0},
		{"retroceso salida", time.Second, []IfCounter{ctr("{A}", 200, 50)}, 0},
		{"hueco > 3 s", 4 * time.Second, []IfCounter{ctr("{A}", 200, 200)}, 0},
		{"hueco = 3 s", 3 * time.Second, []IfCounter{ctr("{A}", 200, 200)}, 1},
		{"ausente", time.Second, nil, 0},
		{"caido", time.Second, []IfCounter{{GUID: "{A}", In: 200, Out: 200}}, 0},
		{"guid en minusculas", time.Second, []IfCounter{ctr("{a}", 200, 200)}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newMonEnv(t)
			e.step(0, ctr("{A}", 100, 100))
			e.step(c.d, c.next...)
			if n := len(e.m.Snapshot()[0].Points); n != c.wantN {
				t.Fatalf("puntos = %d, quiero %d", n, c.wantN)
			}
		})
	}
}

func TestMonitorAppearDisappear(t *testing.T) {
	e := newMonEnv(t)
	e.step(0, ctr("{A}", 0, 0))
	e.step(time.Second, ctr("{A}", 100, 100))
	// Cae: estado desconectado, la serie se corta.
	e.step(time.Second)
	if s := e.m.Snapshot()[0]; s.Status != StatusDown || len(s.Points) != 1 {
		t.Fatalf("caido: %+v", s)
	}
	// Vuelve: el primer muestreo solo fija la base; el siguiente lleva Gap.
	e.step(time.Second, ctr("{A}", 10, 10))
	if s := e.m.Snapshot()[0]; s.Status != StatusUp || len(s.Points) != 1 {
		t.Fatalf("base: %+v", s)
	}
	e.step(time.Second, ctr("{A}", 20, 20))
	if p := e.m.Snapshot()[0].Points; len(p) != 2 || !p[1].Gap {
		t.Fatalf("gap: %+v", p)
	}
	// Aparece un adaptador nuevo y otro desaparece del inventario.
	e.inv = append(e.inv, Info{Adapter: Adapter{Name: "Wi-Fi", GUID: "{W}", Kind: KindWiFi}})
	e.step(time.Second, ctr("{A}", 30, 30), ctr("{W}", 1, 1))
	if s := e.m.Snapshot(); len(s) != 2 || s[1].GUID != "{W}" || len(s[1].Points) != 0 {
		t.Fatalf("nuevo: %+v", s)
	}
	e.inv = e.inv[2:]
	e.step(time.Second, ctr("{W}", 2, 2))
	if s := e.m.Snapshot(); len(s) != 1 || s[0].GUID != "{W}" {
		t.Fatalf("desaparecido: %+v", s)
	}
}

func TestMonitorWindowAndCap(t *testing.T) {
	e := newMonEnv(t)
	var n uint64
	e.step(0, ctr("{A}", 0, 0))
	for i := 0; i < 1000; i++ { // 1000 s a 1 Hz
		n += 1000
		e.step(time.Second, ctr("{A}", n, n))
	}
	p := e.m.Snapshot()[0].Points
	if len(p) != MonitorMax {
		t.Fatalf("tope: %d", len(p))
	}
	if e.now.Sub(p[0].At) > MonitorWindow {
		t.Fatalf("fuera de ventana")
	}
	// Con pasos de 3 s manda la ventana de 5 min (100 puntos), no el tope.
	e = newMonEnv(t)
	e.step(0, ctr("{A}", 0, 0))
	for i := 0; i < 500; i++ {
		n += 1000
		e.step(3*time.Second, ctr("{A}", n, n))
	}
	p = e.m.Snapshot()[0].Points
	if len(p) != 101 || e.now.Sub(p[0].At) > MonitorWindow {
		t.Fatalf("ventana: %d puntos", len(p))
	}
	if c := cap(e.m.states["{a}"].series.Points); c > 2*MonitorMax {
		t.Fatalf("memoria sin acotar: cap %d", c)
	}
}

func TestMonitorPermit(t *testing.T) {
	e := newMonEnv(t)
	e.m.Step(e.now) // sin Touch
	if e.fc.calls != 0 || e.invN != 0 {
		t.Fatal("consultó sin permiso")
	}
	e.m.Touch()
	e.m.Step(e.now.Add(MonitorPermit - time.Millisecond))
	if e.fc.calls != 1 || e.invN != 1 {
		t.Fatal("no consultó con permiso")
	}
	e.m.Step(e.now.Add(MonitorPermit)) // vencido
	if e.fc.calls != 1 || e.invN != 1 {
		t.Fatal("consultó con el permiso vencido")
	}
	e.m.Touch()
	e.m.Stop()
	e.m.Step(e.now)
	if e.fc.calls != 1 || e.invN != 1 {
		t.Fatal("consultó tras Stop")
	}
}

func TestMonitorErrors(t *testing.T) {
	e := newMonEnv(t)
	e.step(0, ctr("{A}", 0, 0))
	e.fc.err = errors.New("fallo")
	e.step(time.Second)
	if s := e.m.Snapshot(); len(s) != 1 || s[0].Status != StatusUp {
		t.Fatalf("un error de la fuente no debe alterar las series: %+v", s)
	}
}

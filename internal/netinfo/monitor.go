package netinfo

import (
	"strings"
	"sync"
	"time"
)

// Parámetros del monitor de tarjetas.
const (
	MonitorWindow = 5 * time.Minute // ventana de la serie
	MonitorMax    = 300             // tope de puntos por adaptador
	MonitorMaxGap = 3 * time.Second // hueco máximo entre muestras
	MonitorPermit = 3 * time.Second // vigencia de cada Touch
	StatusUp      = "up"
	StatusDown    = "disconnected"
)

// RatePoint es la tasa media entre dos muestras consecutivas.
type RatePoint struct {
	At     time.Time
	InBps  float64
	OutBps float64
	// Gap: antes de este punto hubo un corte (la serie no debe unirse al anterior).
	Gap bool
}

// MonitorSeries es la serie de un adaptador físico.
type MonitorSeries struct {
	GUID   string
	Name   string
	Kind   Kind
	Status string // StatusUp o StatusDown
	Points []RatePoint
}

type ifState struct {
	series  MonitorSeries
	hasBase bool
	baseAt  time.Time
	in, out uint64
	cut     bool // el próximo punto lleva Gap
}

// Monitor calcula tasas por adaptador. Solo consulta mientras haya permiso vigente
// (Touch); sin renovarlo, o tras Stop, no toca ninguna fuente.
type Monitor struct {
	src CounterSource
	inv func() ([]Info, error)
	now func() time.Time

	mu     sync.Mutex
	until  time.Time
	order  []string
	states map[string]*ifState
}

// NewMonitor crea el monitor; inv entrega el inventario (para excluir virtuales) y
// now es el reloj (nil = time.Now).
func NewMonitor(src CounterSource, inv func() ([]Info, error), now func() time.Time) *Monitor {
	if now == nil {
		now = time.Now
	}
	return &Monitor{src: src, inv: inv, now: now, states: map[string]*ifState{}}
}

// Touch concede (o renueva) el permiso de muestreo durante MonitorPermit.
func (m *Monitor) Touch() {
	m.mu.Lock()
	m.until = m.now().Add(MonitorPermit)
	m.mu.Unlock()
}

// Stop retira el permiso y olvida las bases: al reanudar el primer punto solo fija la base.
func (m *Monitor) Stop() {
	m.mu.Lock()
	m.until = time.Time{}
	for _, s := range m.states {
		s.hasBase = false
		s.cut = true
	}
	m.mu.Unlock()
}

// Active indica si hay permiso vigente en el instante now.
func (m *Monitor) Active(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return now.Before(m.until)
}

// Step toma una muestra en now si el permiso sigue vigente; si no, no consulta nada.
func (m *Monitor) Step(now time.Time) {
	m.mu.Lock()
	ok := now.Before(m.until)
	m.mu.Unlock()
	if !ok {
		return
	}
	infos, err := m.inv()
	if err != nil {
		return
	}
	cs, err := m.src.Counters()
	if err != nil {
		return
	}
	byGUID := make(map[string]IfCounter, len(cs))
	for _, c := range cs {
		byGUID[strings.ToLower(c.GUID)] = c
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !now.Before(m.until) { // Stop llegó durante las consultas
		return
	}
	seen := map[string]bool{}
	order := make([]string, 0, len(infos))
	for _, in := range infos {
		if in.Virtual || in.GUID == "" {
			continue
		}
		key := strings.ToLower(in.GUID)
		if seen[key] {
			continue
		}
		seen[key] = true
		order = append(order, key)
		s := m.states[key]
		if s == nil {
			s = &ifState{}
			m.states[key] = s
		}
		s.series.GUID, s.series.Name, s.series.Kind = in.GUID, in.Name, in.Kind
		c, found := byGUID[key]
		if !found || !c.Up {
			s.series.Status = StatusDown
			s.hasBase = false
			s.cut = true
			continue
		}
		s.series.Status = StatusUp
		s.sample(now, c)
	}
	for k := range m.states {
		if !seen[k] {
			delete(m.states, k)
		}
	}
	m.order = order
}

func (s *ifState) sample(now time.Time, c IfCounter) {
	defer func() { s.hasBase, s.baseAt, s.in, s.out = true, now, c.In, c.Out }()
	if !s.hasBase {
		return
	}
	dt := now.Sub(s.baseAt)
	if dt <= 0 || dt > MonitorMaxGap || c.In < s.in || c.Out < s.out {
		s.cut = true
		return
	}
	sec := dt.Seconds()
	s.series.Points = append(s.series.Points, RatePoint{
		At: now, InBps: float64(c.In-s.in) * 8 / sec, OutBps: float64(c.Out-s.out) * 8 / sec, Gap: s.cut,
	})
	s.cut = false
	// Ventana de 5 min y tope de puntos; se recorta copiando para liberar memoria.
	p := s.series.Points
	drop := 0
	for drop < len(p) && now.Sub(p[drop].At) > MonitorWindow {
		drop++
	}
	if len(p)-drop > MonitorMax {
		drop = len(p) - MonitorMax
	}
	if drop > 0 {
		s.series.Points = append(make([]RatePoint, 0, len(p)-drop+1), p[drop:]...)
	}
}

// Snapshot devuelve una copia de las series, en el orden del inventario.
func (m *Monitor) Snapshot() []MonitorSeries {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MonitorSeries, 0, len(m.order))
	for _, k := range m.order {
		s := m.states[k]
		if s == nil {
			continue
		}
		c := s.series
		c.Points = append([]RatePoint(nil), s.series.Points...)
		out = append(out, c)
	}
	return out
}

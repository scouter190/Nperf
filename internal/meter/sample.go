package meter

import (
	"math"
	"time"
)

// IntervalWidth es el ancho nominal de cada ventana de muestreo.
const IntervalWidth = time.Second

// MaxSamples es la capacidad del canal de muestras (300 s de fase máxima).
const MaxSamples = 300

// Sample es una ventana cerrada de una fase.
type Sample struct {
	Index    int           // 0..n-1
	Start    time.Duration // inicio respecto al primer byte o datagrama
	Duration time.Duration // duración real de la ventana
	Bytes    int64

	// Solo UDP.
	UDP      bool
	Received int64
	Expected int64
	Lost     int64         // máx(0, Expected - Received)
	Jitter   time.Duration // RFC 3550 al cerrar la ventana
}

// Mbps devuelve la tasa de la ventana en megabits por segundo.
func (s Sample) Mbps() float64 {
	return Result{Bytes: s.Bytes, Elapsed: s.Duration}.Mbps()
}

// LossPercent devuelve la pérdida de la ventana en porcentaje.
func (s Sample) LossPercent() float64 {
	if s.Expected <= 0 {
		return 0
	}
	return float64(s.Lost) * 100 / float64(s.Expected)
}

// Series reparte una fase de duración d en ⌈d⌉ ventanas de 1 s desde el primer byte o
// datagrama. La última queda abierta hasta Finish y lleva su duración real. No es
// segura para uso concurrente: un único goroutine la alimenta (el reloj va por parámetro).
// Una ventana se cierra en el primer "now" que alcanza su borde; si el aviso llega tarde,
// la ventana lleva su duración efectiva y la suma de bytes no cambia.
type Series struct {
	n       int
	out     chan<- Sample
	dropped int

	started bool
	first   time.Time
	idx     int
	winFrom time.Time // inicio efectivo de la ventana abierta
	bytes   int64
	done    bool

	// UDP.
	udp       bool
	received  int64
	haveSeq   bool
	firstSeq  uint64
	maxSeq    uint64
	expClosed int64 // esperados acumulados al último cierre
	haveTr    bool
	prevTr    time.Duration
	jitter    float64 // ns
}

// NewSeries crea la serie de ⌈d⌉ ventanas (mínimo 1). Cada muestra cerrada se entrega a out
// sin bloquear: si el canal está lleno se descarta y se cuenta (Dropped). out puede ser nil.
func NewSeries(d time.Duration, out chan<- Sample) *Series {
	n := int(math.Ceil(float64(d) / float64(IntervalWidth)))
	if n < 1 {
		n = 1
	}
	return &Series{n: n, out: out}
}

// Len devuelve el número de ventanas.
func (s *Series) Len() int { return s.n }

// Dropped devuelve las muestras descartadas por canal lleno.
func (s *Series) Dropped() int { return s.dropped }

func (s *Series) edge(i int) time.Time { return s.first.Add(time.Duration(i+1) * IntervalWidth) }

// Observe avanza el reloj: cierra las ventanas (salvo la última) cuyo borde ya pasó.
// Antes del primer byte no hace nada.
func (s *Series) Observe(now time.Time) {
	if !s.started || s.done {
		return
	}
	for s.idx < s.n-1 && !now.Before(s.edge(s.idx)) {
		end := s.edge(s.idx)
		if now.Before(s.edge(s.idx + 1)) {
			end = now // última cruzada: cierra en el borde efectivo
		}
		s.close(end)
	}
}

func (s *Series) begin(now time.Time) {
	if !s.started {
		s.started = true
		s.first = now
		s.winFrom = now
	}
}

// Add suma bytes (TCP) recibidos en now.
func (s *Series) Add(now time.Time, n int64) {
	s.begin(now)
	s.Observe(now)
	s.bytes += n
}

// Packet registra un datagrama de datos (no repetido) de size bytes, con su número de
// secuencia y su tránsito (recepción - envío) para el jitter.
func (s *Series) Packet(now time.Time, size int, seq uint64, transit time.Duration) {
	s.udp = true
	s.begin(now)
	s.Observe(now)
	s.bytes += int64(size)
	s.received++
	if !s.haveSeq {
		s.haveSeq, s.firstSeq, s.maxSeq = true, seq, seq
	} else if seq > s.maxSeq {
		s.maxSeq = seq
	}
	if s.haveTr {
		d := transit - s.prevTr
		if d < 0 {
			d = -d
		}
		s.jitter += (float64(d) - s.jitter) / 16
	}
	s.haveTr, s.prevTr = true, transit
}

// Finish cierra la ventana final en now (fin real de la fase) tras cerrar las pendientes.
// Sin ningún byte previo no entrega nada. Es idempotente.
func (s *Series) Finish(now time.Time) {
	if !s.started || s.done {
		return
	}
	s.Observe(now)
	end := now
	if end.Before(s.winFrom) {
		end = s.winFrom
	}
	// Si la fase terminó antes de la última ventana, las faltantes salen vacías.
	for s.idx < s.n-1 {
		s.close(s.winFrom)
	}
	s.close(end)
	s.done = true
}

func (s *Series) close(end time.Time) {
	smp := Sample{
		Index:    s.idx,
		Start:    s.winFrom.Sub(s.first),
		Duration: end.Sub(s.winFrom),
		Bytes:    s.bytes,
	}
	if s.udp {
		smp.UDP = true
		smp.Received = s.received
		if s.haveSeq {
			cum := int64(s.maxSeq-s.firstSeq) + 1
			smp.Expected = cum - s.expClosed
			s.expClosed = cum
		}
		if smp.Expected > smp.Received {
			smp.Lost = smp.Expected - smp.Received
		}
		smp.Jitter = time.Duration(s.jitter)
	}
	if s.out != nil {
		select {
		case s.out <- smp:
		default:
			s.dropped++
		}
	}
	s.idx++
	s.winFrom = end
	s.bytes, s.received = 0, 0
}

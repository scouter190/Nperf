package probe

import (
	"fmt"
	"math"
)

// Stats resume una serie de ecos. Con Received == 0 los RTT no son
// medibles (RTTOK false); con menos de 2 respuestas el jitter no está
// disponible (JitterOK false). Nunca se devuelve 0 en lugar de "no medido".
type Stats struct {
	Sent      int
	Received  int
	LossPct   float64
	RTTOK     bool
	MinMs     float64
	AvgMs     float64
	MaxMs     float64
	JitterOK  bool
	JitterMs  float64
	LastError string // motivo de la última respuesta de error, si hubo
}

// NewStats calcula las estadísticas de las respuestas, en orden de envío.
// Solo StatusOK cuenta como recibida; el resto es pérdida con su motivo.
func NewStats(replies []Reply) Stats {
	s := Stats{Sent: len(replies)}
	var sum, jsum, prev float64
	havePrev := false
	jn := 0
	for _, r := range replies {
		if r.Status != StatusOK {
			if r.Reason != "" {
				s.LastError = r.Reason
			}
			continue
		}
		s.Received++
		sum += r.RTTms
		if s.Received == 1 || r.RTTms < s.MinMs {
			s.MinMs = r.RTTms
		}
		if s.Received == 1 || r.RTTms > s.MaxMs {
			s.MaxMs = r.RTTms
		}
		if havePrev {
			jsum += math.Abs(r.RTTms - prev)
			jn++
		}
		prev, havePrev = r.RTTms, true
	}
	if s.Sent > 0 {
		s.LossPct = float64(s.Sent-s.Received) * 100 / float64(s.Sent)
	}
	if s.Received > 0 {
		s.RTTOK = true
		s.AvgMs = sum / float64(s.Received)
	}
	if jn > 0 {
		s.JitterOK = true
		s.JitterMs = jsum / float64(jn)
	}
	return s
}

// FormatMs da formato a una latencia; por debajo de 1 ms (resolución de
// Windows) muestra "< 1 ms".
func FormatMs(ms float64) string {
	if ms < 1 {
		return "< 1 ms"
	}
	return fmt.Sprintf("%.1f ms", ms)
}

// FormatJitter da formato al jitter o "no disponible".
func (s Stats) FormatJitter() string {
	if !s.JitterOK {
		return "no disponible"
	}
	return FormatMs(s.JitterMs)
}

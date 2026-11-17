package client

import (
	"fmt"
	"sort"
	"time"
)

// BufferbloatThreshold es el aumento de la mediana bajo carga sobre el RTT medio inicial
// que dispara la nota (supuesto de la spec 005, ajustable tras la validación).
const BufferbloatThreshold = 100 * time.Millisecond

// LoadedRTT resume el RTT bajo carga de un sentido sobre las sondas respondidas.
type LoadedRTT struct {
	Min, Avg, Median, Max time.Duration
	Samples               int // sondas respondidas
	Sent                  int // sondas enviadas (incluye sin respuesta)
}

// NewLoadedRTT calcula la estadística; devuelve nil sin sondas respondidas.
func NewLoadedRTT(answered []time.Duration, sent int) *LoadedRTT {
	if len(answered) == 0 {
		return nil
	}
	s := append([]time.Duration(nil), answered...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum time.Duration
	for _, d := range s {
		sum += d
	}
	n := len(s)
	med := s[n/2]
	if n%2 == 0 {
		med = (s[n/2-1] + s[n/2]) / 2
	}
	return &LoadedRTT{Min: s[0], Avg: sum / time.Duration(n), Median: med, Max: s[n-1], Samples: n, Sent: sent}
}

// BufferbloatNote devuelve la nota si la mediana bajo carga >= RTT medio inicial + 100 ms;
// vacío si no aplica o faltan datos. dir es el sentido ("ida", "vuelta"...).
func BufferbloatNote(initialAvg time.Duration, l *LoadedRTT, dir string) string {
	if l == nil || l.Samples == 0 || initialAvg <= 0 {
		return ""
	}
	rise := l.Median - initialAvg
	if rise < BufferbloatThreshold {
		return ""
	}
	return fmt.Sprintf("la latencia sube %d ms con carga en %s: colas llenas en el enlace (bufferbloat)",
		rise.Milliseconds(), dir)
}

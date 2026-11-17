package proto

import (
	"errors"
	"fmt"
	"math"
)

// MaxSamples es el máximo de muestras por fase (duración máxima 300 s, ventanas de 1 s).
const MaxSamples = 300

// Cotas de validación de una muestra recibida.
const (
	maxSampleMbps       = 100000 // 100 Gbps
	maxSampleMS         = 60000  // jitter y RTT
	maxSampleDurMS      = 300000
	maxSampleBytesPerMS = 12500000 // 100 Gbps en bytes por ms
)

// Sample es una muestra de un intervalo recibida por el control (spec 005).
type Sample struct {
	Index      int
	TMS        int64 // inicio del intervalo desde el primer byte, ms
	DurationMS int64
	Bytes      int64
	Mbps       float64
	Received   int64 // solo UDP
	Expected   int64
	Lost       int64
	LossPct    float64
	JitterMS   float64
}

// SampleMessage convierte una muestra en mensaje "sample".
func SampleMessage(s Sample) Message {
	return Message{Type: TypeSample, Index: s.Index, TMS: s.TMS, DurationMS: s.DurationMS,
		Bytes: s.Bytes, Mbps: s.Mbps, Received: s.Received, Expected: s.Expected,
		Lost: s.Lost, LossPct: s.LossPct, JitterMS: s.JitterMS}
}

// SampleFromMessage valida un "sample" recibido de la red. last es el índice de la muestra
// anterior (-1 si es la primera) y durationMS la duración pedida de la fase: el índice debe
// ser mayor que last y menor que min(⌈D⌉, 300). Nada puede ser negativo ni no finito.
func SampleFromMessage(m Message, last int, durationMS int64) (Sample, error) {
	if m.Type != TypeSample {
		return Sample{}, fmt.Errorf("mensaje %q no es una muestra", m.Type)
	}
	if durationMS <= 0 {
		return Sample{}, errors.New("muestra inválida: duración de fase no válida")
	}
	max := (durationMS + 999) / 1000
	if max > MaxSamples {
		max = MaxSamples
	}
	if m.Index < 0 || int64(m.Index) >= max {
		return Sample{}, fmt.Errorf("muestra inválida: índice %d fuera de rango", m.Index)
	}
	if m.Index <= last {
		return Sample{}, fmt.Errorf("muestra inválida: índice %d no es creciente", m.Index)
	}
	if m.TMS < 0 || m.DurationMS < 0 || m.Bytes < 0 || m.Received < 0 || m.Expected < 0 || m.Lost < 0 {
		return Sample{}, errors.New("muestra inválida: valor negativo")
	}
	for _, f := range []float64{m.Mbps, m.LossPct, m.JitterMS} {
		if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return Sample{}, errors.New("muestra inválida: valor negativo o no finito")
		}
	}
	if m.LossPct > 100 || m.Mbps > maxSampleMbps || m.JitterMS > maxSampleMS ||
		m.DurationMS > maxSampleDurMS || m.Bytes > maxSampleBytesPerMS*m.DurationMS {
		return Sample{}, errors.New("muestra inválida: valor fuera de rango")
	}
	return Sample{Index: m.Index, TMS: m.TMS, DurationMS: m.DurationMS, Bytes: m.Bytes,
		Mbps: m.Mbps, Received: m.Received, Expected: m.Expected, Lost: m.Lost,
		LossPct: m.LossPct, JitterMS: m.JitterMS}, nil
}

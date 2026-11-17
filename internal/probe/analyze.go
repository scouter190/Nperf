package probe

import "fmt"

// Umbrales del análisis de traceroute (spec 004).
const (
	TraceLossPct   = 20.0 // pérdida persistente: >= 20 %
	TraceLatencyUp = 20.0 // subida de latencia persistente: >= 20 ms
)

// Analyze genera los hallazgos en español de un traceroute: límite ICMP no
// persistente, pérdida persistente hasta el destino, subida de latencia
// persistente y destino que no responde. Cada texto aparece como máximo una
// vez (el primer salto afectado).
func Analyze(res TraceResult) []string {
	hops := res.Hops
	var out []string

	// Límite ICMP: pérdida en un salto intermedio que no se mantiene.
	for i := 0; i < len(hops)-1; i++ {
		if hops[i].Stats.LossPct <= 0 {
			continue
		}
		for _, h := range hops[i+1:] {
			if h.Stats.LossPct < hops[i].Stats.LossPct {
				out = append(out, fmt.Sprintf("salto %d: el equipo limita respuestas ICMP (no es pérdida real)", hops[i].TTL))
				goto loss
			}
		}
	}
loss:
	// Pérdida >= 20 % que se mantiene hasta el destino.
	if res.Reached {
		for i := range hops {
			if persistsLoss(hops[i:]) {
				out = append(out, fmt.Sprintf("pérdida desde el salto %d", hops[i].TTL))
				break
			}
		}
	}

	// Latencia: media >= la del salto anterior medido + 20 ms y persistente.
	prev := -1.0
	for i, h := range hops {
		if !h.Stats.RTTOK {
			continue
		}
		if prev >= 0 && h.Stats.AvgMs >= prev+TraceLatencyUp {
			ok := true
			for _, n := range hops[i+1:] {
				if n.Stats.RTTOK && n.Stats.AvgMs < prev+TraceLatencyUp {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, fmt.Sprintf("la latencia sube en el salto %d", h.TTL))
				break
			}
		}
		prev = h.Stats.AvgMs
	}

	// Destino que no responde: racha final de saltos sin ninguna respuesta.
	if !res.Reached && len(hops) > 0 {
		n := len(hops)
		for n > 0 && hops[n-1].Stats.Received == 0 {
			n--
		}
		if n < len(hops) {
			out = append(out, fmt.Sprintf("el destino no responde o filtra ICMP a partir del salto %d", hops[n].TTL))
		}
	}
	return out
}

func persistsLoss(hops []Hop) bool {
	for _, h := range hops {
		if h.Stats.LossPct < TraceLossPct {
			return false
		}
	}
	return true
}

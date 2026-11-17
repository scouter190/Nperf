package probe

import (
	"fmt"

	"nperf/internal/netinfo"
)

// Umbrales del diagnóstico combinado (decisiones §19).
const (
	combinedLossPct = 5.0  // pérdida >= 5 % = degradado
	combinedAvgMs   = 20.0 // media > 20 ms = degradado; <= 20 ms = RTT bajo
	standardMTU     = 1500
)

// CombinedInput son los datos del diagnóstico combinado. Hop1 es el tramo 1:
// el salto 1 del trace o, si el destino es la puerta de enlace o está en
// enlace directo (Route.NextHop vacío), el ping al destino.
type CombinedInput struct {
	Dest      string
	Hop1      Stats
	Kind      netinfo.Kind // interfaz local de salida; "" = desconocida
	SignalPct *int         // señal WiFi, nil = no disponible
	MTU       int          // MTU de la ruta; 0 = desconocido
}

// Combined es el resultado: textos en español, en orden de importancia.
type Combined struct {
	Messages []string
	Healthy  bool // pérdida 0 y RTT medio <= 20 ms en el tramo 1
}

// Diagnose combina tramo 1, tipo de interfaz y MTU. Función pura.
func Diagnose(in CombinedInput) Combined {
	var c Combined
	h := in.Hop1
	if h.Sent == 0 {
		c.Messages = append(c.Messages, "sin datos suficientes para el diagnóstico combinado")
		return c
	}
	lossy := h.LossPct >= combinedLossPct
	slow := h.RTTOK && h.AvgMs > combinedAvgMs
	switch {
	case lossy || slow:
		switch in.Kind {
		case netinfo.KindWiFi:
			if in.SignalPct != nil {
				c.Messages = append(c.Messages, fmt.Sprintf("pérdida/latencia en el tramo WiFi de este equipo (señal %d %%)", *in.SignalPct))
			} else {
				c.Messages = append(c.Messages, "pérdida/latencia en el tramo WiFi de este equipo (señal no disponible)")
			}
		case netinfo.KindEthernet:
			c.Messages = append(c.Messages, "problema en el primer tramo (cable, switch o gateway)")
		default:
			c.Messages = append(c.Messages, "pérdida/latencia en el primer tramo (tipo de interfaz desconocido)")
		}
	case h.LossPct == 0 && h.RTTOK && h.AvgMs <= combinedAvgMs:
		c.Healthy = true
		c.Messages = append(c.Messages, "enlace sano hasta "+in.Dest)
	default:
		c.Messages = append(c.Messages, "el primer tramo no muestra un problema claro")
	}
	if in.MTU > 0 && in.MTU < standardMTU {
		c.Messages = append(c.Messages, fmt.Sprintf("MTU reducido en la ruta (%d; VPN/PPPoE/túnel): posibles cortes con paquetes grandes", in.MTU))
	}
	return c
}

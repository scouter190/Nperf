package inet

import "nperf/internal/probe"

// Umbrales de §21 (supuestos, se revisan tras la validación).
const (
	GatewayMaxAvgMs  = 20.0  // puerta de enlace sana: pérdida 0 y RTT <= 20 ms
	DegradedLossPct  = 2.0   // internet degradado: pérdida >= 2 %
	DegradedAvgMs    = 100.0 // RTT medio > 100 ms
	DegradedJitterMs = 30.0  // jitter > 30 ms
)

// GatewayUnresponsiveJudgesInternetOnly fija qué hacer si la puerta de
// enlace no responde (o no se detectó): true = juzgar solo internet y añadir
// una nota (propuesta del plan, pendiente de confirmar); false = tratarla
// como degradada ("red local").
const GatewayUnresponsiveJudgesInternetOnly = true

// Kind es el veredicto.
type Kind string

const (
	Healthy      Kind = "healthy"       // sin problemas detectados
	Provider     Kind = "provider"      // el problema está después del router
	LAN          Kind = "lan"           // el problema está en la red local
	NoInternet   Kind = "no_internet"   // sin salida a internet
	ICMPFiltered Kind = "icmp_filtered" // solo TCP 443: sin veredicto de latencia
)

// Result es el veredicto con su texto y notas.
type Result struct {
	Kind     Kind
	Message  string
	Notes    []string
	Degraded int // índice del primer destino de internet degradado; -1 si ninguno
}

// GatewayNote es la nota cuando la puerta de enlace no responde a ping.
const GatewayNote = "puerta de enlace no responde a ping"

var messages = map[Kind]string{
	Healthy:      "sin problemas detectados",
	Provider:     "el problema está después del router (proveedor)",
	LAN:          "el problema está en la red local (cable/WiFi/router)",
	NoInternet:   "sin salida a internet",
	ICMPFiltered: "ICMP filtrado (sin veredicto de latencia)",
}

func responded(s probe.Stats) bool { return s.Sent > 0 && s.Received > 0 }

// GatewayHealthy: pérdida 0 y RTT medio <= 20 ms.
func GatewayHealthy(s probe.Stats) bool {
	return responded(s) && s.LossPct == 0 && s.RTTOK && s.AvgMs <= GatewayMaxAvgMs
}

// InternetDegraded: pérdida >= 2 %, RTT medio > 100 ms o jitter > 30 ms.
func InternetDegraded(s probe.Stats) bool {
	if s.LossPct >= DegradedLossPct {
		return true
	}
	if s.RTTOK && s.AvgMs > DegradedAvgMs {
		return true
	}
	return s.JitterOK && s.JitterMs > DegradedJitterMs
}

// Verdict decide LAN / proveedor / sin salida. tcp443[i] indica si el
// destino i abrió TCP 443 (solo se consulta si ningún destino respondió ICMP).
func Verdict(gateway probe.Stats, internet []probe.Stats, tcp443 []bool) Result {
	r := Result{Degraded: -1}
	gwResp := responded(gateway)
	if !gwResp && GatewayUnresponsiveJudgesInternetOnly {
		r.Notes = append(r.Notes, GatewayNote)
	}
	if gwResp && !GatewayHealthy(gateway) || !gwResp && !GatewayUnresponsiveJudgesInternetOnly {
		r.Kind = LAN
		r.Message = messages[LAN]
		return r
	}
	anyICMP := false
	for i, s := range internet {
		if !responded(s) {
			continue
		}
		anyICMP = true
		if r.Degraded < 0 && InternetDegraded(s) {
			r.Degraded = i
		}
	}
	switch {
	case !anyICMP:
		r.Kind = NoInternet
		for _, ok := range tcp443 {
			if ok {
				r.Kind = ICMPFiltered
				break
			}
		}
	case r.Degraded >= 0:
		r.Kind = Provider
	default:
		r.Kind = Healthy
	}
	r.Message = messages[r.Kind]
	return r
}

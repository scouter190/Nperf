package app

import (
	"strings"
	"time"

	"nperf/internal/client"
	"nperf/internal/diag"
	"nperf/internal/proto"
)

// ChartSample es una muestra de 1 s: [t_s, dur_s, mbps, rtt_ms, loss_pct, jitter_ms].
// rtt_ms es null sin sonda respondida; loss_pct y jitter_ms son null en TCP.
type ChartSample [6]*float64

// ChartRTT es el RTT inicial (antes de la carga), en ms.
type ChartRTT struct {
	Min float64 `json:"min_ms"`
	Avg float64 `json:"avg_ms"`
	Max float64 `json:"max_ms"`
}

// ChartLoaded resume el RTT bajo carga de una fase, en ms.
type ChartLoaded struct {
	Min     float64 `json:"min_ms"`
	Avg     float64 `json:"avg_ms"`
	Median  float64 `json:"median_ms"`
	Max     float64 `json:"max_ms"`
	Samples int     `json:"samples"`
	Sent    int     `json:"sent"`
}

// ChartPhase es la gráfica de una fase (tcp_fwd, tcp_rev, udp_fwd, udp_rev).
type ChartPhase struct {
	Phase       string        `json:"phase"`
	Label       string        `json:"label"` // texto con alias y sentido; la UI lo muestra como texto
	NominalMbps *float64      `json:"nominal_mbps"`
	RTTInitial  *ChartRTT     `json:"rtt_initial"`
	Samples     []ChartSample `json:"samples"`
	LoadedRTT   *ChartLoaded  `json:"loaded_rtt"`
	Note        string        `json:"note"`
	Available   bool          `json:"available"`
}

// Chart es la gráfica del TEST!: se arma en vivo y se conserva al terminar (done, cancelled
// o error) hasta el siguiente TEST!. Los campos sin etiqueta json son estado interno.
type Chart struct {
	Phases    []ChartPhase `json:"phases"`
	Cancelled bool         `json:"cancelled,omitempty"`

	local, peer    string // alias de los extremos (etiquetas)
	locEnd, remEnd diag.End
	rtt            *ChartRTT
}

// maxChartSamples acota la memoria por fase.
const maxChartSamples = proto.MaxSamples

func (c *Chart) clone() *Chart {
	if c == nil {
		return nil
	}
	n := *c
	n.Phases = make([]ChartPhase, len(c.Phases))
	for i, p := range c.Phases {
		p.Samples = append([]ChartSample(nil), p.Samples...)
		n.Phases[i] = p
	}
	return &n
}

func f64(v float64) *float64 { return &v }

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func phaseLabel(phase, local, peer string) string {
	pr := "TCP"
	if strings.HasPrefix(phase, "udp") {
		pr = "UDP"
	}
	if strings.HasSuffix(phase, "_rev") {
		return pr + " " + peer + " → " + local
	}
	return pr + " " + local + " → " + peer
}

// nominal es mín(tx del emisor, rx del receptor) en Mbps; nil si no hay datos (§18).
func (c *Chart) nominal(phase string) *float64 {
	s, r := c.locEnd, c.remEnd
	if strings.HasSuffix(phase, "_rev") {
		s, r = r, s
	}
	if n := diag.Evaluate(0, s, r).Nominal; n > 0 {
		return f64(float64(n) / 1e6)
	}
	return nil
}

// get devuelve la fase, creándola si falta.
func (c *Chart) get(phase string) *ChartPhase {
	for i := range c.Phases {
		if c.Phases[i].Phase == phase {
			return &c.Phases[i]
		}
	}
	c.Phases = append(c.Phases, ChartPhase{Phase: phase, Label: phaseLabel(phase, c.local, c.peer),
		NominalMbps: c.nominal(phase), RTTInitial: c.rtt, Samples: []ChartSample{}, Available: true})
	return &c.Phases[len(c.Phases)-1]
}

// setLive fija si la fase tiene muestras en vivo (false = par antiguo: gráfica no disponible
// desde el inicio de la fase).
func (c *Chart) setLive(phase string, live bool) { c.get(phase).Available = live }

func chartSample(iv client.Interval) ChartSample {
	s := ChartSample{f64(iv.Start.Seconds()), f64(iv.Duration.Seconds()), f64(iv.Mbps)}
	if iv.HasRTT {
		s[3] = f64(ms(iv.RTT))
	}
	if iv.UDP {
		s[4], s[5] = f64(iv.LossPct), f64(ms(iv.Jitter))
	}
	return s
}

// ready fija los enlaces y recalcula las referencias de las fases ya creadas.
func (c *Chart) ready(local, remote *proto.Link) {
	c.locEnd, c.remEnd = diagEnd(c.local, local), diagEnd(c.peer, remote)
	for i := range c.Phases {
		c.Phases[i].NominalMbps = c.nominal(c.Phases[i].Phase)
	}
}

func (c *Chart) setRTT(r client.RTT) {
	c.rtt = &ChartRTT{Min: ms(r.Min), Avg: ms(r.Avg), Max: ms(r.Max)}
	for i := range c.Phases {
		c.Phases[i].RTTInitial = c.rtt
	}
}

func (c *Chart) addInterval(phase string, iv client.Interval) {
	p := c.get(phase)
	if len(p.Samples) >= maxChartSamples {
		return
	}
	p.Samples = append(p.Samples, chartSample(iv))
	p.LoadedRTT = loadedFromSamples(p.Samples)
}

// loadedFromSamples resume en vivo las sondas respondidas de las muestras.
func loadedFromSamples(ss []ChartSample) *ChartLoaded {
	var vals []time.Duration
	for _, s := range ss {
		if s[3] != nil {
			vals = append(vals, time.Duration(*s[3]*float64(time.Millisecond)))
		}
	}
	return loadedFrom(client.NewLoadedRTT(vals, len(ss)))
}

func loadedFrom(l *client.LoadedRTT) *ChartLoaded {
	if l == nil {
		return nil
	}
	return &ChartLoaded{Min: ms(l.Min), Avg: ms(l.Avg), Median: ms(l.Median), Max: ms(l.Max),
		Samples: l.Samples, Sent: l.Sent}
}

// finish reconstruye las fases con el resultado final (autoritativo): muestras, RTT bajo
// carga, nota y disponibilidad (false con un par sin soporte en vivo).
func (c *Chart) finish(res client.Result, notes []string) {
	if res.RTT != nil {
		c.setRTT(*res.RTT)
	}
	c.ready(res.LocalLink, res.RemoteLink)
	for _, x := range []struct {
		phase string
		d     client.Dir
	}{{"tcp_fwd", res.Fwd}, {"tcp_rev", res.Rev}, {"udp_fwd", res.UDPFwd}, {"udp_rev", res.UDPRev}} {
		if !x.d.Measured {
			continue
		}
		p := c.get(x.phase)
		p.Available = x.d.Live
		p.Samples = []ChartSample{}
		for _, iv := range x.d.Intervals {
			if len(p.Samples) >= maxChartSamples {
				break
			}
			p.Samples = append(p.Samples, chartSample(iv))
		}
		p.LoadedRTT = loadedFrom(x.d.LoadedRTT)
		if !p.Available {
			p.LoadedRTT = nil
			continue
		}
		lbl := "TCP"
		if strings.HasPrefix(x.phase, "udp") {
			lbl = "UDP"
		}
		if strings.HasSuffix(x.phase, "_rev") {
			lbl += " vuelta"
		} else {
			lbl += " ida"
		}
		for _, n := range notes {
			if strings.Contains(n, "en "+lbl) {
				p.Note = n
				break
			}
		}
	}
}

// Package export genera los formatos de exportación (CSV, HTML, SVG).
package export

import (
	"bytes"
	"encoding/xml"
	"math"
	"strconv"
	"strings"
)

// SVGPhase es la gráfica de una fase en la forma compacta de §20 (se decodifica desde el JSON
// del registro; no importa el paquete app). Cada muestra es
// [t_s, dur_s, mbps, rtt_ms, loss_pct, jitter_ms] con null donde no aplica.
type SVGPhase struct {
	Phase       string       `json:"phase"`
	Label       string       `json:"label"`
	NominalMbps *float64     `json:"nominal_mbps"`
	RTTInitial  *SVGRTT      `json:"rtt_initial"`
	Samples     [][]*float64 `json:"samples"`
	Note        string       `json:"note"`
	Available   bool         `json:"available"`
}

// SVGRTT es el RTT inicial en ms.
type SVGRTT struct {
	Min float64 `json:"min_ms"`
	Avg float64 `json:"avg_ms"`
	Max float64 `json:"max_ms"`
}

const (
	svgW, svgH           = 640.0, 300.0
	svgL, svgR           = 60.0, 16.0
	svgT, svgB           = 34.0, 66.0
	unavailableText      = "gráfica no disponible"
	maxSVGLabel          = 200
	maxSVGValue          = 1e12 // mayores se tratan como no disponibles
	maxTicks             = 50
	colSpeed, colLatency = "#1f4e9c", "#a14a00"
)

type svgPoint struct{ x, y float64 }

type svgSeries struct {
	name   string
	color  string
	dash   string // trazo de la serie ("" = continuo)
	marker string // circle | square
	// segs son tramos continuos (un null parte la línea).
	segs [][]svgPoint
}

type svgRef struct {
	label string
	y     float64
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// esc escapa texto para contenido y atributos XML (comillas incluidas).
func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// clean elimina controles y acota la longitud (en runas).
func clean(s string) string {
	r := []rune(s)
	if len(r) > maxSVGLabel {
		r = r[:maxSVGLabel]
	}
	for i, c := range r {
		if c < 0x20 || c == 0x7f {
			r[i] = ' '
		}
	}
	return string(r)
}

func at(s []*float64, i int) (float64, bool) {
	if i >= len(s) || s[i] == nil || !finite(*s[i]) || math.Abs(*s[i]) > maxSVGValue {
		return 0, false
	}
	return *s[i], true
}

// SpeedSVG dibuja la velocidad (Mbps) por segundo con la velocidad nominal discontinua.
func SpeedSVG(p SVGPhase) string {
	return build(p, "Velocidad", "Mbps", 2, colSpeed, "", "circle", func(s []*float64) (float64, bool) { return at(s, 2) },
		func() []svgRef {
			if p.NominalMbps != nil && finite(*p.NominalMbps) && *p.NominalMbps > 0 && *p.NominalMbps <= maxSVGValue {
				return []svgRef{{"nominal " + num(*p.NominalMbps) + " Mbps", *p.NominalMbps}}
			}
			return nil
		})
}

// LatencySVG dibuja el RTT (ms) bajo carga con el RTT inicial como referencia discontinua.
func LatencySVG(p SVGPhase) string {
	return build(p, "Latencia bajo carga", "ms", 1, colLatency, "6 3", "square", func(s []*float64) (float64, bool) { return at(s, 3) },
		func() []svgRef {
			if p.RTTInitial != nil && finite(p.RTTInitial.Avg) && p.RTTInitial.Avg > 0 && p.RTTInitial.Avg <= maxSVGValue {
				return []svgRef{{"RTT inicial " + num(p.RTTInitial.Avg) + " ms", p.RTTInitial.Avg}}
			}
			return nil
		})
}

func build(p SVGPhase, kind, unit string, dec int, color, dash, marker string,
	val func([]*float64) (float64, bool), refs func() []svgRef) string {
	title := kind + " · " + clean(p.Label)
	var series svgSeries
	series = svgSeries{name: kind, color: color, dash: dash, marker: marker}
	var cur []svgPoint
	var all []float64
	maxX := 0.0
	for _, s := range p.Samples {
		t, okT := at(s, 0)
		d, okD := at(s, 1)
		v, ok := val(s)
		if !okT || !ok || v < 0 {
			if len(cur) > 0 {
				series.segs = append(series.segs, cur)
				cur = nil
			}
			continue
		}
		if !okD || d < 0 {
			d = 0
		}
		x := t + d
		cur = append(cur, svgPoint{x, v})
		all = append(all, v)
		maxX = math.Max(maxX, x)
	}
	if len(cur) > 0 {
		series.segs = append(series.segs, cur)
	}
	if !p.Available || len(all) == 0 {
		return unavailableSVG(title)
	}
	rf := refs()
	maxY := 0.0
	sum, mn, mx := 0.0, all[0], all[0]
	for _, v := range all {
		sum += v
		mn, mx = math.Min(mn, v), math.Max(mx, v)
	}
	maxY = mx
	for _, r := range rf {
		maxY = math.Max(maxY, r.y)
	}
	maxX, maxY = math.Max(maxX, 1e-3), math.Max(maxY, 1e-3)
	step := niceStep(maxY, 4)
	top := math.Ceil(maxY/step) * step
	xStep := niceStep(maxX, 6)
	xTop := math.Ceil(maxX/xStep) * xStep
	if !finite(step) || step <= 0 || !finite(top) || top <= 0 || !finite(xStep) || xStep <= 0 || !finite(xTop) || xTop <= 0 {
		return unavailableSVG(title)
	}
	pw, ph := svgW-svgL-svgR, svgH-svgT-svgB
	px := func(x float64) float64 { return svgL + x/xTop*pw }
	py := func(y float64) float64 { return svgT + ph - y/top*ph }

	summary := "mín " + fix(mn, dec) + " · prom " + fix(sum/float64(len(all)), dec) + " · máx " + fix(mx, dec) + " " + unit
	if len(rf) > 0 {
		summary += " · " + rf[0].label
	}
	var b strings.Builder
	head(&b, title, summary)
	b.WriteString(`<g stroke="#888" stroke-width="1" fill="none">`)
	b.WriteString(`<rect x="` + num(svgL) + `" y="` + num(svgT) + `" width="` + num(pw) + `" height="` + num(ph) + `"/></g>`)
	b.WriteString(`<g font-family="sans-serif" font-size="11" fill="#222">`)
	for i := 0; i <= maxTicks && float64(i)*step <= top+step/2; i++ {
		v := float64(i) * step
		y := py(v)
		b.WriteString(`<line x1="` + num(svgL) + `" y1="` + num(y) + `" x2="` + num(svgL+pw) + `" y2="` + num(y) + `" stroke="#ddd"/>`)
		b.WriteString(`<text x="` + num(svgL-6) + `" y="` + num(y+4) + `" text-anchor="end">` + esc(fix(v, tickDec(step))) + `</text>`)
	}
	for i := 0; i <= maxTicks && float64(i)*xStep <= xTop+xStep/2; i++ {
		v := float64(i) * xStep
		x := px(v)
		b.WriteString(`<line x1="` + num(x) + `" y1="` + num(svgT+ph) + `" x2="` + num(x) + `" y2="` + num(svgT+ph+4) + `" stroke="#888"/>`)
		b.WriteString(`<text x="` + num(x) + `" y="` + num(svgT+ph+16) + `" text-anchor="middle">` + esc(fix(v, tickDec(xStep))) + `</text>`)
	}
	b.WriteString(`<text x="` + num(svgL+pw/2) + `" y="` + num(svgT+ph+32) + `" text-anchor="middle">Tiempo (s)</text>`)
	b.WriteString(`<text x="` + num(svgL) + `" y="` + num(svgT-8) + `">` + esc(unit) + `</text>`)
	b.WriteString(`<text x="` + num(svgL) + `" y="` + num(svgH-8) + `">` + esc(summary) + `</text></g>`)
	for _, r := range rf {
		y := py(r.y)
		b.WriteString(`<line x1="` + num(svgL) + `" y1="` + num(y) + `" x2="` + num(svgL+pw) + `" y2="` + num(y) + `" stroke="#000" stroke-width="1.5" stroke-dasharray="2 4"/>`)
		b.WriteString(`<text x="` + num(svgL+pw) + `" y="` + num(y-4) + `" text-anchor="end" font-family="sans-serif" font-size="10" fill="#000">` + esc(r.label) + `</text>`)
	}
	for _, seg := range series.segs {
		if len(seg) > 1 {
			pts := make([]string, len(seg))
			for i, q := range seg {
				pts[i] = num(px(q.x)) + "," + num(py(q.y))
			}
			b.WriteString(`<polyline fill="none" stroke="` + series.color + `" stroke-width="2"`)
			if series.dash != "" {
				b.WriteString(` stroke-dasharray="` + series.dash + `"`)
			}
			b.WriteString(` points="` + strings.Join(pts, " ") + `"/>`)
		}
		if len(seg) <= 60 {
			for _, q := range seg {
				b.WriteString(markerSVG(series.marker, px(q.x), py(q.y), series.color))
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func markerSVG(kind string, x, y float64, color string) string {
	const r = 3.5
	if kind == "square" {
		return `<rect x="` + num(x-r) + `" y="` + num(y-r) + `" width="7" height="7" fill="#fff" stroke="` + color + `" stroke-width="1.5"/>`
	}
	return `<circle cx="` + num(x) + `" cy="` + num(y) + `" r="3.5" fill="` + color + `" stroke="#fff" stroke-width="0.5"/>`
}

func head(b *strings.Builder, title, desc string) {
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + num(svgW) + ` ` + num(svgH) +
		`" width="` + num(svgW) + `" height="` + num(svgH) + `" role="img" aria-label="` + esc(title+". "+desc) + `">`)
	b.WriteString(`<title>` + esc(title) + `</title><desc>` + esc(desc) + `</desc>`)
}

func unavailableSVG(title string) string {
	var b strings.Builder
	head(&b, title, unavailableText)
	b.WriteString(`<rect x="1" y="1" width="` + num(svgW-2) + `" height="` + num(svgH/3) + `" fill="none" stroke="#888"/>`)
	b.WriteString(`<text x="` + num(svgW/2) + `" y="` + num(svgH/6+4) + `" text-anchor="middle" font-family="sans-serif" font-size="13" fill="#222">` +
		esc(title+": "+unavailableText) + `</text></svg>`)
	return b.String()
}

func fix(v float64, d int) string { return strconv.FormatFloat(v, 'f', d, 64) }

func tickDec(step float64) int {
	if step >= 1 {
		return 0
	}
	if step >= 0.1 {
		return 1
	}
	return 2
}

func niceStep(span float64, n int) float64 {
	raw := span / float64(n)
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	f := raw / p
	switch {
	case f <= 1:
		return p
	case f <= 2:
		return 2 * p
	case f <= 5:
		return 5 * p
	}
	return 10 * p
}

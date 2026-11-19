package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nperf/internal/history"
)

// MLabURL es el único enlace del informe (texto de atribución; no carga nada).
const MLabURL = "https://www.measurementlab.net/"

// MLabNotice es el aviso fijo sobre la IP pública (spec 006, §21).
const MLabNotice = "M-Lab publica en sus datos abiertos la IP pública de quien mide."

const (
	maxReportRecords = 200
	maxReportField   = 200
	maxReportNotes   = 4000
)

// ReportOptions son los campos opcionales que se rellenan al exportar.
type ReportOptions struct {
	Client     string
	Technician string
	Notes      string
}

// reportCSP prohíbe scripts y cualquier recurso externo; solo permite el CSS en línea.
const reportCSP = "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"

type rLine struct{ Label, Value string }

type rRow struct{ Phase, Dir, Speed, Detail string }

type rLight struct{ Dir, Color, ColorName, Text string }

type rEnd struct {
	Role, Alias, IP, SSID, Link string
	ShowSSID, ShowLink          bool
}

type rChart struct {
	Title string
	SVG   template.HTML
}

type rRecord struct {
	Title, Time, Status, Error string
	Ends                       []rEnd
	Lines                      []rLine
	Rows                       []rRow
	Lights                     []rLight
	Charts                     []rChart
	ShowSSID, ShowLink         bool
	Notes                      []string
	Version                    string
}

type rView struct {
	CSP        template.HTML
	Generated  string
	Client     string
	Technician string
	Notes      string
	Records    []rRecord
	MLab       bool
	MLabURL    string
	MLabNotice string
	Truncated  bool
}

type linkJSON struct {
	Kind      string `json:"kind"`
	TxBps     int64  `json:"tx_bps"`
	RxBps     int64  `json:"rx_bps"`
	SSID      string `json:"ssid"`
	Band      string `json:"band"`
	Standard  string `json:"standard"`
	SignalPct int    `json:"signal_pct"`
}

type dirDiag struct {
	Color   string `json:"color"`
	Percent int    `json:"percent"`
	Text    string `json:"text"`
	Partial bool   `json:"partial"`
}

type diagJSON struct {
	Fwd *dirDiag `json:"fwd"`
	Rev *dirDiag `json:"rev"`
}

type chartJSON struct {
	Phases []SVGPhase `json:"phases"`
}

// comma formatea con coma decimal y d decimales; no finito = "—".
func comma(v float64, d int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	return strings.Replace(strconv.FormatFloat(v, 'f', d, 64), ".", ",", 1)
}

func pf(v *float64, d int, unit string) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return "—"
	}
	return comma(*v, d) + unit
}

func speedStr(v *float64) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 {
		return "no medido"
	}
	return comma(*v, 2) + " Mbps"
}

// cut elimina controles y acota la longitud en runas (el escapado lo hace html/template).
func cut(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r == utf8.RuneError || r < 0x20 || r == 0x7f {
			continue
		}
		if n++; n > max {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func cutNotes(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r == utf8.RuneError || (r < 0x20 && r != '\n') || r == 0x7f {
			continue
		}
		if n++; n > max {
			b.WriteString("…")
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func statusText(s string) string {
	switch s {
	case history.StatusDone:
		return "Completado"
	case history.StatusCancelled:
		return "Cancelado"
	case history.StatusError:
		return "Con error"
	}
	return "Desconocido"
}

func kindTitle(k string) string {
	switch k {
	case history.KindTest:
		return "Prueba de velocidad en la red local"
	case history.KindPing:
		return "Ping"
	case history.KindTrace:
		return "Traceroute"
	case history.KindMTU:
		return "MTU de la ruta"
	case history.KindPorts:
		return "Comprobación de puertos"
	case history.KindInternet:
		return "Latencia hacia internet"
	case history.KindSpeed:
		return "Velocidad de internet (M-Lab NDT7)"
	}
	return "Resultado"
}

func linkText(k string) string {
	switch k {
	case "eth":
		return "Cable"
	case "wifi":
		return "Wi-Fi"
	case "other":
		return "Otro"
	}
	return ""
}

func nominal(mbps float64) string {
	if mbps <= 0 || math.IsNaN(mbps) || math.IsInf(mbps, 0) {
		return ""
	}
	return comma(mbps, 0) + " Mbps nominales"
}

func endOf(role string, e *history.Endpoint) (rEnd, bool) {
	if e == nil {
		return rEnd{}, false
	}
	l := cut(linkText(e.Link), 40)
	if l == "" {
		l = cut(e.Link, 40)
	}
	if n := nominal(e.NominalMbps); n != "" {
		if l != "" {
			l += ", "
		}
		l += n
	}
	return rEnd{Role: role, Alias: cut(e.Alias, maxReportField), IP: cut(e.IP, maxReportField),
		SSID: cut(e.SSID, maxReportField), Link: l}, true
}

func dirRows(phase string, p *pair, udp bool) []rRow {
	var out []rRow
	for _, x := range []struct {
		name string
		d    dir
	}{{"Cliente a servidor", p.Fwd}, {"Servidor a cliente", p.Rev}} {
		if x.d.Status == "" {
			continue
		}
		detail := ""
		switch {
		case x.d.Status != "ok":
			detail = "no disponible"
		case udp && x.d.UDP != nil:
			detail = "pérdida " + pf(&x.d.UDP.LossPct, 2, " %") + ", jitter " + pf(&x.d.UDP.JitterMS, 2, " ms")
		case !udp && x.d.Retrans != nil && x.d.Retrans.Count != nil:
			detail = fmt.Sprintf("retransmisiones %d", *x.d.Retrans.Count)
			if x.d.Retrans.Estimated {
				detail += " (estimadas)"
			}
		}
		out = append(out, rRow{Phase: phase, Dir: x.name, Speed: speedStr(x.d.ThroughputMbps), Detail: detail})
	}
	return out
}

func lights(raw json.RawMessage) []rLight {
	var d diagJSON
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return nil
	}
	var out []rLight
	for _, x := range []struct {
		name string
		d    *dirDiag
	}{{"Cliente a servidor", d.Fwd}, {"Servidor a cliente", d.Rev}} {
		if x.d == nil || x.d.Color == "" {
			continue
		}
		var col, name string
		switch x.d.Color {
		case "green":
			col, name = "#1b7f3b", "Verde"
		case "yellow":
			col, name = "#a67c00", "Amarillo"
		case "red":
			col, name = "#b3261e", "Rojo"
		default:
			continue
		}
		t := cut(x.d.Text, 400)
		if x.d.Partial {
			t += " (diagnóstico parcial)"
		}
		out = append(out, rLight{Dir: x.name, Color: col, ColorName: name + ", " + strconv.Itoa(x.d.Percent) + " %", Text: t})
	}
	return out
}

func phaseTitle(p SVGPhase) string {
	proto, d := "", ""
	switch {
	case strings.HasPrefix(p.Phase, "tcp"):
		proto = "TCP"
	case strings.HasPrefix(p.Phase, "udp"):
		proto = "UDP"
	}
	switch {
	case strings.HasSuffix(p.Phase, "_fwd"):
		d = "Cliente a servidor"
	case strings.HasSuffix(p.Phase, "_rev"):
		d = "Servidor a cliente"
	}
	if proto == "" || d == "" {
		return cut(p.Label, maxReportField)
	}
	return proto + " · " + d
}

func charts(raw json.RawMessage) []rChart {
	var c chartJSON
	if len(raw) == 0 || json.Unmarshal(raw, &c) != nil {
		return nil
	}
	var out []rChart
	for i, p := range c.Phases {
		if i >= 8 {
			break
		}
		// SEGURIDAD: SpeedSVG y LatencySVG son nuestro generador; escapan (esc) todo texto del
		// registro y solo emiten números formateados por NPerf. Por eso su salida puede
		// insertarse como template.HTML sin pasar de nuevo por el escapado.
		t := phaseTitle(p)
		out = append(out, rChart{"Velocidad · " + t, template.HTML(SpeedSVG(p))})             // #nosec G203
		out = append(out, rChart{"Latencia bajo carga · " + t, template.HTML(LatencySVG(p))}) // #nosec G203
	}
	return out
}

func buildRecord(r *history.Record) rRecord {
	rr := rRecord{Title: kindTitle(r.Kind), Status: statusText(r.Status),
		Error: cut(r.Error, 1024), Version: cut(r.Version, 64)}
	if t, err := time.Parse(time.RFC3339, r.Time); err == nil {
		rr.Time = t.Format("02/01/2006 15:04:05 -07:00")
	}
	if e, ok := endOf("Origen", r.Source); ok {
		rr.Ends = append(rr.Ends, e)
	}
	if e, ok := endOf("Destino", r.Dest); ok {
		rr.Ends = append(rr.Ends, e)
	}
	for _, e := range rr.Ends {
		rr.ShowSSID = rr.ShowSSID || e.SSID != ""
		rr.ShowLink = rr.ShowLink || e.Link != ""
	}
	for i := range rr.Ends {
		rr.Ends[i].ShowSSID, rr.Ends[i].ShowLink = rr.ShowSSID, rr.ShowLink
	}
	res := parseResult(r)
	switch r.Kind {
	case history.KindTest:
		if res.RTT != nil {
			rr.Lines = append(rr.Lines, rLine{"Latencia inicial (mín / media / máx)",
				pf(res.RTT.Min, 2, "") + " / " + pf(res.RTT.Avg, 2, "") + " / " + pf(res.RTT.Max, 2, " ms")})
		}
		if res.TCP != nil {
			rr.Rows = append(rr.Rows, dirRows("TCP", res.TCP, false)...)
		}
		if res.UDP != nil {
			rr.Rows = append(rr.Rows, dirRows("UDP", res.UDP, true)...)
		}
		rr.Lights = lights(r.Diagnosis)
		rr.Charts = charts(r.Chart)
	case history.KindPing, history.KindInternet:
		rr.Lines = append(rr.Lines,
			rLine{"Pérdida", pf(res.LossPct, 2, " %")},
			rLine{"Latencia (mín / media / máx)", pf(res.MinMs, 2, "") + " / " + pf(res.AvgMs, 2, "") + " / " + pf(res.MaxMs, 2, " ms")},
			rLine{"Jitter", pf(res.JitterMs, 2, " ms")})
	case history.KindMTU:
		if res.MTUBytes != nil {
			rr.Lines = append(rr.Lines, rLine{"MTU de la ruta", strconv.FormatInt(*res.MTUBytes, 10) + " bytes"})
		}
	case history.KindSpeed:
		for _, x := range []struct {
			n string
			d *speedDir
		}{{"Descarga", res.Download}, {"Subida", res.Upload}} {
			if x.d == nil {
				rr.Lines = append(rr.Lines, rLine{x.n, "no disponible"})
				continue
			}
			v := speedStr(x.d.Mbps)
			if x.d.Incomplete {
				v += " (incompleta)"
			}
			if x.d.MinRTTMs != nil {
				v += ", RTT mínimo " + pf(x.d.MinRTTMs, 1, " ms")
			}
			rr.Lines = append(rr.Lines, rLine{x.n, v})
		}
	}
	for i, n := range r.Notes {
		if i >= 20 {
			break
		}
		rr.Notes = append(rr.Notes, cutNotes(n, 1024))
	}
	return rr
}

// ReportHTML genera el informe para el cliente: un HTML autocontenido (CSS y SVG en línea, sin
// scripts ni recursos externos) pensado para imprimir a PDF en A4 desde el navegador.
// Incluye IPs y SSID (decisión del usuario); no hay clave, MAC ni BSSID en el registro. El
// escapado lo hace html/template; solo el SVG de svg.go entra como template.HTML.
func ReportHTML(recs []*history.Record, o ReportOptions, now time.Time) ([]byte, error) {
	v := rView{
		// #nosec G203: constante propia, sin datos externos.
		CSP:        template.HTML(`<meta http-equiv="Content-Security-Policy" content="` + reportCSP + `">`),
		Generated:  now.Format("02/01/2006 15:04"),
		Client:     cut(o.Client, maxReportField),
		Technician: cut(o.Technician, maxReportField),
		Notes:      cutNotes(o.Notes, maxReportNotes),
		MLabURL:    MLabURL,
		MLabNotice: MLabNotice,
	}
	if len(recs) > maxReportRecords {
		recs, v.Truncated = recs[:maxReportRecords], true
	}
	for _, r := range recs {
		if r == nil {
			continue
		}
		if r.Kind == history.KindSpeed {
			v.MLab = true
		}
		v.Records = append(v.Records, buildRecord(r))
	}
	var b bytes.Buffer
	if err := reportTmpl.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

var reportTmpl = template.Must(template.New("report").Parse(reportSrc))

const reportSrc = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
{{.CSP}}
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<title>Informe de red - NPerf</title>
<style>
@page { size: A4; margin: 15mm; }
* { box-sizing: border-box; }
body { font: 11pt/1.4 "Segoe UI", Arial, sans-serif; color: #111; background: #fff; margin: 0 auto; max-width: 190mm; padding: 8mm; }
h1 { font-size: 20pt; margin: 0 0 4mm; }
h2 { font-size: 14pt; margin: 0 0 2mm; }
h3 { font-size: 11pt; margin: 4mm 0 1mm; }
h4 { font-size: 10pt; margin: 3mm 0 0; }
.meta { border: 1px solid #999; padding: 3mm; margin-bottom: 5mm; }
.meta p { margin: 0 0 1mm; }
.rec { border-top: 2px solid #1f4e9c; padding-top: 3mm; margin-top: 6mm; break-inside: avoid-page; }
table { border-collapse: collapse; width: 100%; margin: 2mm 0; }
th, td { border: 1px solid #999; padding: 1.5mm 2mm; text-align: left; vertical-align: top; overflow-wrap: anywhere; }
th { background: #eee; }
.ends td:nth-child(1), .ends td:nth-child(3) { white-space: nowrap; }
table { table-layout: auto; max-width: 100%; }
.light { display: flex; gap: 3mm; align-items: baseline; margin: 1mm 0; }
.dot { display: inline-block; width: 4mm; height: 4mm; border-radius: 50%; flex: none; border: 1px solid #000; }
.err { color: #b3261e; font-weight: bold; }
.notes { white-space: pre-wrap; overflow-wrap: anywhere; }
svg { max-width: 100%; height: auto; margin: 2mm 0; break-inside: avoid; }
.mlab { border: 1px solid #999; padding: 3mm; margin-top: 6mm; font-size: 10pt; }
footer { margin-top: 6mm; font-size: 9pt; color: #444; }
h2, h3, h4 { break-after: avoid; }
@media print { h2, h3, h4 { break-after: avoid; break-inside: avoid; } body { padding: 0; max-width: none; } .rec { break-inside: auto; } table, svg { break-inside: avoid; } }
</style>
</head>
<body>
<h1>Informe de red</h1>
<div class="meta">
<p>Generado: {{.Generated}}</p>
{{if .Client}}<p>Cliente: {{.Client}}</p>{{end}}
{{if .Technician}}<p>Técnico: {{.Technician}}</p>{{end}}
{{if .Notes}}<p>Notas:</p><p class="notes">{{.Notes}}</p>{{end}}
</div>
{{if .Truncated}}<p>Solo se muestran los primeros resultados seleccionados.</p>{{end}}
{{range .Records}}
<section class="rec">
<h2>{{.Title}}</h2>
<p>{{.Time}} - {{.Status}}</p>
{{if .Error}}<p class="err">Error: {{.Error}}</p>{{end}}
{{if .Ends}}<table class="ends">
<tr><th>Equipo</th><th>Nombre</th><th>IP</th>{{if .ShowSSID}}<th>Wi-Fi (SSID)</th>{{end}}{{if .ShowLink}}<th>Enlace</th>{{end}}</tr>
{{range .Ends}}<tr><td>{{.Role}}</td><td>{{.Alias}}</td><td>{{.IP}}</td>{{if .ShowSSID}}<td>{{.SSID}}</td>{{end}}{{if .ShowLink}}<td>{{.Link}}</td>{{end}}</tr>
{{end}}</table>{{end}}
{{if .Lines}}<table>
{{range .Lines}}<tr><th>{{.Label}}</th><td>{{.Value}}</td></tr>
{{end}}</table>{{end}}
{{if .Rows}}<table>
<tr><th>Protocolo</th><th>Sentido</th><th>Velocidad</th><th>Detalle</th></tr>
{{range .Rows}}<tr><td>{{.Phase}}</td><td>{{.Dir}}</td><td>{{.Speed}}</td><td>{{.Detail}}</td></tr>
{{end}}</table>{{end}}
{{if .Lights}}<h3>Diagnóstico</h3>
{{range .Lights}}<div class="light"><span class="dot" style="background:{{.Color}}"></span><span><strong>{{.Dir}}: {{.ColorName}}.</strong> {{.Text}}</span></div>
{{end}}{{end}}
{{if .Charts}}<h3>Gráficas</h3>
{{range .Charts}}<h4>{{.Title}}</h4>{{.SVG}}
{{end}}{{end}}
{{if .Notes}}<h3>Notas del resultado</h3><ul>{{range .Notes}}<li class="notes">{{.}}</li>{{end}}</ul>{{end}}
</section>
{{end}}
{{if .MLab}}<div class="mlab">
<p><strong>Aviso.</strong> {{.MLabNotice}} La prueba de velocidad consume datos.</p>
<p>Medición realizada con M-Lab NDT7 (Measurement Lab). Más información: <a href="{{.MLabURL}}">{{.MLabURL}}</a></p>
</div>{{end}}
<footer>Informe generado por NPerf.</footer>
</body>
</html>
`

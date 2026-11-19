package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"nperf/internal/history"
)

// CSVFormat elige separador y decimal del CSV.
type CSVFormat int

const (
	// CSVStd usa ',' y punto decimal.
	CSVStd CSVFormat = iota
	// CSVExcelES usa ';' y coma decimal (Excel con configuración regional española).
	CSVExcelES
)

// SummaryHeader es la cabecera fija del CSV de resumen. phase indica el contexto: tcp/udp en
// un TEST! (loss_pct y jitter_ms solo se rellenan en la fase udp), ping/mtu/trace/ports en
// diagnósticos (loss_pct y jitter_ms del ping) y speed en la prueba hacia internet
// (rtt_min_ms = RTT mínimo del servidor). Número no medido: celda vacía y measure_status.
var SummaryHeader = []string{
	"id", "time", "kind", "status", "error",
	"source_alias", "source_ip", "source_ssid", "dest_alias", "dest_ip", "dest_ssid",
	"phase", "direction", "measure_status", "throughput_mbps", "bytes", "duration_s",
	"retransmissions", "retransmissions_estimated", "loss_pct", "jitter_ms",
	"rtt_min_ms", "rtt_avg_ms", "rtt_max_ms", "mtu_bytes",
}

// IntervalsHeader es la cabecera fija del CSV de intervalos.
var IntervalsHeader = []string{
	"id", "time", "phase", "direction", "t_s", "duration_s", "bytes", "throughput_mbps",
	"rtt_ms", "datagrams_received", "datagrams_lost", "loss_pct", "jitter_ms",
}

type cw struct{ fm CSVFormat }

// txt neutraliza la inyección de fórmulas en texto que no es un número de NPerf.
func txt(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + s
	}
	return s
}

func (c cw) dec(s string) string {
	if c.fm == CSVExcelES {
		return strings.Replace(s, ".", ",", 1)
	}
	return s
}

func (c cw) fp(v *float64) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return ""
	}
	return c.dec(strconv.FormatFloat(math.Round(*v*1000)/1000, 'f', -1, 64))
}

func (c cw) f(v float64) string { return c.fp(&v) }

func ip(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func (c cw) encode(header []string, rows [][]string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	if c.fm == CSVExcelES {
		w.Comma = ';'
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	return b.Bytes(), w.Error()
}

type rtt struct {
	Min *float64 `json:"min_ms"`
	Avg *float64 `json:"avg_ms"`
	Max *float64 `json:"max_ms"`
}

type retr struct {
	Status    string `json:"status"`
	Count     *int64 `json:"count"`
	Estimated bool   `json:"estimated"`
}

type udpInfo struct {
	JitterMS float64 `json:"jitter_ms"`
	LossPct  float64 `json:"loss_pct"`
}

type interval struct {
	TS                float64  `json:"t_s"`
	DurationS         float64  `json:"duration_s"`
	Bytes             int64    `json:"bytes"`
	ThroughputMbps    float64  `json:"throughput_mbps"`
	RTTMS             *float64 `json:"rtt_ms"`
	DatagramsReceived *int64   `json:"datagrams_received"`
	DatagramsLost     *int64   `json:"datagrams_lost"`
	LossPct           *float64 `json:"loss_pct"`
	JitterMS          *float64 `json:"jitter_ms"`
}

type dir struct {
	Status         string     `json:"status"`
	ThroughputMbps *float64   `json:"throughput_mbps"`
	Bytes          *int64     `json:"bytes"`
	DurationS      *float64   `json:"duration_s"`
	Retrans        *retr      `json:"retransmissions"`
	UDP            *udpInfo   `json:"udp"`
	Intervals      []interval `json:"intervals"`
}

type pair struct {
	Fwd dir `json:"fwd"`
	Rev dir `json:"rev"`
}

// result reúne los campos que el CSV lee de los distintos kinds (los ausentes quedan vacíos).
type result struct {
	// test
	RTT *rtt  `json:"rtt"`
	TCP *pair `json:"tcp"`
	UDP *pair `json:"udp"`
	// ping
	Status   string   `json:"status"`
	LossPct  *float64 `json:"loss_pct"`
	MinMs    *float64 `json:"min_ms"`
	AvgMs    *float64 `json:"avg_ms"`
	MaxMs    *float64 `json:"max_ms"`
	JitterMs *float64 `json:"jitter_ms"`
	MTUBytes *int64   `json:"mtu_bytes"`
	// speed
	Download *speedDir `json:"download"`
	Upload   *speedDir `json:"upload"`
}

type speedDir struct {
	Mbps       *float64 `json:"mbps"`
	Bytes      *int64   `json:"bytes"`
	DurationS  *float64 `json:"duration_s"`
	MinRTTMs   *float64 `json:"min_rtt_ms"`
	Incomplete bool     `json:"incomplete"`
}

func parseResult(r *history.Record) result {
	var res result
	if len(r.Result) > 0 {
		// Un resultado ilegible se trata como no medido.
		if json.Unmarshal(r.Result, &res) != nil {
			res = result{}
		}
	}
	return res
}

func (c cw) head(r *history.Record) []string {
	var s, d history.Endpoint
	if r.Source != nil {
		s = *r.Source
	}
	if r.Dest != nil {
		d = *r.Dest
	}
	return []string{
		txt(r.ID), txt(r.Time), txt(r.Kind), txt(r.Status), txt(r.Error),
		txt(s.Alias), txt(s.IP), txt(s.SSID), txt(d.Alias), txt(d.IP), txt(d.SSID),
	}
}

func (c cw) row(r *history.Record, tail []string) []string {
	return append(c.head(r), tail...)
}

// tail: phase, direction, measure_status, throughput, bytes, duration, retrans, retrans_est,
// udp_loss, udp_jitter, rtt min/avg/max, mtu.
func (c cw) tail(phase, direction, status string, thr *float64, bytes *int64, dur *float64,
	rt *retr, u *udpInfo, rr *rtt, mtu *int64) []string {
	t := []string{txt(phase), txt(direction), txt(status), c.fp(thr), ip(bytes), c.fp(dur), "", "", "", ""}
	if rt != nil && rt.Count != nil {
		t[6] = ip(rt.Count)
		if rt.Estimated {
			t[7] = "true"
		} else {
			t[7] = "false"
		}
	}
	if u != nil {
		t[8], t[9] = c.f(u.LossPct), c.f(u.JitterMS)
	}
	if rr != nil {
		t = append(t, c.fp(rr.Min), c.fp(rr.Avg), c.fp(rr.Max))
	} else {
		t = append(t, "", "", "")
	}
	return append(t, ip(mtu))
}

func (c cw) dirRow(r *history.Record, phase, direction string, d dir, rr *rtt, udp bool) []string {
	st := d.Status
	if st == "" {
		st = "not_measured"
	}
	var u *udpInfo
	if udp {
		u = d.UDP
	}
	var rt *retr
	if !udp {
		rt = d.Retrans
	}
	return c.row(r, c.tail(phase, direction, st, d.ThroughputMbps, d.Bytes, d.DurationS, rt, u, rr, nil))
}

// SummaryCSV genera el CSV de resumen: una fila por fase y sentido del TEST! y una por
// diagnóstico o prueba hacia internet (dos para la velocidad). No medido → vacío + estado.
func SummaryCSV(recs []*history.Record, f CSVFormat) ([]byte, error) {
	c := cw{f}
	var rows [][]string
	for _, r := range recs {
		res := parseResult(r)
		switch r.Kind {
		case history.KindTest:
			if res.TCP == nil && res.UDP == nil {
				rows = append(rows, c.row(r, c.tail("", "", "not_measured", nil, nil, nil, nil, nil, res.RTT, nil)))
				continue
			}
			if res.TCP != nil {
				rows = append(rows, c.dirRow(r, "tcp", "fwd", res.TCP.Fwd, res.RTT, false),
					c.dirRow(r, "tcp", "rev", res.TCP.Rev, res.RTT, false))
			}
			if res.UDP != nil {
				rows = append(rows, c.dirRow(r, "udp", "fwd", res.UDP.Fwd, res.RTT, true),
					c.dirRow(r, "udp", "rev", res.UDP.Rev, res.RTT, true))
			}
		case history.KindSpeed:
			for _, x := range []struct {
				n string
				d *speedDir
			}{{"download", res.Download}, {"upload", res.Upload}} {
				if x.d == nil {
					rows = append(rows, c.row(r, c.tail("speed", x.n, "not_measured", nil, nil, nil, nil, nil, nil, nil)))
					continue
				}
				st := "ok"
				if x.d.Incomplete {
					st = "incomplete"
				} else if x.d.Mbps == nil {
					st = "not_measured"
				}
				rows = append(rows, c.row(r, c.tail("speed", x.n, st, x.d.Mbps, x.d.Bytes, x.d.DurationS, nil, nil,
					&rtt{Min: x.d.MinRTTMs}, nil)))
			}
		default:
			st := res.Status
			if len(r.Result) == 0 {
				st = "not_measured"
			}
			var u *udpInfo
			if res.LossPct != nil {
				u = &udpInfo{LossPct: *res.LossPct}
				if res.JitterMs != nil {
					u.JitterMS = *res.JitterMs
				}
			}
			t := c.tail(r.Kind, "", st, nil, nil, nil, nil, u, &rtt{Min: res.MinMs, Avg: res.AvgMs, Max: res.MaxMs}, res.MTUBytes)
			if u != nil && res.JitterMs == nil {
				t[9] = ""
			}
			rows = append(rows, c.row(r, t))
		}
	}
	return c.encode(SummaryHeader, rows)
}

// IntervalsCSV genera el CSV de intervalos (solo registros TEST! con muestras de 1 s).
func IntervalsCSV(recs []*history.Record, f CSVFormat) ([]byte, error) {
	c := cw{f}
	var rows [][]string
	for _, r := range recs {
		if r.Kind != history.KindTest {
			continue
		}
		res := parseResult(r)
		for _, p := range []struct {
			ph string
			pr *pair
		}{{"tcp", res.TCP}, {"udp", res.UDP}} {
			if p.pr == nil {
				continue
			}
			for _, d := range []struct {
				n string
				d dir
			}{{"fwd", p.pr.Fwd}, {"rev", p.pr.Rev}} {
				for _, iv := range d.d.Intervals {
					rows = append(rows, []string{
						txt(r.ID), txt(r.Time), p.ph, d.n,
						c.f(iv.TS), c.f(iv.DurationS), strconv.FormatInt(iv.Bytes, 10), c.f(iv.ThroughputMbps),
						c.fp(iv.RTTMS), ip(iv.DatagramsReceived), ip(iv.DatagramsLost), c.fp(iv.LossPct), c.fp(iv.JitterMS),
					})
				}
			}
		}
	}
	return c.encode(IntervalsHeader, rows)
}

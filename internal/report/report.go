// Package report da formato al resultado de una medición: texto legible y JSON (formato v1).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nperf/internal/client"
	"nperf/internal/meter"
)

// FormatVersion es la versión del formato JSON (docs/decisiones.md sección 16).
const FormatVersion = 1

// Params son los parámetros de la prueba que se informan junto al resultado.
type Params struct {
	Host      string
	Port      int
	Proto     string // tcp, udp o both
	Direction string // fwd, rev o both
	Duration  time.Duration
	Streams   int
	UDPRate   float64 // bps pedidos con -udp-rate; 0 = automática
	Version   string
}

// Estados explícitos del JSON.
const (
	StatusMeasured    = "measured"
	StatusNotMeasured = "not_measured"
	StatusAvailable   = "available"
	StatusUnavailable = "unavailable"
)

// Report es el objeto JSON v1. Los nombres de campo son estables.
type Report struct {
	FormatVersion int        `json:"format_version"`
	Params        JSONParams `json:"params"`
	RTT           JSONRTT    `json:"rtt"`
	TCP           JSONPair   `json:"tcp"`
	UDP           JSONPair   `json:"udp"`
	Notes         []string   `json:"notes"`
}

type JSONParams struct {
	Host       string   `json:"host"`
	Port       int      `json:"port"`
	Proto      string   `json:"proto"`
	Direction  string   `json:"direction"`
	DurationS  float64  `json:"duration_s"`
	Streams    int      `json:"streams"`
	UDPRateBps *float64 `json:"udp_rate_bps"` // null = automática
	Version    string   `json:"version"`
}

type JSONRTT struct {
	Status  string   `json:"status"`
	MinMS   *float64 `json:"min_ms"`
	AvgMS   *float64 `json:"avg_ms"`
	MaxMS   *float64 `json:"max_ms"`
	Samples int      `json:"samples"`
}

type JSONPair struct {
	Fwd JSONDir `json:"fwd"`
	Rev JSONDir `json:"rev"`
}

// JSONDir es un protocolo en un sentido. Con status "not_measured" los demás campos son null.
type JSONDir struct {
	Status         string       `json:"status"`
	ThroughputMbps *float64     `json:"throughput_mbps"` // medido por el receptor
	Bytes          *int64       `json:"bytes"`           // recibidos
	DurationS      *float64     `json:"duration_s"`
	Retransmission *JSONRetrans `json:"retransmissions"` // solo TCP
	UDP            *JSONUDP     `json:"udp"`             // solo UDP
}

type JSONRetrans struct {
	Status    string  `json:"status"` // available | unavailable
	Count     *uint64 `json:"count"`
	Estimated bool    `json:"estimated"`
}

type JSONUDP struct {
	SentMbps          float64  `json:"sent_mbps"`
	SentBytes         int64    `json:"sent_bytes"`
	TargetRateBps     *float64 `json:"target_rate_bps"`
	JitterMS          float64  `json:"jitter_ms"`
	LossPct           float64  `json:"loss_pct"`
	DatagramsSent     int64    `json:"datagrams_sent"`
	DatagramsExpected int64    `json:"datagrams_expected"`
	DatagramsLost     int64    `json:"datagrams_lost"`
}

func ptr[T any](v T) *T { return &v }

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// targets devuelve la tasa UDP objetivo (bps) de cada sentido; 0 = desconocida.
func targets(p Params, res client.Result) (fwd, rev float64) {
	switch p.Proto {
	case "both":
		return res.UDPRateFwd, res.UDPRateRev
	case "udp":
		t := p.UDPRate
		if t == 0 {
			t = client.DefaultUDPRate
		}
		return t, t
	}
	return 0, 0
}

func jsonDir(d client.Dir, target float64) JSONDir {
	if !d.Measured {
		return JSONDir{Status: StatusNotMeasured}
	}
	j := JSONDir{Status: StatusMeasured, ThroughputMbps: ptr(d.Mbps), Bytes: ptr(d.Received.Bytes),
		DurationS: ptr(d.Received.Elapsed.Seconds())}
	if u := d.UDP; u != nil {
		ju := &JSONUDP{SentMbps: u.SentMbps, SentBytes: d.Sent.Bytes, JitterMS: u.JitterMS, LossPct: u.LossPct,
			DatagramsSent: u.Packets, DatagramsExpected: u.Expected, DatagramsLost: u.Lost}
		if target > 0 {
			ju.TargetRateBps = ptr(target)
		}
		j.UDP = ju
		return j
	}
	r := &JSONRetrans{Status: StatusUnavailable}
	if d.Sent.RetransOK {
		r = &JSONRetrans{Status: StatusAvailable, Count: ptr(d.Sent.Retrans), Estimated: d.Sent.RetransEstimated}
	}
	j.Retransmission = r
	return j
}

// Build arma el objeto JSON v1 a partir del resultado.
func Build(p Params, res client.Result) Report {
	r := Report{FormatVersion: FormatVersion, Notes: Notes(p, res)}
	r.Params = JSONParams{Host: p.Host, Port: p.Port, Proto: p.Proto, Direction: p.Direction,
		DurationS: p.Duration.Seconds(), Streams: p.Streams, Version: p.Version}
	if p.UDPRate > 0 {
		r.Params.UDPRateBps = ptr(p.UDPRate)
	}
	r.RTT = JSONRTT{Status: StatusNotMeasured}
	if t := res.RTT; t != nil && p.Proto == "both" {
		r.RTT = JSONRTT{Status: StatusMeasured, MinMS: ptr(ms(t.Min)), AvgMS: ptr(ms(t.Avg)), MaxMS: ptr(ms(t.Max)), Samples: t.Samples}
	}
	tf, tr := targets(p, res)
	none := JSONDir{Status: StatusNotMeasured}
	r.TCP, r.UDP = JSONPair{Fwd: none, Rev: none}, JSONPair{Fwd: none, Rev: none}
	switch p.Proto {
	case "both":
		r.TCP = JSONPair{Fwd: jsonDir(res.Fwd, 0), Rev: jsonDir(res.Rev, 0)}
		r.UDP = JSONPair{Fwd: jsonDir(res.UDPFwd, tf), Rev: jsonDir(res.UDPRev, tr)}
	case "tcp":
		r.TCP = JSONPair{Fwd: jsonDir(res.Fwd, 0), Rev: jsonDir(res.Rev, 0)}
	case "udp":
		r.UDP = JSONPair{Fwd: jsonDir(res.Fwd, tf), Rev: jsonDir(res.Rev, tr)}
	}
	return r
}

// JSON escribe el informe como un único objeto JSON indentado seguido de un salto de línea.
func JSON(w io.Writer, p Params, res client.Result) error {
	return encode(w, Build(p, res))
}

// JSONError escribe {"error": "..."}.
func JSONError(w io.Writer, err error) error {
	return encode(w, struct {
		Error string `json:"error"`
	}{err.Error()})
}

func encode(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// Notes reúne las notas del cliente y la de tasa UDP no alcanzada, sin vacías.
func Notes(p Params, res client.Result) []string {
	notes := []string{}
	notes = append(notes, res.Notes...)
	tf, tr := targets(p, res)
	switch p.Proto {
	case "both":
		notes = append(notes, RateNote("fwd", res.UDPFwd, tf), RateNote("rev", res.UDPRev, tr))
	case "udp":
		notes = append(notes, RateNote("fwd", res.Fwd, tf), RateNote("rev", res.Rev, tr))
	}
	out := notes[:0]
	for _, n := range notes {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// RateNote devuelve la nota de "tasa no alcanzada" si lo enviado queda por debajo del 90 %
// de la objetivo (bps); "" si no aplica.
func RateNote(dir string, d client.Dir, targetBps float64) string {
	if !d.Measured || d.UDP == nil || targetBps <= 0 {
		return ""
	}
	target := targetBps / 1e6
	if d.UDP.SentMbps >= 0.9*target {
		return ""
	}
	return fmt.Sprintf("UDP %s: no se alcanzó la tasa objetivo (enviado %.1f de %.1f Mbps)", dir, d.UDP.SentMbps, target)
}

// Text escribe el resultado legible: RTT, TCP fwd, TCP rev, UDP fwd, UDP rev y las notas.
func Text(w io.Writer, p Params, res client.Result) {
	if p.Proto == "both" {
		if r := res.RTT; r != nil {
			fmt.Fprintf(w, "RTT: mín %s, media %s, máx %s (%d pings)\n", FmtRTT(r.Min), FmtRTT(r.Avg), FmtRTT(r.Max), r.Samples)
		}
		printDir(w, "tcp", "fwd", "cliente -> servidor", res.Fwd)
		printDir(w, "tcp", "rev", "servidor -> cliente", res.Rev)
		printDir(w, "udp", "fwd", "cliente -> servidor", res.UDPFwd)
		printDir(w, "udp", "rev", "servidor -> cliente", res.UDPRev)
	} else {
		printDir(w, p.Proto, "fwd", "cliente -> servidor", res.Fwd)
		printDir(w, p.Proto, "rev", "servidor -> cliente", res.Rev)
	}
	for _, n := range Notes(p, res) {
		fmt.Fprintln(w, "nota:", n)
	}
}

// FmtRTT da formato a un RTT en ms; por debajo de 1 µs (incluido 0, por la resolución del
// reloj) muestra "<1 µs".
func FmtRTT(d time.Duration) string {
	if d < time.Microsecond {
		return "<1 µs"
	}
	return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
}

func printDir(w io.Writer, proto, dir, label string, d client.Dir) {
	name := strings.ToUpper(proto)
	if !d.Measured {
		fmt.Fprintf(w, "%s %s (%s): no medido\n", name, dir, label)
		return
	}
	if u := d.UDP; u != nil {
		fmt.Fprintf(w, "%s %s (%s): enviado %.1f Mbps, recibido %.1f Mbps, jitter %.3f ms, pérdida %.2f %% (%d de %d datagramas)\n",
			name, dir, label, u.SentMbps, d.Mbps, u.JitterMS, u.LossPct, u.Lost, u.Expected)
		return
	}
	fmt.Fprintf(w, "%s %s (%s): %.1f Mbps (%d bytes en %.1f s), retransmisiones: %s\n",
		name, dir, label, d.Mbps, d.Received.Bytes, d.Received.Elapsed.Seconds(), FmtRetrans(d.Sent))
}

// FmtRetrans da formato a las retransmisiones del emisor o "no disponible".
func FmtRetrans(r meter.Result) string {
	if !r.RetransOK {
		return "no disponible"
	}
	if r.RetransEstimated {
		return "~" + strconv.FormatUint(r.Retrans, 10) + " (estimado)"
	}
	return strconv.FormatUint(r.Retrans, 10)
}

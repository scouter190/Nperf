package inet

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	"nperf/internal/probe"
	"nperf/internal/report"
)

// ReportFormatVersion es la versión de la forma JSON de la latencia a internet.
const ReportFormatVersion = 1

const (
	statusOK    = "ok"
	statusUnavl = "unavailable"
)

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// JSONError escribe {"error": "..."} igual que el resto de la CLI.
func JSONError(w io.Writer, err error) error { return report.JSONError(w, err) }

func fptr(ok bool, v float64) *float64 {
	if !ok {
		return nil
	}
	v = math.Round(v*1000) / 1000
	return &v
}

func statusOf(ok bool) string {
	if ok {
		return statusOK
	}
	return statusUnavl
}

type jsonStats struct {
	Sent         int      `json:"sent"`
	Received     int      `json:"received"`
	LossPct      float64  `json:"loss_pct"`
	MinMs        *float64 `json:"min_ms"`
	AvgMs        *float64 `json:"avg_ms"`
	MaxMs        *float64 `json:"max_ms"`
	RTTStatus    string   `json:"rtt_status"`
	JitterMs     *float64 `json:"jitter_ms"`
	JitterStatus string   `json:"jitter_status"`
	LastError    string   `json:"last_error,omitempty"`
}

func statsJSON(s probe.Stats) jsonStats {
	return jsonStats{
		Sent: s.Sent, Received: s.Received, LossPct: math.Round(s.LossPct*1000) / 1000,
		MinMs: fptr(s.RTTOK, s.MinMs), AvgMs: fptr(s.RTTOK, s.AvgMs), MaxMs: fptr(s.RTTOK, s.MaxMs),
		RTTStatus: statusOf(s.RTTOK), JitterMs: fptr(s.JitterOK, s.JitterMs),
		JitterStatus: statusOf(s.JitterOK), LastError: s.LastError,
	}
}

type jsonGateway struct {
	IP     *string `json:"ip"`
	Status string  `json:"status"` // ok | unavailable (sin respuesta o no detectada)
	*jsonStats
}

type jsonTarget struct {
	IP     string `json:"ip"`
	Status string `json:"status"` // ok | unavailable
	jsonStats
	TCP443       *bool  `json:"tcp443"` // null si no se probó
	TCP443Status string `json:"tcp443_status"`
}

type jsonHop struct {
	TTL    int     `json:"ttl"`
	From   *string `json:"from"`
	Status string  `json:"status"`
	jsonStats
}

type jsonTrace struct {
	Target  string    `json:"target"`
	Reached bool      `json:"reached"`
	Hops    []jsonHop `json:"hops"`
}

type jsonVerdict struct {
	Kind    string   `json:"kind"`
	Message string   `json:"message"`
	Notes   []string `json:"notes"`
}

type jsonReport struct {
	FormatVersion int          `json:"format_version"`
	Command       string       `json:"command"`
	Gateway       jsonGateway  `json:"gateway"`
	Targets       []jsonTarget `json:"targets"`
	Verdict       jsonVerdict  `json:"verdict"`
	Trace         *jsonTrace   `json:"trace"`
	TraceNotes    []string     `json:"trace_notes"`
}

func notes(n []string) []string {
	if n == nil {
		return []string{}
	}
	return n
}

// ReportJSON escribe el resultado de la latencia a internet como JSON v1.
func ReportJSON(w io.Writer, o Outcome) error {
	out := jsonReport{
		FormatVersion: ReportFormatVersion, Command: "internet",
		Targets:    []jsonTarget{},
		Verdict:    jsonVerdict{Kind: string(o.Verdict.Kind), Message: o.Verdict.Message, Notes: notes(o.Verdict.Notes)},
		TraceNotes: notes(o.TraceNotes),
	}
	gwOK := o.Gateway.IsValid() && o.GatewayStats.Received > 0
	out.Gateway = jsonGateway{Status: statusOf(gwOK)}
	if o.Gateway.IsValid() {
		s := o.Gateway.String()
		out.Gateway.IP = &s
		st := statsJSON(o.GatewayStats)
		out.Gateway.jsonStats = &st
	}
	for _, t := range o.Targets {
		jt := jsonTarget{IP: t.Addr.String(), Status: statusOf(t.Stats.Received > 0),
			jsonStats: statsJSON(t.Stats), TCP443Status: statusUnavl}
		if t.TCPTried {
			v := t.TCP443
			jt.TCP443, jt.TCP443Status = &v, statusOK
		}
		out.Targets = append(out.Targets, jt)
	}
	if o.Trace != nil {
		tr := &jsonTrace{Target: o.Trace.Dst.String(), Reached: o.Trace.Reached, Hops: []jsonHop{}}
		for _, h := range o.Trace.Hops {
			jh := jsonHop{TTL: h.TTL, Status: statusOf(h.From.IsValid()), jsonStats: statsJSON(h.Stats)}
			if h.From.IsValid() {
				s := h.From.String()
				jh.From = &s
			}
			tr.Hops = append(tr.Hops, jh)
		}
		out.Trace = tr
	}
	return encode(w, out)
}

func rttText(s probe.Stats) string {
	if s.Received == 0 || !s.RTTOK {
		return "no disponible"
	}
	return probe.FormatMs(s.AvgMs)
}

func lineFor(name string, s probe.Stats) string {
	if s.Received == 0 {
		return fmt.Sprintf("%s: sin respuesta (enviados %d)", name, s.Sent)
	}
	return fmt.Sprintf("%s: RTT medio %s, jitter %s, pérdida %.0f %%", name, rttText(s), s.FormatJitter(), s.LossPct)
}

// ReportText escribe el resultado de la latencia a internet en español.
func ReportText(w io.Writer, o Outcome) {
	fmt.Fprintln(w, "Latencia a internet")
	if o.Gateway.IsValid() {
		fmt.Fprintln(w, "  "+lineFor("Puerta de enlace "+o.Gateway.String(), o.GatewayStats))
	} else {
		fmt.Fprintln(w, "  Puerta de enlace: no detectada")
	}
	for _, t := range o.Targets {
		fmt.Fprintln(w, "  "+lineFor(t.Addr.String(), t.Stats))
		if t.TCPTried {
			if t.TCP443 {
				fmt.Fprintf(w, "    TCP 443: abre\n")
			} else {
				fmt.Fprintf(w, "    TCP 443: sin conexión\n")
			}
		}
	}
	fmt.Fprintf(w, "Veredicto: %s\n", o.Verdict.Message)
	for _, n := range o.Verdict.Notes {
		fmt.Fprintf(w, "- %s\n", n)
	}
	if o.Trace != nil {
		fmt.Fprintf(w, "Ruta a %s\n", o.Trace.Dst)
		for _, h := range o.Trace.Hops {
			from := "*"
			if h.From.IsValid() {
				from = h.From.String()
			}
			fmt.Fprintf(w, "%3d  %s  %s  pérdida %.0f %%\n", h.TTL, from, rttText(h.Stats), h.Stats.LossPct)
		}
	}
	for _, n := range o.TraceNotes {
		fmt.Fprintf(w, "- %s\n", n)
	}
}

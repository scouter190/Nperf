package probe

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/netip"
	"strings"

	"nperf/internal/report"
)

// ReportFormatVersion es la versión de la forma JSON de los diagnósticos.
const ReportFormatVersion = 1

const (
	statusOKJSON    = "ok"
	statusUnavailJS = "unavailable"
)

// Dest es el destino: nombre pedido por el usuario e IP usada.
type Dest struct {
	Name string
	IP   netip.Addr
}

func (t Dest) label() string {
	if t.Name == "" || t.Name == t.IP.String() {
		return t.IP.String()
	}
	return fmt.Sprintf("%s (%s)", t.Name, t.IP)
}

type jsonHeader struct {
	FormatVersion int    `json:"format_version"`
	Command       string `json:"command"`
	Host          string `json:"host"`
	IP            string `json:"ip"`
}

func header(cmd string, t Dest) jsonHeader {
	return jsonHeader{ReportFormatVersion, cmd, t.Name, t.IP.String()}
}

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// JSONError escribe {"error": "..."} igual que el resto de la CLI.
func JSONError(w io.Writer, err error) error {
	return report.JSONError(w, err)
}

func fptr(ok bool, v float64) *float64 {
	if !ok {
		return nil
	}
	v = math.Round(v*1000) / 1000
	return &v
}

func statusOf(ok bool) string {
	if ok {
		return statusOKJSON
	}
	return statusUnavailJS
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

func statsJSON(s Stats) jsonStats {
	return jsonStats{
		Sent: s.Sent, Received: s.Received, LossPct: math.Round(s.LossPct*1000) / 1000,
		MinMs: fptr(s.RTTOK, s.MinMs), AvgMs: fptr(s.RTTOK, s.AvgMs), MaxMs: fptr(s.RTTOK, s.MaxMs),
		RTTStatus: statusOf(s.RTTOK), JitterMs: fptr(s.JitterOK, s.JitterMs),
		JitterStatus: statusOf(s.JitterOK), LastError: s.LastError,
	}
}

func rttText(ok bool, ms float64) string {
	if !ok {
		return "no disponible"
	}
	return FormatMs(ms)
}

// ---- ping ----

type jsonReply struct {
	Seq    int      `json:"seq"`
	Status string   `json:"status"`
	From   string   `json:"from,omitempty"`
	RTTMs  *float64 `json:"rtt_ms"`
	Reason string   `json:"reason,omitempty"`
}

type jsonPing struct {
	jsonHeader
	jsonStats
	Replies []jsonReply `json:"replies"`
}

// PingJSON escribe el resultado de ping como JSON v1.
func PingJSON(w io.Writer, t Dest, s Stats, replies []Reply) error {
	out := jsonPing{jsonHeader: header("ping", t), jsonStats: statsJSON(s), Replies: []jsonReply{}}
	for i, r := range replies {
		jr := jsonReply{Seq: i + 1, Status: r.Status.String(), Reason: r.Reason}
		if r.From.IsValid() {
			jr.From = r.From.String()
		}
		jr.RTTMs = fptr(r.Status == StatusOK, r.RTTms)
		out.Replies = append(out.Replies, jr)
	}
	return encode(w, out)
}

// PingText escribe el resultado de ping en español.
func PingText(w io.Writer, t Dest, s Stats, replies []Reply) {
	fmt.Fprintf(w, "Ping a %s\n", t.label())
	for i, r := range replies {
		if r.Status == StatusOK {
			fmt.Fprintf(w, "  %d: respuesta de %s, %s\n", i+1, r.From, FormatMs(r.RTTms))
		} else {
			fmt.Fprintf(w, "  %d: %s\n", i+1, replyReason(r))
		}
	}
	fmt.Fprintf(w, "Enviados %d, recibidos %d, pérdida %.0f %%\n", s.Sent, s.Received, s.LossPct)
	fmt.Fprintf(w, "RTT mín/media/máx: %s / %s / %s\n",
		rttText(s.RTTOK, s.MinMs), rttText(s.RTTOK, s.AvgMs), rttText(s.RTTOK, s.MaxMs))
	fmt.Fprintf(w, "Jitter: %s\n", s.FormatJitter())
}

func replyReason(r Reply) string {
	if r.Reason != "" {
		return r.Reason
	}
	if r.Status == StatusTimeout {
		return "tiempo de espera agotado"
	}
	return "sin respuesta"
}

// ---- trace ----

type jsonHop struct {
	TTL    int         `json:"ttl"`
	From   *string     `json:"from"`
	Status string      `json:"status"`
	Probes []jsonProbe `json:"probes"`
	jsonStats
}

type jsonProbe struct {
	Status string   `json:"status"`
	RTTMs  *float64 `json:"rtt_ms"`
}

func probeResponded(r Reply) bool { return r.Status == StatusOK || r.Status == StatusTTLExpired }

type jsonTrace struct {
	jsonHeader
	Reached  bool      `json:"reached"`
	Hops     []jsonHop `json:"hops"`
	Findings []string  `json:"findings"`
}

// TraceJSON escribe el resultado de trace como JSON v1.
func TraceJSON(w io.Writer, t Dest, res TraceResult) error {
	out := jsonTrace{jsonHeader: header("trace", t), Reached: res.Reached, Hops: []jsonHop{}, Findings: Analyze(res)}
	if out.Findings == nil {
		out.Findings = []string{}
	}
	for _, h := range res.Hops {
		jh := jsonHop{TTL: h.TTL, Status: statusOf(h.From.IsValid()), jsonStats: statsJSON(h.Stats)}
		jh.Probes = []jsonProbe{}
		for _, p := range h.Probes {
			jh.Probes = append(jh.Probes, jsonProbe{Status: p.Status.String(), RTTMs: fptr(probeResponded(p), p.RTTms)})
		}
		if h.From.IsValid() {
			s := h.From.String()
			jh.From = &s
		}
		out.Hops = append(out.Hops, jh)
	}
	return encode(w, out)
}

// TraceText escribe el resultado de trace en español.
func TraceText(w io.Writer, t Dest, res TraceResult) {
	fmt.Fprintf(w, "Ruta a %s\n", t.label())
	for _, h := range res.Hops {
		var ps []string
		for _, p := range h.Probes {
			if probeResponded(p) {
				ps = append(ps, FormatMs(p.RTTms))
			} else {
				ps = append(ps, "*")
			}
		}
		from := "*"
		if h.From.IsValid() {
			from = h.From.String()
		}
		fmt.Fprintf(w, "%3d  %s  %s  pérdida %.0f %%\n", h.TTL, from, strings.Join(ps, "  "), h.Stats.LossPct)
	}
	if res.Reached {
		fmt.Fprintln(w, "Destino alcanzado.")
	} else {
		fmt.Fprintln(w, "Destino no alcanzado.")
	}
	for _, f := range Analyze(res) {
		fmt.Fprintf(w, "- %s\n", f)
	}
}

// ---- mtu ----

type jsonMTU struct {
	jsonHeader
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	MTUBytes          *int     `json:"mtu_bytes"`
	MaxPayloadBytes   *int     `json:"max_payload_bytes"`
	Probes            int      `json:"probes"`
	InterfaceMTUBytes int      `json:"interface_mtu_bytes"`
	Notes             []string `json:"notes"`
}

// MTUJSON escribe el resultado de MTU como JSON v1.
func MTUJSON(w io.Writer, t Dest, r MTUResult) error {
	out := jsonMTU{jsonHeader: header("mtu", t), Status: statusOf(r.Available), Reason: r.Reason,
		Probes: r.Probes, InterfaceMTUBytes: r.InterfaceMTU, Notes: r.Notes}
	if out.Notes == nil {
		out.Notes = []string{}
	}
	if r.Available {
		m, p := r.MTU, r.MaxPayload
		out.MTUBytes, out.MaxPayloadBytes = &m, &p
	}
	return encode(w, out)
}

// MTUText escribe el resultado de MTU en español.
func MTUText(w io.Writer, t Dest, r MTUResult) {
	fmt.Fprintf(w, "MTU de la ruta a %s\n", t.label())
	if !r.Available {
		fmt.Fprintf(w, "MTU: no disponible (%s)\n", r.Reason)
	} else {
		fmt.Fprintf(w, "MTU: %d bytes (carga ICMP máxima %d bytes)\n", r.MTU, r.MaxPayload)
	}
	fmt.Fprintf(w, "Sondas enviadas: %d\n", r.Probes)
	for _, n := range r.Notes {
		fmt.Fprintf(w, "- %s\n", n)
	}
}

// ---- ports ----

type jsonPorts struct {
	jsonHeader
	Ports []PortResult `json:"ports"`
}

// PortsJSON escribe el resultado del sondeo de puertos como JSON v1.
func PortsJSON(w io.Writer, t Dest, res []PortResult) error {
	if res == nil {
		res = []PortResult{}
	}
	return encode(w, jsonPorts{header("ports", t), res})
}

func portStateText(s PortState) string {
	switch s {
	case PortOpen:
		return "abierto"
	case PortClosed:
		return "cerrado"
	case PortFiltered:
		return "filtrado"
	case PortCanceled:
		return "cancelado"
	}
	return "error"
}

// PortsText escribe el resultado del sondeo de puertos en español.
func PortsText(w io.Writer, t Dest, res []PortResult) {
	fmt.Fprintf(w, "Puertos TCP de %s\n", t.label())
	for _, p := range res {
		line := fmt.Sprintf("  %d/tcp  %s", p.Port, portStateText(p.State))
		if p.Detail != "" {
			line += " (" + strings.TrimSpace(p.Detail) + ")"
		}
		fmt.Fprintln(w, line)
	}
}

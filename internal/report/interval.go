package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"nperf/internal/client"
)

// IntervalLine da formato a una muestra de 1 s para la consola, en minúsculas y sin las
// cadenas "TCP fwd" ni "RTT:" (el e2e comprueba su orden en stdout). phase es "tcp_fwd",
// "udp_rev"...; ej.: "  tcp ida 3-4 s: 941.2 Mbps, rtt 1.3 ms".
func IntervalLine(phase string, iv client.Interval) string {
	proto, dir, _ := strings.Cut(phase, "_")
	switch dir {
	case "fwd":
		dir = "ida"
	case "rev":
		dir = "vuelta"
	}
	end := iv.Start + iv.Duration
	s := fmt.Sprintf("  %s %s %d-%d s: %.1f Mbps", proto, dir, secs(iv.Start), secs(end), iv.Mbps)
	if iv.UDP {
		s += fmt.Sprintf(", pérdida %.2f %%, jitter %.3f ms", iv.LossPct, ms(iv.Jitter))
	}
	if iv.HasRTT {
		s += ", rtt " + FmtRTT(iv.RTT)
	}
	return s
}

func secs(d time.Duration) int { return int(math.Round(d.Seconds())) }

// JSONInterval es una muestra de 1 s. Los campos UDP son null en TCP.
type JSONInterval struct {
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

// JSONLoadedRTT resume el RTT bajo carga; con status "unavailable" los valores son null.
type JSONLoadedRTT struct {
	Status   string   `json:"status"`
	MinMS    *float64 `json:"min_ms"`
	AvgMS    *float64 `json:"avg_ms"`
	MedianMS *float64 `json:"median_ms"`
	MaxMS    *float64 `json:"max_ms"`
	Samples  int      `json:"samples"`
}

func jsonIntervals(d client.Dir) []JSONInterval {
	if len(d.Intervals) == 0 {
		return nil
	}
	out := make([]JSONInterval, 0, len(d.Intervals))
	for _, iv := range d.Intervals {
		j := JSONInterval{TS: iv.Start.Seconds(), DurationS: iv.Duration.Seconds(), Bytes: iv.Bytes, ThroughputMbps: iv.Mbps}
		if iv.HasRTT {
			j.RTTMS = ptr(ms(iv.RTT))
		}
		if iv.UDP {
			j.DatagramsReceived, j.DatagramsLost = ptr(iv.Received), ptr(iv.Lost)
			j.LossPct, j.JitterMS = ptr(iv.LossPct), ptr(ms(iv.Jitter))
		}
		out = append(out, j)
	}
	return out
}

func jsonLoadedRTT(d client.Dir) *JSONLoadedRTT {
	l := d.LoadedRTT
	if l == nil {
		return &JSONLoadedRTT{Status: StatusUnavailable}
	}
	return &JSONLoadedRTT{Status: StatusAvailable, MinMS: ptr(ms(l.Min)), AvgMS: ptr(ms(l.Avg)),
		MedianMS: ptr(ms(l.Median)), MaxMS: ptr(ms(l.Max)), Samples: l.Samples}
}

package report_test

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/report"
)

func liveResult() (report.Params, client.Result) {
	p, res := fullResult()
	res.Fwd.Live = true
	res.Fwd.Intervals = []client.Interval{
		{Index: 0, Start: 0, Duration: time.Second, Bytes: 125000000, Mbps: 1000, RTT: ms(12), HasRTT: true},
		{Index: 1, Start: time.Second, Duration: time.Second, Bytes: 120000000, Mbps: 960},
	}
	res.Fwd.LoadedRTT = &client.LoadedRTT{Min: ms(10), Avg: ms(15), Median: ms(12), Max: ms(30), Samples: 1, Sent: 2}
	res.Rev.Live = true // medido sin muestras: unavailable
	res.UDPFwd.Live = true
	res.UDPFwd.Intervals = []client.Interval{
		{Index: 0, Start: 0, Duration: time.Second, Bytes: 11000000, Mbps: 88, UDP: true, Received: 7000,
			Expected: 7100, Lost: 100, LossPct: 1.41, Jitter: 125 * time.Microsecond, RTT: ms(300), HasRTT: true},
	}
	res.UDPFwd.LoadedRTT = &client.LoadedRTT{Min: ms(300), Avg: ms(300), Median: ms(300), Max: ms(300), Samples: 1, Sent: 1}
	res.Notes = append(res.Notes, client.BufferbloatNote(ms(1), res.UDPFwd.LoadedRTT, "ida"))
	return p, res
}

func TestGoldenLive(t *testing.T) {
	p, res := liveResult()
	var buf bytes.Buffer
	if err := report.JSON(&buf, p, res); err != nil {
		t.Fatal(err)
	}
	const path = "testdata/report_v1_live.golden.json"
	if *update {
		os.WriteFile(path, buf.Bytes(), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("JSON distinto del golden file:\n%s", buf.String())
	}
	s := buf.String()
	if !strings.Contains(s, "bufferbloat") {
		t.Error("falta la nota de bufferbloat")
	}
	m := decode(t, p, res)
	if get(t, m, "tcp", "rev", "loaded_rtt", "status") != "unavailable" || get(t, m, "tcp", "rev", "intervals") != nil {
		t.Error("sin muestras: intervals null y loaded_rtt unavailable")
	}
	if get(t, m, "udp", "rev", "loaded_rtt") != nil || get(t, m, "udp", "rev", "intervals") != nil {
		t.Error("not_measured: intervals y loaded_rtt null")
	}
	if get(t, m, "format_version") != float64(1) {
		t.Error("format_version debe seguir en 1")
	}
}

func TestIntervalLine(t *testing.T) {
	iv := client.Interval{Start: 3 * time.Second, Duration: time.Second, Mbps: 941.2, RTT: 1300 * time.Microsecond, HasRTT: true}
	if got, want := report.IntervalLine("tcp_fwd", iv), "  tcp ida 3-4 s: 941.2 Mbps, rtt 1.30 ms"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	iv = client.Interval{Duration: time.Second, Mbps: 88, UDP: true, LossPct: 1.5, Jitter: 125 * time.Microsecond}
	got := report.IntervalLine("udp_rev", iv)
	if got != "  udp vuelta 0-1 s: 88.0 Mbps, pérdida 1.50 %, jitter 0.125 ms" {
		t.Errorf("udp: %q", got)
	}
	for _, l := range []string{got, report.IntervalLine("tcp_fwd", client.Interval{Duration: time.Second})} {
		if strings.Contains(l, "TCP fwd") || strings.Contains(l, "RTT:") || strings.Contains(l, "rtt") && !strings.Contains(l, "rtt ") {
			t.Errorf("línea con cadenas prohibidas: %q", l)
		}
	}
}

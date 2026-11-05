package report_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/meter"
	"nperf/internal/report"
)

var update = flag.Bool("update", false, "reescribe el golden file")

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func tcpDir(mbps float64, retrans meter.Result) client.Dir {
	return client.Dir{Measured: true, Mbps: mbps, Sent: retrans,
		Received: meter.Result{Bytes: 1250000000, Elapsed: 10 * time.Second}}
}

func udpDir(sent, recv float64) client.Dir {
	return client.Dir{Measured: true, Mbps: recv, Sent: meter.Result{Bytes: 112500000},
		Received: meter.Result{Bytes: 111000000, Elapsed: 10 * time.Second},
		UDP:      &client.UDPResult{SentMbps: sent, Packets: 80000, Expected: 80000, Lost: 800, LossPct: 1, JitterMS: 0.125}}
}

// fullResult: valores inyectados y deterministas de una medición completa.
func fullResult() (report.Params, client.Result) {
	p := report.Params{Host: "192.168.1.10", Port: 5300, Proto: "both", Direction: "both",
		Duration: 10 * time.Second, Streams: 4, Version: "0.1.0"}
	res := client.Result{
		RTT:        &client.RTT{Min: 500 * time.Microsecond, Avg: ms(1), Max: ms(2), Samples: 10},
		Fwd:        tcpDir(1000, meter.Result{Retrans: 12, RetransOK: true}),
		Rev:        tcpDir(900, meter.Result{}), // retransmisiones no disponibles
		UDPFwd:     udpDir(900, 890),
		UDPRev:     client.Dir{}, // no medido
		UDPRateFwd: 900e6, UDPRateRev: 0,
		Notes: []string{"UDP rev no medido: prueba"},
	}
	return p, res
}

func TestGolden(t *testing.T) {
	p, res := fullResult()
	var buf bytes.Buffer
	if err := report.JSON(&buf, p, res); err != nil {
		t.Fatal(err)
	}
	const path = "testdata/report_v1.golden.json"
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
}

func decode(t *testing.T, p report.Params, res client.Result) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := report.JSON(&buf, p, res); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	return m
}

func get(t *testing.T, m map[string]any, path ...string) any {
	t.Helper()
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("%v: no es objeto", path)
		}
		v, ok := mm[k]
		if !ok {
			t.Fatalf("falta el campo %v", path)
		}
		cur = v
	}
	return cur
}

func TestJSONAllFieldsAndExplicitStates(t *testing.T) {
	p, res := fullResult()
	m := decode(t, p, res)
	for _, path := range [][]string{
		{"format_version"}, {"params", "host"}, {"params", "port"}, {"params", "proto"}, {"params", "direction"},
		{"params", "duration_s"}, {"params", "streams"}, {"params", "udp_rate_bps"}, {"params", "version"},
		{"rtt", "status"}, {"rtt", "min_ms"}, {"rtt", "avg_ms"}, {"rtt", "max_ms"}, {"rtt", "samples"},
		{"tcp", "fwd", "throughput_mbps"}, {"tcp", "fwd", "bytes"}, {"tcp", "fwd", "duration_s"},
		{"tcp", "fwd", "retransmissions", "status"}, {"tcp", "fwd", "retransmissions", "count"}, {"tcp", "fwd", "retransmissions", "estimated"},
		{"udp", "fwd", "udp", "sent_mbps"}, {"udp", "fwd", "udp", "sent_bytes"}, {"udp", "fwd", "udp", "target_rate_bps"},
		{"udp", "fwd", "udp", "jitter_ms"}, {"udp", "fwd", "udp", "loss_pct"}, {"udp", "fwd", "udp", "datagrams_sent"},
		{"udp", "fwd", "udp", "datagrams_expected"}, {"udp", "fwd", "udp", "datagrams_lost"},
		{"udp", "fwd", "throughput_mbps"}, {"notes"},
	} {
		get(t, m, path...)
	}
	if get(t, m, "format_version") != float64(1) {
		t.Error("format_version != 1")
	}
	// "no disponible" explícito, no 0.
	if get(t, m, "tcp", "rev", "retransmissions", "status") != "unavailable" || get(t, m, "tcp", "rev", "retransmissions", "count") != nil {
		t.Error("retransmisiones no disponibles mal representadas")
	}
	if get(t, m, "tcp", "fwd", "retransmissions", "status") != "available" {
		t.Error("retransmisiones disponibles mal representadas")
	}
	// "no medido" explícito.
	if get(t, m, "udp", "rev", "status") != "not_measured" || get(t, m, "udp", "rev", "throughput_mbps") != nil {
		t.Error("UDP rev debería ser not_measured con null")
	}
	// Retransmisión estimada.
	res.Fwd.Sent = meter.Result{Retrans: 3, RetransOK: true, RetransEstimated: true}
	if get(t, decodeM(t, p, res), "estimated") != true {
		t.Error("estimated debería ser true")
	}
	// Proto tcp: RTT y UDP no medidos.
	p.Proto = "tcp"
	m = decode(t, p, res)
	if get(t, m, "rtt", "status") != "not_measured" || get(t, m, "udp", "fwd", "status") != "not_measured" {
		t.Error("proto tcp: RTT/UDP deberían ser not_measured")
	}
	// Proto udp: UDP viene de Fwd/Rev; objetivo por defecto 100 Mbps.
	p.Proto = "udp"
	res.Fwd = udpDir(95, 94)
	m = decode(t, p, res)
	if get(t, m, "udp", "fwd", "status") != "measured" || get(t, m, "udp", "fwd", "udp", "target_rate_bps") != 100e6 ||
		get(t, m, "tcp", "fwd", "status") != "not_measured" {
		t.Error("proto udp mal representado")
	}
}

func decodeM(t *testing.T, p report.Params, res client.Result) map[string]any {
	m := decode(t, p, res)
	return get(t, m, "tcp", "fwd", "retransmissions").(map[string]any)
}

func TestJSONError(t *testing.T) {
	var buf bytes.Buffer
	report.JSONError(&buf, errors.New("fallo"))
	var m map[string]string
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil || m["error"] != "fallo" || len(m) != 1 {
		t.Errorf("%q %v", buf.String(), err)
	}
}

func TestTextOrder(t *testing.T) {
	p, res := fullResult()
	var buf bytes.Buffer
	report.Text(&buf, p, res)
	out := buf.String()
	last := -1
	for _, k := range []string{"RTT:", "TCP fwd", "TCP rev", "UDP fwd", "UDP rev", "nota:"} {
		i := strings.Index(out, k)
		if i < 0 || i < last {
			t.Fatalf("orden incorrecto en %q (%s)", k, out)
		}
		last = i
	}
	for _, k := range []string{"retransmisiones: 12", "retransmisiones: no disponible", "UDP rev (servidor -> cliente): no medido"} {
		if !strings.Contains(out, k) {
			t.Errorf("falta %q en %s", k, out)
		}
	}
}

func TestRateNote(t *testing.T) {
	mk := func(sent float64) client.Dir {
		return client.Dir{Measured: true, UDP: &client.UDPResult{SentMbps: sent}}
	}
	if got, want := report.RateNote("fwd", mk(50), 100e6), "UDP ida: no se alcanzó la tasa objetivo (enviado 50.0 de 100.0 Mbps)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := report.RateNote("rev", mk(90), 100e6); got != "" {
		t.Errorf("90 %% exacto no debe avisar: %q", got)
	}
	if got := report.RateNote("rev", client.Dir{}, 100e6); got != "" {
		t.Errorf("no medido: %q", got)
	}
	if got := report.RateNote("rev", mk(1), 0); got != "" {
		t.Errorf("sin objetivo: %q", got)
	}
}

func TestFmtRTT(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "<1 µs", 999: "<1 µs", 1500 * time.Microsecond: "1.50 ms"} {
		if got := report.FmtRTT(d); got != want {
			t.Errorf("%v -> %q, quería %q", d, got, want)
		}
	}
}

func TestFmtRetrans(t *testing.T) {
	if got := report.FmtRetrans(meter.Result{}); got != "no disponible" {
		t.Errorf("sin datos: %q", got)
	}
	if got := report.FmtRetrans(meter.Result{Retrans: 7, RetransOK: true, RetransEstimated: true}); got != "~7 (estimado)" {
		t.Errorf("estimado: %q", got)
	}
	if got := report.FmtRetrans(meter.Result{Retrans: 7, RetransOK: true}); got != "7" {
		t.Errorf("con datos: %q", got)
	}
}

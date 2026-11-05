package main

import (
	"nperf/internal/meter"
	"testing"
	"time"
)

func TestUDPRateWithTCPIsUsageError(t *testing.T) {
	if code := run([]string{"client", "127.0.0.1", "-proto", "tcp", "-udp-rate", "50M"}); code != 2 {
		t.Fatalf("código %d, quería 2", code)
	}
}

func TestUDPRateAcceptedWithBoth(t *testing.T) {
	// Puerto cerrado: llega a conectar (código 1), no es error de uso (2).
	if code := run([]string{"client", "127.0.0.1", "-port", "1", "-udp-rate", "50M", "-duration", "3"}); code != 1 {
		t.Fatalf("código %d, quería 1", code)
	}
}

func TestFmtRTT(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "<1 µs", 999: "<1 µs", 1500 * time.Microsecond: "1.50 ms"} {
		if got := fmtRTT(d); got != want {
			t.Errorf("%v -> %q, quería %q", d, got, want)
		}
	}
}

func TestFmtRetrans(t *testing.T) {
	if got := fmtRetrans(meter.Result{}); got != "no disponible" {
		t.Errorf("sin datos: %q", got)
	}
	if got := fmtRetrans(meter.Result{Retrans: 7, RetransOK: true, RetransEstimated: true}); got != "~7 (estimado)" {
		t.Errorf("estimado: %q", got)
	}
	if got := fmtRetrans(meter.Result{Retrans: 7, RetransOK: true}); got != "7" {
		t.Errorf("con datos: %q", got)
	}
}

package main

import "testing"

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

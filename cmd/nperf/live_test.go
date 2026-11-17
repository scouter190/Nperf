package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestHelpHidesNoLoadRTT(t *testing.T) {
	out, _ := capture(t, t.TempDir())
	if code := run([]string{"client", "-h"}); code != 0 {
		t.Fatalf("código %d", code)
	}
	if strings.Contains(out.String(), "no-load-rtt") {
		t.Errorf("la ayuda lista el flag oculto: %s", out)
	}
}

func TestLiveLinesText(t *testing.T) {
	out, _ := capture(t, t.TempDir())
	port := startServer(t, "")
	if code := run([]string{"client", "127.0.0.1", "-port", port, "-proto", "tcp", "-direction", "fwd", "-duration", "3"}); code != 0 {
		t.Fatalf("código %d: %s", code, out)
	}
	n := len(regexp.MustCompile(`(?m)^  tcp ida \d+-\d+ s: .* Mbps`).FindAllString(out.String(), -1))
	if n != 3 {
		t.Errorf("%d líneas por intervalo, quería 3: %s", n, out)
	}
}

func TestNoLoadRTTJSON(t *testing.T) {
	out, _ := capture(t, t.TempDir())
	port := startServer(t, "")
	args := []string{"client", "127.0.0.1", "-port", port, "-proto", "tcp", "-direction", "fwd", "-duration", "3", "-json", "-no-load-rtt"}
	if code := run(args); code != 0 {
		t.Fatalf("código %d: %s", code, out)
	}
	var v struct {
		TCP struct {
			Fwd struct {
				Intervals []struct {
					RTT *float64 `json:"rtt_ms"`
				} `json:"intervals"`
				LoadedRTT struct {
					Status string `json:"status"`
				} `json:"loaded_rtt"`
			} `json:"fwd"`
		} `json:"tcp"`
	}
	dec := json.NewDecoder(out)
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("stdout no es un objeto JSON: %v: %s", err, out)
	}
	if dec.More() {
		t.Errorf("stdout lleva más de un objeto: %s", out)
	}
	if v.TCP.Fwd.LoadedRTT.Status != "unavailable" || len(v.TCP.Fwd.Intervals) == 0 {
		t.Fatalf("loaded_rtt/intervals inesperados: %+v", v.TCP.Fwd)
	}
	for _, iv := range v.TCP.Fwd.Intervals {
		if iv.RTT != nil {
			t.Errorf("rtt_ms no nulo con -no-load-rtt")
		}
	}
}

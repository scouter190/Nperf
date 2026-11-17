package probe

import (
	"strings"
	"testing"

	"nperf/internal/netinfo"
)

func TestDiagnose(t *testing.T) {
	sig := 42
	ok := func(loss, avg float64) Stats {
		return Stats{Sent: 10, Received: 10, LossPct: loss, RTTOK: true, AvgMs: avg}
	}
	tests := []struct {
		name    string
		in      CombinedInput
		want    []string
		healthy bool
	}{
		{"sin datos", CombinedInput{Dest: "d"}, []string{"sin datos"}, false},
		{"wifi perdida", CombinedInput{Dest: "d", Hop1: ok(5, 3), Kind: netinfo.KindWiFi, SignalPct: &sig}, []string{"tramo WiFi", "señal 42 %"}, false},
		{"wifi latencia", CombinedInput{Dest: "d", Hop1: ok(0, 20.1), Kind: netinfo.KindWiFi}, []string{"tramo WiFi", "no disponible"}, false},
		{"cable perdida", CombinedInput{Dest: "d", Hop1: ok(10, 1), Kind: netinfo.KindEthernet}, []string{"cable, switch o gateway"}, false},
		{"sin respuestas", CombinedInput{Dest: "d", Hop1: Stats{Sent: 4, LossPct: 100}, Kind: netinfo.KindEthernet}, []string{"cable, switch"}, false},
		{"otro tipo", CombinedInput{Dest: "d", Hop1: ok(50, 1), Kind: netinfo.KindOther}, []string{"desconocido"}, false},
		{"sano limite", CombinedInput{Dest: "10.0.0.1", Hop1: ok(0, 20), Kind: netinfo.KindWiFi}, []string{"enlace sano hasta 10.0.0.1"}, true},
		{"sano wifi", CombinedInput{Dest: "d", Hop1: ok(0, 2), Kind: netinfo.KindWiFi, SignalPct: &sig}, []string{"enlace sano hasta d"}, true},
		{"perdida 4.9 no degrada", CombinedInput{Dest: "d", Hop1: ok(4.9, 2), Kind: netinfo.KindWiFi}, []string{"no muestra un problema"}, false},
		{"mtu reducido y sano", CombinedInput{Dest: "d", Hop1: ok(0, 2), MTU: 1400}, []string{"enlace sano", "MTU reducido en la ruta (1400"}, true},
		{"mtu 1500", CombinedInput{Dest: "d", Hop1: ok(0, 2), MTU: 1500}, []string{"enlace sano"}, true},
		{"mtu desconocido", CombinedInput{Dest: "d", Hop1: ok(0, 2)}, []string{"enlace sano"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Diagnose(tc.in)
			all := strings.Join(got.Messages, " | ")
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("falta %q en %q", w, all)
				}
			}
			if got.Healthy != tc.healthy {
				t.Errorf("Healthy = %v, quiero %v (%q)", got.Healthy, tc.healthy, all)
			}
			if !tc.healthy && strings.Contains(all, "enlace sano") {
				t.Errorf("no debía decir sano: %q", all)
			}
		})
	}
}

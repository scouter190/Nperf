package diag

import (
	"strings"
	"testing"
)

const M = 1_000_000

func wired(a string, tx, rx uint64) End {
	return End{Alias: a, Known: true, Kind: KindWired, TxBps: tx, RxBps: rx, Signal: -1}
}
func wifi(a string, tx, rx uint64, sig int) End {
	return End{Alias: a, Known: true, Kind: KindWiFi, TxBps: tx, RxBps: rx, Signal: sig}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name     string
		thr      float64
		s, r     End
		color    Color
		pct      int
		partial  bool
		over     bool
		both     bool
		limiter  string
		contains []string
	}{
		{"cable verde justo sobre 80", 80.1 * M, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Green, 80, false, false, true, "", nil},
		{"cable exacto 80 ambar", 80 * M, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Amber, 80, false, false, true, "", []string{"posible cuello en la red intermedia"}},
		{"cable exacto 50 ambar", 50 * M, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Amber, 50, false, false, true, "", nil},
		{"cable bajo 50 rojo", 49.9 * M, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Red, 50, false, false, true, "", []string{"Por debajo del enlace más lento"}},
		{"wifi sobre 50 verde", 50.1 * M, wifi("A", 100*M, 100*M, 70), wired("B", 1000*M, 1000*M), Green, 50, false, false, false, "A", nil},
		{"wifi exacto 50 ambar", 50 * M, wifi("A", 100*M, 100*M, 70), wired("B", 1000*M, 1000*M), Amber, 50, false, false, false, "A", nil},
		{"wifi exacto 30 ambar", 30 * M, wifi("A", 100*M, 100*M, 70), wired("B", 1000*M, 1000*M), Amber, 30, false, false, false, "A", nil},
		{"wifi bajo 30 rojo", 29.9 * M, wifi("A", 100*M, 100*M, 70), wired("B", 1000*M, 1000*M), Red, 30, false, false, false, "A", nil},
		{"nominal min tx emisor rx receptor: limita receptor wifi", 60 * M, wired("A", 1000*M, 1000*M), wifi("B", 144*M, 144*M, 42), Amber, 42, false, false, false, "B", []string{"Limitado por el WiFi de B (enlace 144 Mbps, señal 42 %)"}},
		{"nominal ignora rx emisor y tx receptor", 90 * M, wired("A", 100*M, 10*M), wired("B", 10*M, 1000*M), Green, 90, false, false, false, "A", nil},
		{"empate ambos extremos", 90 * M, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Green, 90, false, false, true, "", []string{"Limitado por ambos extremos (enlace 100 Mbps)"}},
		{"empate con wifi usa umbral wifi", 40 * M, wired("A", 100*M, 100*M), wifi("B", 100*M, 100*M, 50), Amber, 40, false, false, true, "", nil},
		{"throughput mayor que nominal", 150 * M, wired("A", 100*M, 100*M), wired("B", 1000*M, 1000*M), Green, 100, false, true, false, "A", []string{"se muestra 100 %"}},
		{"emisor sin datos parcial", 90 * M, End{Alias: "A"}, wired("B", 100*M, 100*M), Green, 90, true, false, false, "B", []string{"parcial"}},
		{"receptor sin datos parcial", 20 * M, wired("A", 100*M, 100*M), End{Alias: "B", Known: true}, Red, 20, true, false, false, "A", []string{"parcial"}},
		{"velocidad cero cuenta como sin datos", 90 * M, wired("A", 0, 0), wired("B", 100*M, 100*M), Green, 90, true, false, false, "B", nil},
		{"ninguno sin semaforo", 90 * M, End{}, End{}, None, 0, false, false, false, "", []string{"Sin datos de enlace"}},
		{"no medido sin semaforo", -1, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), None, 0, false, false, false, "", []string{"no medido"}},
		{"throughput cero medido rojo", 0, wired("A", 100*M, 100*M), wired("B", 100*M, 100*M), Red, 0, false, false, true, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Evaluate(c.thr, c.s, c.r)
			if r.Color != c.color || r.Percent != c.pct || r.Partial != c.partial || r.Over != c.over || r.Both != c.both || r.Limiter != c.limiter {
				t.Fatalf("got %+v", r)
			}
			for _, s := range c.contains {
				if !strings.Contains(r.Text, s) {
					t.Errorf("texto %q no contiene %q", r.Text, s)
				}
			}
		})
	}
}

func TestFormatBps(t *testing.T) {
	for in, want := range map[uint64]string{100 * M: "100 Mbps", 1000 * M: "1 Gbps", 2500 * M: "2.5 Gbps", 144 * M: "144 Mbps"} {
		if got := FormatBps(in); got != want {
			t.Errorf("%d: %q != %q", in, got, want)
		}
	}
}

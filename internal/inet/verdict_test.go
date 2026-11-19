package inet

import (
	"net/netip"
	"reflect"
	"testing"

	"nperf/internal/probe"
)

func st(loss, avg, jit float64) probe.Stats {
	return probe.Stats{Sent: 100, Received: 100, LossPct: loss, RTTOK: true, AvgMs: avg, JitterOK: true, JitterMs: jit}
}

var dead = probe.Stats{Sent: 4}

func TestVerdict(t *testing.T) {
	ok := st(0, 10, 1)
	tests := []struct {
		name string
		gw   probe.Stats
		in   []probe.Stats
		tcp  []bool
		want Kind
		deg  int
		note bool
	}{
		{"sano", ok, []probe.Stats{ok, ok}, nil, Healthy, -1, false},
		{"gateway 20 ms sano", st(0, 20, 0), []probe.Stats{ok}, nil, Healthy, -1, false},
		{"gateway 20,1 ms LAN", st(0, 20.1, 0), []probe.Stats{ok}, nil, LAN, -1, false},
		{"gateway con pérdida LAN", st(0.5, 5, 0), []probe.Stats{ok}, nil, LAN, -1, false},
		{"LAN aunque internet caído", st(10, 5, 0), []probe.Stats{dead}, []bool{false}, LAN, -1, false},
		{"proveedor pérdida 2 %", ok, []probe.Stats{ok, st(2, 10, 1)}, nil, Provider, 1, false},
		{"pérdida 1,99 % sano", ok, []probe.Stats{st(1.99, 10, 1)}, nil, Healthy, -1, false},
		{"RTT 100 sano", ok, []probe.Stats{st(0, 100, 1)}, nil, Healthy, -1, false},
		{"RTT 100,1 proveedor", ok, []probe.Stats{st(0, 100.1, 1)}, nil, Provider, 0, false},
		{"jitter 30 sano", ok, []probe.Stats{st(0, 10, 30)}, nil, Healthy, -1, false},
		{"jitter 30,1 proveedor", ok, []probe.Stats{st(0, 10, 30.1)}, nil, Provider, 0, false},
		{"jitter no disponible", ok, []probe.Stats{{Sent: 4, Received: 4, RTTOK: true, AvgMs: 10, JitterMs: 99}}, nil, Healthy, -1, false},
		{"un destino responde, otro no", ok, []probe.Stats{dead, ok}, []bool{true, true}, Healthy, -1, false},
		{"sin salida", ok, []probe.Stats{dead, dead}, []bool{false, false}, NoInternet, -1, false},
		{"sin salida sin tcp", ok, []probe.Stats{dead}, nil, NoInternet, -1, false},
		{"ICMP filtrado", ok, []probe.Stats{dead, dead}, []bool{false, true}, ICMPFiltered, -1, false},
		{"gateway mudo, internet sano", dead, []probe.Stats{ok}, nil, Healthy, -1, true},
		{"gateway mudo, internet degradado", probe.Stats{}, []probe.Stats{st(5, 10, 1)}, nil, Provider, 0, true},
		{"gateway mudo, sin salida", dead, []probe.Stats{dead}, []bool{false}, NoInternet, -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Verdict(tc.gw, tc.in, tc.tcp)
			if r.Kind != tc.want || r.Degraded != tc.deg || (len(r.Notes) > 0) != tc.note {
				t.Fatalf("got %+v, want kind=%s deg=%d note=%v", r, tc.want, tc.deg, tc.note)
			}
			if r.Message == "" {
				t.Fatal("mensaje vacío")
			}
		})
	}
}

func TestParseTargets(t *testing.T) {
	def, err := ParseTargets(nil)
	if err != nil || !reflect.DeepEqual(def, []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")}) {
		t.Fatalf("defecto: %v %v", def, err)
	}
	good, err := ParseTargets([]string{" 9.9.9.9 ", "9.9.9.9", "208.67.222.222"})
	if err != nil || len(good) != 2 {
		t.Fatalf("válidos: %v %v", good, err)
	}
	if _, err := ParseTargets([]string{"1.1.1.1", "8.8.8.8", "9.9.9.9", "4.4.4.4"}); err != nil {
		t.Fatalf("4 deben valer: %v", err)
	}
	bad := [][]string{
		{"1.1.1.1", "8.8.8.8", "9.9.9.9", "4.4.4.4", "208.67.222.222"},
		{"192.168.1.1"}, {"10.0.0.1"}, {"172.16.0.1"}, {"127.0.0.1"}, {"169.254.1.1"},
		{"224.0.0.1"}, {"255.255.255.255"}, {"0.0.0.0"}, {"100.64.0.1"}, {"::1"},
		{"2606:4700:4700::1111"}, {"example.com"}, {""}, {"1.1.1.1/32"}, {"1.1.1.1:53"},
	}
	for _, b := range bad {
		if _, err := ParseTargets(b); err == nil {
			t.Errorf("%v debía rechazarse", b)
		}
	}
}

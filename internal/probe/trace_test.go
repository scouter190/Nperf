package probe

import (
	"context"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeTraceEcho struct {
	dst      netip.Addr
	destTTL  int // TTL desde el que llega el destino (0 = nunca)
	silent   map[int]bool
	inflight int32
	maxSeen  int32
	calls    int
	cancel   func()
	cancelAt int
}

func (f *fakeTraceEcho) Echo(ctx context.Context, req EchoRequest) (Reply, error) {
	n := atomic.AddInt32(&f.inflight, 1)
	if n > f.maxSeen {
		f.maxSeen = n
	}
	defer atomic.AddInt32(&f.inflight, -1)
	f.calls++
	if f.cancel != nil && f.calls == f.cancelAt {
		f.cancel()
	}
	if f.silent[req.TTL] {
		return Reply{Status: StatusTimeout}, nil
	}
	if f.destTTL > 0 && req.TTL >= f.destTTL {
		return Reply{Status: StatusOK, From: f.dst, RTTms: 5}, nil
	}
	return Reply{Status: StatusTTLExpired, From: netip.AddrFrom4([4]byte{10, 0, 0, byte(req.TTL)}), RTTms: 2}, nil
}

func TestTraceStopsAtDestination(t *testing.T) {
	dst := netip.MustParseAddr("8.8.8.8")
	f := &fakeTraceEcho{dst: dst, destTTL: 3}
	res, err := Trace(context.Background(), f, dst, TraceOptions{MaxHops: 30, Timeout: DefaultTimeout})
	if err != nil || !res.Reached || len(res.Hops) != 3 || f.calls != 9 {
		t.Fatalf("err=%v reached=%v hops=%d calls=%d", err, res.Reached, len(res.Hops), f.calls)
	}
	if f.maxSeen != 1 {
		t.Fatalf("sondas simultáneas: %d", f.maxSeen)
	}
	if res.Hops[2].From != dst || res.Hops[0].From.String() != "10.0.0.1" {
		t.Fatalf("hops: %+v", res.Hops)
	}
}

func TestTraceStopsAtMaxHopsAndStars(t *testing.T) {
	dst := netip.MustParseAddr("8.8.8.8")
	f := &fakeTraceEcho{dst: dst, silent: map[int]bool{2: true}}
	res, err := Trace(context.Background(), f, dst, TraceOptions{MaxHops: 4, Timeout: DefaultTimeout})
	if err != nil || res.Reached || len(res.Hops) != 4 || f.calls != 12 {
		t.Fatalf("err=%v reached=%v hops=%d calls=%d", err, res.Reached, len(res.Hops), f.calls)
	}
	h := res.Hops[1]
	if h.From.IsValid() || len(h.Probes) != 3 || h.Stats.LossPct != 100 || h.Stats.RTTOK {
		t.Fatalf("salto *: %+v", h)
	}
}

func TestTraceCancelAndInvalid(t *testing.T) {
	dst := netip.MustParseAddr("8.8.8.8")
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeTraceEcho{dst: dst, cancel: cancel, cancelAt: 4}
	res, err := Trace(ctx, f, dst, DefaultTrace())
	if err == nil || f.calls != 4 || len(res.Hops) != 1 {
		t.Fatalf("err=%v calls=%d hops=%d", err, f.calls, len(res.Hops))
	}
	f2 := &fakeTraceEcho{dst: dst}
	if _, err := Trace(context.Background(), f2, dst, TraceOptions{MaxHops: 0, Timeout: DefaultTimeout}); err == nil || f2.calls != 0 {
		t.Fatal("opciones inválidas deben rechazarse antes de enviar")
	}
}

// hop construye un salto con pérdida y latencia dadas (avg<0 = sin RTT).
func hop(ttl int, loss, avg float64) Hop {
	s := Stats{Sent: 3, LossPct: loss}
	s.Received = 3
	if loss >= 100 {
		s.Received = 0
	}
	if avg >= 0 && s.Received > 0 {
		s.RTTOK, s.AvgMs = true, avg
	}
	return Hop{TTL: ttl, Stats: s}
}

func TestAnalyze(t *testing.T) {
	const (
		icmp = "limita respuestas ICMP"
		loss = "pérdida desde el salto"
		lat  = "la latencia sube en el salto"
		dst  = "el destino no responde o filtra ICMP a partir del salto"
	)
	tests := []struct {
		name    string
		hops    []Hop
		reached bool
		want    []string // subcadenas esperadas
		not     []string
	}{
		{"limite ICMP no persistente", []Hop{hop(1, 0, 1), hop(2, 33, 2), hop(3, 0, 3)}, true, []string{"salto 2: el equipo limita respuestas ICMP (no es pérdida real)"}, []string{loss}},
		{"limite ICMP con salto 100 % mudo", []Hop{hop(1, 0, 1), hop(2, 100, -1), hop(3, 0, 3)}, true, []string{"salto 2: el equipo limita respuestas ICMP"}, []string{loss, dst}},
		{"salto mudo que persiste no es limite ICMP", []Hop{hop(1, 0, 1), hop(2, 100, -1), hop(3, 100, -1)}, true, []string{"pérdida desde el salto 2"}, []string{icmp}},
		{"perdida persistente", []Hop{hop(1, 0, 1), hop(2, 33, 2), hop(3, 33, 3)}, true, []string{"pérdida desde el salto 2"}, []string{icmp}},
		{"borde 20 % persiste", []Hop{hop(1, 0, 1), hop(2, 20, 2), hop(3, 20, 3)}, true, []string{"pérdida desde el salto 2"}, nil},
		{"borde 19.9 % no", []Hop{hop(1, 0, 1), hop(2, 19.9, 2), hop(3, 19.9, 3)}, true, nil, []string{loss}},
		{"perdida solo en destino", []Hop{hop(1, 0, 1), hop(2, 0, 2), hop(3, 33, 3)}, true, []string{"pérdida desde el salto 3"}, nil},
		{"latencia +20 ms exacto persiste", []Hop{hop(1, 0, 5), hop(2, 0, 25), hop(3, 0, 30)}, true, []string{lat + " 2"}, nil},
		{"latencia +19.9 ms no", []Hop{hop(1, 0, 5), hop(2, 0, 24.9), hop(3, 0, 30)}, true, nil, []string{lat}},
		{"latencia pico no persiste", []Hop{hop(1, 0, 5), hop(2, 0, 50), hop(3, 0, 6)}, true, nil, []string{lat}},
		{"destino no responde", []Hop{hop(1, 0, 1), hop(2, 100, -1), hop(3, 100, -1)}, false, []string{dst + " 2"}, []string{loss}},
		{"sin llegar pero ultimo responde", []Hop{hop(1, 0, 1), hop(2, 0, 2)}, false, nil, []string{dst}},
		{"sano", []Hop{hop(1, 0, 1), hop(2, 0, 2), hop(3, 0, 3)}, true, nil, []string{icmp, loss, lat, dst}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(Analyze(TraceResult{Hops: tc.hops, Reached: tc.reached}), "\n")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("falta %q en:\n%s", w, got)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("sobra %q en:\n%s", n, got)
				}
			}
		})
	}
}

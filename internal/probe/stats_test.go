package probe

import (
	"context"
	"math"
	"testing"
	"time"
)

func ok(ms float64) Reply { return Reply{Status: StatusOK, RTTms: ms} }

func TestNewStats(t *testing.T) {
	to := Reply{Status: StatusTimeout, Reason: "tiempo de espera agotado"}
	tests := []struct {
		name               string
		in                 []Reply
		sent, recv         int
		loss               float64
		rttOK              bool
		min, avg, max      float64
		jitterOK           bool
		jitter             float64
		lastErr, fmtJitter string
	}{
		{"todo responde", []Reply{ok(10), ok(20), ok(10), ok(20)}, 4, 4, 0, true, 10, 15, 20, true, 10, "", "10.0 ms"},
		{"perdida parcial", []Reply{ok(5), to, ok(7), to}, 4, 2, 50, true, 5, 6, 7, true, 2, "tiempo de espera agotado", "2.0 ms"},
		{"100% perdida", []Reply{to, to}, 2, 0, 100, false, 0, 0, 0, false, 0, "tiempo de espera agotado", "no disponible"},
		{"una respuesta", []Reply{to, ok(3)}, 2, 1, 50, true, 3, 3, 3, false, 0, "tiempo de espera agotado", "no disponible"},
		{"rtt cero", []Reply{ok(0), ok(0)}, 2, 2, 0, true, 0, 0, 0, true, 0, "", "< 1 ms"},
		{"error ICMP es perdida", []Reply{{Status: StatusHostUnreachable, Reason: "equipo de destino inalcanzable"}, ok(1)}, 2, 1, 50, true, 1, 1, 1, false, 0, "equipo de destino inalcanzable", "no disponible"},
		{"vacio", nil, 0, 0, 0, false, 0, 0, 0, false, 0, "", "no disponible"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStats(tc.in)
			near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
			if s.Sent != tc.sent || s.Received != tc.recv || !near(s.LossPct, tc.loss) ||
				s.RTTOK != tc.rttOK || !near(s.MinMs, tc.min) || !near(s.AvgMs, tc.avg) || !near(s.MaxMs, tc.max) ||
				s.JitterOK != tc.jitterOK || !near(s.JitterMs, tc.jitter) || s.LastError != tc.lastErr ||
				s.FormatJitter() != tc.fmtJitter {
				t.Fatalf("got %+v jitter=%q", s, s.FormatJitter())
			}
		})
	}
}

func TestFormatMs(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{{0, "< 1 ms"}, {0.5, "< 1 ms"}, {1, "1.0 ms"}, {12.34, "12.3 ms"}} {
		if got := FormatMs(tc.in); got != tc.want {
			t.Errorf("FormatMs(%v)=%q, quiero %q", tc.in, got, tc.want)
		}
	}
}

func TestStatusFromCode(t *testing.T) {
	tests := []struct {
		code   uint32
		st     Status
		reason string
	}{
		{0, StatusOK, ""},
		{11002, StatusNetUnreachable, "red de destino inalcanzable"},
		{11003, StatusHostUnreachable, "equipo de destino inalcanzable"},
		{11005, StatusPortUnreachable, "puerto de destino inalcanzable"},
		{11009, StatusFragNeeded, "paquete demasiado grande (requiere fragmentar)"},
		{11010, StatusTimeout, "tiempo de espera agotado"},
		{11013, StatusTTLExpired, "TTL agotado en tránsito"},
		{99999, StatusOther, "no disponible"},
		{11004, StatusOther, "no disponible"},
	}
	for _, tc := range tests {
		st, r := StatusFromCode(tc.code)
		if st != tc.st || r != tc.reason {
			t.Errorf("code %d: got (%v,%q)", tc.code, st, r)
		}
	}
	if StatusTTLExpired.String() != "ttl_expired" || Status(99).String() != "other" {
		t.Error("String de Status")
	}
}

func TestClocks(t *testing.T) {
	f := NewFakeClock(time.Unix(100, 0))
	ctx := context.Background()
	if err := f.Sleep(ctx, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !f.Now().Equal(time.Unix(100, 0).Add(500*time.Millisecond)) || len(f.Sleeps()) != 1 {
		t.Fatal("FakeClock no avanza")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if f.Sleep(cctx, time.Second) == nil || (RealClock{}).Sleep(cctx, time.Second) == nil {
		t.Fatal("Sleep debe respetar la cancelación")
	}
	if err := (RealClock{}).Sleep(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

package probe

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type scriptEcho struct {
	mu      sync.Mutex
	replies []Reply
	calls   int
	reqs    []EchoRequest
}

func (s *scriptEcho) Echo(_ context.Context, req EchoRequest) (Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	r := s.replies[s.calls%len(s.replies)]
	s.calls++
	return r, nil
}

type blockEcho struct {
	mu    sync.Mutex
	calls int
	rel   chan struct{}
}

func (b *blockEcho) Echo(ctx context.Context, _ EchoRequest) (Reply, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case <-b.rel:
	case <-ctx.Done():
	}
	return Reply{Status: StatusTimeout}, nil
}

var pingDst = netip.MustParseAddr("10.0.0.1")

func pingOpts(n int) PingOptions {
	return PingOptions{Count: n, Interval: 500 * time.Millisecond, Timeout: time.Second}
}

func TestPingCases(t *testing.T) {
	ok := func(ms float64) Reply { return Reply{Status: StatusOK, RTTms: ms} }
	cases := []struct {
		name     string
		replies  []Reply
		n        int
		recv     int
		jitterOK bool
	}{
		{"todo responde", []Reply{ok(1), ok(2), ok(3)}, 3, 3, true},
		{"perdida parcial", []Reply{ok(1), {Status: StatusTimeout, Reason: "tiempo de espera agotado"}, ok(3)}, 3, 2, true},
		{"perdida total", []Reply{{Status: StatusHostUnreachable, Reason: "equipo de destino inalcanzable"}}, 3, 0, false},
		{"una respuesta", []Reply{ok(5), {Status: StatusTimeout}}, 2, 1, false},
		{"ttl agotado", []Reply{{Status: StatusTTLExpired, Reason: "TTL agotado en tránsito"}, ok(0)}, 2, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &scriptEcho{replies: c.replies}
			env := Env{Echoer: e, Clock: NewFakeClock(time.Unix(0, 0))}
			st, rs, err := Ping(context.Background(), env, pingDst, pingOpts(c.n))
			if err != nil || len(rs) != c.n || st.Sent != c.n || st.Received != c.recv || st.JitterOK != c.jitterOK {
				t.Fatalf("err=%v n=%d stats=%+v", err, len(rs), st)
			}
			if c.recv < c.n && st.LastError == "" && c.name != "una respuesta" {
				t.Errorf("sin motivo de pérdida: %+v", st)
			}
			for _, r := range e.reqs {
				if r.Size != 32 || r.Dst != pingDst || r.Timeout != time.Second {
					t.Errorf("petición %+v", r)
				}
			}
		})
	}
}

func TestPingInterval(t *testing.T) {
	fc := NewFakeClock(time.Unix(0, 0))
	e := &scriptEcho{replies: []Reply{{Status: StatusOK, RTTms: 1}}}
	if _, _, err := Ping(context.Background(), Env{Echoer: e, Clock: fc}, pingDst, pingOpts(4)); err != nil {
		t.Fatal(err)
	}
	sl := fc.Sleeps()
	if len(sl) != 3 {
		t.Fatalf("esperas: %v", sl)
	}
	for _, d := range sl {
		if d != 500*time.Millisecond {
			t.Errorf("espera %v", d)
		}
	}
}

func TestPingInvalidOptions(t *testing.T) {
	e := &scriptEcho{replies: []Reply{{Status: StatusOK}}}
	_, _, err := Ping(context.Background(), Env{Echoer: e, Clock: NewFakeClock(time.Unix(0, 0))}, pingDst, PingOptions{Count: 0})
	if err == nil || e.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, e.calls)
	}
}

func TestPingCancelReturnsFast(t *testing.T) {
	b := &blockEcho{rel: make(chan struct{})}
	defer close(b.rel)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := Ping(ctx, Env{Echoer: b, Clock: NewFakeClock(time.Unix(0, 0))}, pingDst, pingOpts(10))
		done <- err
	}()
	for {
		b.mu.Lock()
		c := b.calls
		b.mu.Unlock()
		if c > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("no volvió en 1 s")
	}
	time.Sleep(20 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.calls != 1 {
		t.Errorf("ecos tras cancelar: %d", b.calls)
	}
}

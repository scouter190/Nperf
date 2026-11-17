package probe

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var loop = netip.MustParseAddr("127.0.0.1")

func listen(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, l.Addr().(*net.TCPAddr).Port
}

func closedPort(t *testing.T) int {
	l, p := listen(t)
	l.Close()
	return p
}

func TestPortsOpenClosedNoData(t *testing.T) {
	l, open := listen(t)
	got := make(chan int, 4)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.SetReadDeadline(time.Now().Add(time.Second))
			buf := make([]byte, 16)
			n, _ := c.Read(buf)
			got <- n
			c.Close()
		}
	}()
	closed := closedPort(t)
	res, err := Ports(context.Background(), &net.Dialer{}, loop, []int{closed, open}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Port != closed || res[0].State != PortClosed {
		t.Errorf("cerrado: %+v", res[0])
	}
	if res[1].Port != open || res[1].State != PortOpen {
		t.Errorf("abierto: %+v", res[1])
	}
	if n := <-got; n != 0 {
		t.Errorf("el servidor recibió %d bytes", n)
	}
}

type fakeDialer struct {
	cur, max atomic.Int32
	hold     time.Duration
	mu       sync.Mutex
	calls    int
}

func (f *fakeDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	n := f.cur.Add(1)
	defer f.cur.Add(-1)
	for {
		m := f.max.Load()
		if n <= m || f.max.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-time.After(f.hold):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return nil, ctx.Err()
}

func TestPortsFilteredAndConcurrency(t *testing.T) {
	f := &fakeDialer{hold: time.Hour}
	ports := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	res, err := Ports(context.Background(), f, loop, ports, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range res {
		if r.State != PortFiltered || r.Port != ports[i] {
			t.Errorf("%d: %+v", i, r)
		}
	}
	if m := f.max.Load(); m > MaxConcurrentPorts || m < 2 {
		t.Errorf("concurrencia máxima %d", m)
	}
}

func TestPortsCancel(t *testing.T) {
	f := &fakeDialer{hold: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	start := time.Now()
	res, err := Ports(ctx, f, loop, []int{1, 2, 3, 4, 5, 6, 7, 8}, 5*time.Second)
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancelación lenta o sin error: %v", err)
	}
	for _, r := range res {
		if r.State != PortCanceled {
			t.Errorf("%+v", r)
		}
	}
	if f.calls > MaxConcurrentPorts {
		t.Errorf("%d llamadas tras cancelar", f.calls)
	}
}

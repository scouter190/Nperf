package discovery

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

var testFrom = netip.MustParseAddrPort("10.0.0.2:5301")

func newTestListener(t *testing.T, count *int) *Listener {
	self := testAnnounce(t)
	return &Listener{
		Self:       func() (Announce, bool) { return self, true },
		OnAnnounce: func(Announce, netip.AddrPort) { *count++ },
	}
}

func TestListenerIgnoresNonQuery(t *testing.T) {
	n := 0
	l := newTestListener(t, &n)
	for _, m := range []string{"", "NPDISC1", "NPDISC1 ?x", "NPDISC1 ? ", QueryMessage, string(QueryPacket()[:127]), string(QueryPacket()) + "x", "npdisc1 ?", "hola", "\x00\xff"} {
		if r := l.handle([]byte(m), testFrom, time.Now()); r != nil {
			t.Errorf("%q: respondió %q", m, r)
		}
	}
	if n != 0 {
		t.Fatal("callback inesperado")
	}
}

func TestListenerBadPadding(t *testing.T) {
	n := 0
	l := newTestListener(t, &n)
	q := QueryPacket()
	q[100] = 'x'
	if l.handle(q, testFrom, time.Now()) != nil {
		t.Fatal("relleno incorrecto debía ignorarse")
	}
	if l.handle(QueryPacket(), testFrom, time.Now()) == nil {
		t.Fatal("consulta válida sin respuesta")
	}
}

func TestListenerRateLimit(t *testing.T) {
	n := 0
	l := newTestListener(t, &n)
	now := time.Now()
	if l.handle(QueryPacket(), testFrom, now) == nil {
		t.Fatal("primera consulta sin respuesta")
	}
	if l.handle(QueryPacket(), testFrom, now.Add(100*time.Millisecond)) != nil {
		t.Fatal("debía limitar")
	}
	if l.handle(QueryPacket(), testFrom, now.Add(2*time.Second)) == nil {
		t.Fatal("debía responder tras el intervalo")
	}
}

func TestListenerDeliversAnnounce(t *testing.T) {
	n := 0
	l := newTestListener(t, &n)
	msg, _ := testAnnounce(t).Marshal()
	if l.handle(msg, testFrom, time.Now()) != nil || n != 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestListenerServeStopsOnCancel(t *testing.T) {
	c := listen(t)
	l := &Listener{Conn: c}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve no terminó")
	}
	_ = net.ErrClosed
}

func FuzzListenerHandle(f *testing.F) {
	f.Add(QueryPacket())
	f.Add([]byte("NPDISC1 id=0123456789abcdef ip=1.2.3.4 port=1 state=busy queue=0 alias=x"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		n := 0
		l := newTestListener(t, &n)
		l.handle(data, testFrom, time.Now())
	})
}

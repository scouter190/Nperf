package meter

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type tcpFakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tcpFakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *tcpFakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// chanReader avisa al entrar en Read: la 2.ª entrada garantiza que el contador ya sumó.
type chanReader struct {
	entered chan struct{}
	data    chan int
}

func (r *chanReader) Read(p []byte) (int, error) {
	r.entered <- struct{}{}
	n, ok := <-r.data
	if !ok {
		return 0, io.EOF
	}
	return n, nil
}

func TestReceiveTCPNSamplesSameResult(t *testing.T) {
	for _, n := range []int{1, 4} {
		mk := func() []io.Reader {
			rs := make([]io.Reader, n)
			for i := range rs {
				rs[i] = bytes.NewReader(make([]byte, 300_000+i))
			}
			return rs
		}
		a, errA := ReceiveTCPN(mk(), nil)
		ch := make(chan Sample, 8)
		b, errB := ReceiveTCPNSamples(mk(), nil, 3*time.Second, ch)
		if errA != nil || errB != nil || a.Bytes != b.Bytes || a.Retrans != b.Retrans ||
			a.RetransOK != b.RetransOK || a.RetransEstimated != b.RetransEstimated {
			t.Fatalf("n=%d: %+v %v vs %+v %v", n, a, errA, b, errB)
		}
		var sum int64
		for len(ch) > 0 {
			sum += (<-ch).Bytes
		}
		if sum != b.Bytes {
			t.Fatalf("n=%d: muestras %d != %d", n, sum, b.Bytes)
		}
	}
}

func TestReceiveTCPNFakeClockSamples(t *testing.T) {
	clk := &tcpFakeClock{t: time.Unix(1000, 0)}
	tick := make(chan time.Time)
	out := make(chan Sample, 8)
	rds := []*chanReader{
		{entered: make(chan struct{}), data: make(chan int)},
		{entered: make(chan struct{}), data: make(chan int)},
	}
	smp := &tcpSampler{series: NewSeries(3*time.Second, out), now: clk.Now, tick: tick}
	type ret struct {
		res Result
		err error
	}
	rc := make(chan ret, 1)
	go func() {
		res, err := receiveTCPN([]io.Reader{rds[0], rds[1]}, nil, smp)
		rc <- ret{res, err}
	}()
	for _, r := range rds {
		<-r.entered
	}
	feed := func(i, n int) { rds[i].data <- n; <-rds[i].entered }
	step := func() { tick <- clk.Now(); tick <- clk.Now() } // el 2.º garantiza el 1.º procesado
	feed(0, 100)
	feed(1, 50)
	step()
	clk.Advance(time.Second)
	feed(0, 200)
	step()
	clk.Advance(time.Second)
	feed(1, 300)
	step()
	clk.Advance(500 * time.Millisecond)
	close(rds[0].data)
	close(rds[1].data)
	r := <-rc
	if r.err != nil || r.res.Bytes != 650 {
		t.Fatalf("%+v %v", r.res, r.err)
	}
	want := []int64{150, 200, 300}
	if len(out) != 3 {
		t.Fatalf("muestras %d", len(out))
	}
	for i, w := range want {
		s := <-out
		if s.Bytes != w || s.Index != i {
			t.Fatalf("muestra %d: %+v, want %d", i, s, w)
		}
	}
}

func TestReceiveTCPNSamplesLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	go func() {
		SendTCP(t.Context(), cc, 1500*time.Millisecond)
		cc.Close()
	}()
	out := make(chan Sample, 8)
	res, err := ReceiveTCPNSamples([]io.Reader{sc}, nil, 1500*time.Millisecond, out)
	if err != nil || res.Bytes == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	var sum int64
	n := len(out)
	for len(out) > 0 {
		sum += (<-out).Bytes
	}
	if n != 2 || sum != res.Bytes {
		t.Fatalf("muestras %d suma %d, Bytes %d", n, sum, res.Bytes)
	}
}

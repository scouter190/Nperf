package meter

import (
	"context"
	"math"
	"net"
	"testing"
	"time"
)

// runUDP envía a 50 Mbps durante d y devuelve estadísticas de ambos lados.
func runUDP(t *testing.T, d time.Duration, loss float64) (UDPSendStats, UDPRecvStats) {
	t.Helper()
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	pc.SetReadBuffer(8 << 20)
	c, err := net.DialUDP("udp", nil, pc.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type out struct {
		st  UDPRecvStats
		err error
	}
	ch := make(chan out, 1)
	go func() {
		st, err := ReceiveUDP(ctx, pc, 500*time.Millisecond)
		ch <- out{st, err}
	}()

	var w = NewLossyWriter(c, loss)
	ss, err := SendUDP(ctx, w, 50e6, DefaultUDPSize, d)
	if err != nil {
		t.Fatal(err)
	}
	o := <-ch
	if o.err != nil {
		t.Fatal(o.err)
	}
	return ss, o.st
}

func TestUDPRate50Mbps(t *testing.T) {
	ss, rs := runUDP(t, 2*time.Second, 0)
	// Se mide la tasa enviada por el emisor: no depende de la carga del receptor.
	if got := ss.Mbps(); math.Abs(got-50) > 2.5 {
		t.Fatalf("tasa enviada %.2f Mbps, quería 50 ±5%%; perdonados %d B, iteraciones %d, mayor exceso de espera %v, transcurrido %v",
			got, ss.Forgiven, ss.Iterations, ss.MaxOvershoot, ss.Elapsed)
	}
	if rs.Received == 0 || rs.Expected == 0 {
		t.Fatalf("receptor sin datos: %+v", rs)
	}
	t.Logf("perdonados %d B, iteraciones %d, mayor exceso %v", ss.Forgiven, ss.Iterations, ss.MaxOvershoot)
	t.Logf("enviado %.2f Mbps, recibido %d/%d, pérdida %.2f%%, jitter %v",
		ss.Mbps(), rs.Received, rs.Expected, rs.LossPercent(), rs.Jitter)
}

func TestUDPLoss10Percent(t *testing.T) {
	_, rs := runUDP(t, time.Second, 0.10)
	if got := rs.LossPercent(); math.Abs(got-10) > 1 {
		t.Fatalf("pérdida %.2f%%, quería 10 ±1 (%+v)", got, rs)
	}
	if rs.Jitter < 0 {
		t.Fatalf("jitter negativo: %v", rs.Jitter)
	}
}

// --- pruebas con reloj y lector falsos ---

type fakeClock struct{ t time.Time }

func (f *fakeClock) clock(onSleep func(d time.Duration), lag time.Duration) clock {
	return clock{
		now: func() time.Time { return f.t },
		sleep: func(_ context.Context, d time.Duration) {
			if onSleep != nil {
				onSleep(d)
			}
			f.t = f.t.Add(d + lag)
		},
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

type pkt struct {
	seq   uint64
	fin   bool
	short bool
	dt    time.Duration // avance del reloj antes de la llegada
	skew  time.Duration // hora de envío = llegada - skew
}

type fakeReader struct {
	f    *fakeClock
	pkts []pkt
}

func (r *fakeReader) SetReadDeadline(time.Time) error { return nil }

func (r *fakeReader) Read(b []byte) (int, error) {
	if len(r.pkts) == 0 {
		return 0, timeoutErr{}
	}
	p := r.pkts[0]
	r.pkts = r.pkts[1:]
	r.f.t = r.f.t.Add(p.dt)
	if p.short {
		return 5, nil
	}
	var fl byte
	if p.fin {
		fl = flagFin
	}
	putHeader(b, p.seq, fl, r.f.t.Add(-p.skew))
	return 100, nil
}

func recv(t *testing.T, pkts []pkt) UDPRecvStats {
	t.Helper()
	f := &fakeClock{t: time.Unix(1000, 0)}
	st, err := receiveUDP(context.Background(), &fakeReader{f: f, pkts: pkts}, time.Second, f.clock(nil, 0))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func seqs(dt time.Duration, skew time.Duration, s ...uint64) []pkt {
	var out []pkt
	for _, x := range s {
		out = append(out, pkt{seq: x, dt: dt, skew: skew})
	}
	return out
}

func TestUDPReorderNoLoss(t *testing.T) {
	st := recv(t, seqs(time.Millisecond, 0, 0, 2, 1, 4, 3))
	if st.Lost != 0 || st.Received != 5 || st.Expected != 5 {
		t.Fatalf("%+v", st)
	}
}

func TestUDPDuplicatesDoNotHideLoss(t *testing.T) {
	st := recv(t, seqs(time.Millisecond, 0, 0, 1, 1, 1, 4))
	if st.Received != 3 || st.Duplicates != 2 || st.Lost != 2 || st.Bytes != 300 {
		t.Fatalf("%+v", st)
	}
}

func TestUDPLostFinEndsByIdle(t *testing.T) {
	st := recv(t, seqs(time.Millisecond, 0, 0, 1, 2)) // sin FIN: Read devuelve timeout
	if st.Received != 3 || st.Lost != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestUDPFinCountsTrailingLoss(t *testing.T) {
	p := seqs(time.Millisecond, 0, 0, 1)
	p = append(p, pkt{seq: 4, fin: true})
	st := recv(t, p)
	if st.Expected != 4 || st.Lost != 2 {
		t.Fatalf("%+v", st)
	}
}

func TestUDPShortDatagramIgnored(t *testing.T) {
	p := []pkt{{seq: 0, dt: time.Millisecond}, {short: true}, {seq: 1, dt: time.Millisecond}}
	st := recv(t, p)
	if st.Received != 2 || st.Lost != 0 || st.Bytes != 200 {
		t.Fatalf("%+v", st)
	}
}

func TestUDPJitterConstantOffsetAndRateIsZero(t *testing.T) {
	// El reloj del emisor va 5 s atrasado, constante; ritmo constante: jitter 0.
	st := recv(t, seqs(time.Millisecond, 5*time.Second, 0, 1, 2, 3, 4, 5, 6, 7))
	if st.Jitter != 0 {
		t.Fatalf("jitter %v, quería 0", st.Jitter)
	}
}

func TestUDPJitterVariableIsPositive(t *testing.T) {
	var p []pkt
	for i := uint64(0); i < 20; i++ {
		var skew time.Duration
		if i%2 == 1 {
			skew = 2 * time.Millisecond // tránsito variable
		}
		p = append(p, pkt{seq: i, dt: time.Millisecond, skew: skew})
	}
	st := recv(t, p)
	if st.Jitter <= 0 {
		t.Fatalf("jitter %v, quería > 0", st.Jitter)
	}
}

type countWriter struct{ n, run, maxRun int }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n++
	c.run++
	if c.run > c.maxRun {
		c.maxRun = c.run
	}
	return len(p), nil
}

func TestUDPCatchUpIsBounded(t *testing.T) {
	// Cada sleep se atrasa 100 ms (> debtWindow): la deuda que excede 20 ms se perdona,
	// así que el total queda bajo el objetivo y la ráfaga entre sleeps no supera la ventana.
	f := &fakeClock{t: time.Unix(1000, 0)}
	w := &countWriter{}
	clk := f.clock(func(time.Duration) { w.run = 0 }, 100*time.Millisecond)
	const rate, size = 10e6, 1000
	d := 2 * time.Second
	st, err := sendUDP(context.Background(), w, rate, size, d, clk)
	if err != nil {
		t.Fatal(err)
	}
	window := int(debtWindow.Seconds() * rate / 8 / size)
	if w.maxRun > window+burstPackets+finCopies {
		t.Fatalf("ráfaga %d > %d", w.maxRun, window)
	}
	max := int64(rate / 8 * d.Seconds())
	if st.Bytes > max*3/4 {
		t.Fatalf("enviado %d > 75%% del objetivo %d: la deuda > 20 ms debía perdonarse", st.Bytes, max)
	}
}

func TestUDPRateWithCoarseTimer(t *testing.T) {
	// Sleeps que se atrasan 16 ms (resolución típica de Windows): la tasa media se mantiene.
	f := &fakeClock{t: time.Unix(1000, 0)}
	w := &countWriter{}
	const rate, size = 50e6, 1200
	st, err := sendUDP(context.Background(), w, rate, size, 3*time.Second, f.clock(nil, 16*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mbps(); math.Abs(got-50) > 1 {
		t.Fatalf("%.2f Mbps, quería 50 ±2%%", got)
	}
}

func TestUDPSendSteadyRateFakeClock(t *testing.T) {
	f := &fakeClock{t: time.Unix(1000, 0)}
	w := &countWriter{}
	st, err := sendUDP(context.Background(), w, 8e6, 1000, time.Second, f.clock(nil, 0))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(st.Mbps()-8) > 0.1 {
		t.Fatalf("%.2f Mbps", st.Mbps())
	}
}

func TestUDPSendStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SendUDP(ctx, &countWriter{}, 1e6, 1000, time.Hour)
	if err == nil {
		t.Fatal("quería error de contexto")
	}
}

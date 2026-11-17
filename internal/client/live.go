package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

// Interval es la muestra de 1 s de una fase, con el RTT de la sonda del mismo índice.
type Interval struct {
	Index    int
	Start    time.Duration // inicio respecto al primer byte o datagrama
	Duration time.Duration
	Bytes    int64
	Mbps     float64
	RTT      time.Duration // RTT bajo carga; válido solo si HasRTT
	HasRTT   bool

	// Solo UDP.
	UDP      bool
	Received int64
	Expected int64
	Lost     int64
	LossPct  float64
	Jitter   time.Duration
}

func fromMeter(s meter.Sample) Interval {
	return Interval{Index: s.Index, Start: s.Start, Duration: s.Duration, Bytes: s.Bytes,
		Mbps: s.Mbps(), UDP: s.UDP, Received: s.Received, Expected: s.Expected,
		Lost: s.Lost, LossPct: s.LossPercent(), Jitter: s.Jitter}
}

func fromProto(s proto.Sample, udp bool) Interval {
	return Interval{Index: s.Index, Start: time.Duration(s.TMS) * time.Millisecond,
		Duration: time.Duration(s.DurationMS) * time.Millisecond, Bytes: s.Bytes, Mbps: s.Mbps,
		UDP: udp, Received: s.Received, Expected: s.Expected, Lost: s.Lost, LossPct: s.LossPct,
		Jitter: time.Duration(s.JitterMS * float64(time.Millisecond))}
}

type probeOut struct {
	rtt time.Duration
	ok  bool
}

type ctlResult struct {
	msg proto.Message
	err error
}

// liveRun coordina una fase en vivo: lectora del control (sample/pong/result/error), prober y
// entrega ordenada de muestras. Mientras vive, solo la lectora toca la lectura del códec.
type liveRun struct {
	ss         *sess
	phase      string
	udp        bool
	durMS      int64
	initAvg    time.Duration
	onInterval func(phase string, iv Interval)

	prober *Prober // nil con NoLoadRTT
	cancel context.CancelFunc
	wake   chan struct{}
	pdone  chan struct{} // se cierra al terminar el prober
	res    chan ctlResult

	deliverMu sync.Mutex // serializa los callbacks
	mu        sync.Mutex
	pending   []Interval
	probes    map[int]probeOut
	delivered []Interval
	answered  []time.Duration
	sent      int
	noted     bool
}

// newLive arranca la lectora y, salvo NoLoadRTT, el prober. Las goroutines cuelgan de ss.wg.
func newLive(ctx context.Context, ss *sess, o *Options, phase string, udp bool) *liveRun {
	lv := &liveRun{ss: ss, phase: phase, udp: udp, durMS: o.Duration.Milliseconds(),
		onInterval: o.OnInterval, res: make(chan ctlResult, 1), probes: map[int]probeOut{},
		wake: make(chan struct{}, 1), cancel: func() {}}
	if ss.rtt != nil {
		lv.initAvg = ss.rtt.Avg
	}
	lv.pdone = make(chan struct{})
	if o.NoLoadRTT {
		close(lv.pdone)
	} else {
		count := int((o.Duration + time.Second - 1) / time.Second)
		if count > proto.MaxSamples {
			count = proto.MaxSamples
		}
		lv.prober = NewProber(time.Now(), count, func(seq int) error {
			lv.mu.Lock()
			lv.sent++
			lv.mu.Unlock()
			return ss.codec.Write(proto.Message{Type: proto.TypePing, Seq: int64(seq)})
		})
		var pctx context.Context
		pctx, lv.cancel = context.WithCancel(ctx)
		ss.wg.Add(1)
		go func() {
			defer ss.wg.Done()
			defer close(lv.pdone)
			lv.prober.Run(pctx, nil, lv.wake, func(r ProbeResult) { lv.addProbe(r.Seq, 0, false) })
		}()
	}
	ss.wg.Add(1)
	go func() { defer ss.wg.Done(); lv.readLoop() }()
	return lv
}

func (lv *liveRun) readLoop() {
	last := -1
	fail := func(err error) { lv.res <- ctlResult{err: err} }
	for {
		m, err := lv.ss.codec.Read()
		if err != nil {
			fail(fmt.Errorf("canal de control cerrado: %w", err))
			return
		}
		switch m.Type {
		case proto.TypeSample:
			s, err := proto.SampleFromMessage(m, last, lv.durMS)
			if err != nil {
				fail(err)
				return
			}
			last = s.Index
			lv.addSample(fromProto(s, lv.udp))
		case proto.TypePong:
			if lv.prober != nil {
				if r, ok := lv.prober.Pong(int(m.Seq), time.Now()); ok {
					lv.addProbe(r.Seq, r.RTT, true)
					select {
					case lv.wake <- struct{}{}:
					default:
					}
				}
			}
		case proto.TypeResult:
			lv.res <- ctlResult{msg: m}
			return
		case proto.TypeError:
			fail(fmt.Errorf("el servidor respondió: %s", m.Error))
			return
		default:
			fail(fmt.Errorf("mensaje inesperado %q durante la fase", m.Type))
			return
		}
	}
}

func (lv *liveRun) addSample(iv Interval) {
	lv.mu.Lock()
	lv.pending = append(lv.pending, iv)
	lv.mu.Unlock()
	lv.deliver(false)
}

func (lv *liveRun) addProbe(seq int, rtt time.Duration, ok bool) {
	lv.mu.Lock()
	lv.probes[seq] = probeOut{rtt, ok}
	if ok {
		lv.answered = append(lv.answered, rtt)
	}
	lv.mu.Unlock()
	lv.deliver(false)
}

// deliver entrega en orden las muestras cuya sonda ya se resolvió (todas si final o sin sondas).
func (lv *liveRun) deliver(final bool) {
	lv.deliverMu.Lock()
	defer lv.deliverMu.Unlock()
	lv.mu.Lock()
	var ready []Interval
	for len(lv.pending) > 0 {
		iv := lv.pending[0]
		p, ok := lv.probes[iv.Index]
		if lv.prober != nil && !ok && !final {
			break
		}
		if ok && p.ok {
			iv.RTT, iv.HasRTT = p.rtt, true
		}
		ready = append(ready, iv)
		lv.pending = lv.pending[1:]
	}
	lv.delivered = append(lv.delivered, ready...)
	lv.mu.Unlock()
	if lv.onInterval != nil {
		for _, iv := range ready {
			lv.onInterval(lv.phase, iv)
		}
	}
}

// pump consume las muestras generadas por el propio cliente (rev). El llamador cierra out al
// terminar el medidor y espera a la función devuelta.
func (lv *liveRun) pump(out <-chan meter.Sample) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for s := range out {
			lv.addSample(fromMeter(s))
		}
	}()
	return func() { <-done }
}

// result espera el result (o error) del servidor.
func (lv *liveRun) result() (proto.Message, error) {
	r := <-lv.res
	return r.msg, r.err
}

// stop detiene el prober sin esperar (rutas de error; el cierre de la sesión espera a todo).
func (lv *liveRun) stop() {
	if lv != nil {
		lv.cancel()
	}
}

// attach cierra la fase: detiene las sondas, entrega lo pendiente y rellena d. Con lv nil
// devuelve d tal cual (servidor sin live). Si la sonda de la última muestra sigue en vuelo
// (hace menos de 1 s), esa muestra no se entrega todavía: queda en ss.late y se completa con
// su pong, que llega entre fases (expectStart) o al cerrar la sesión (finishLate); si no llega
// a tiempo se entrega sin RTT.
func (lv *liveRun) attach(d Dir) Dir {
	if lv == nil {
		return d
	}
	lv.cancel()
	<-lv.pdone
	var (
		lseq   int
		sentAt time.Time
		late   bool
	)
	if lv.prober != nil {
		if seq, at, ok := lv.prober.Pending(); ok && time.Since(at) < probeTimeout {
			lseq, sentAt, late = seq, at, true
		}
	}
	lv.deliver(false)
	var held *Interval
	lv.mu.Lock()
	if late && len(lv.pending) == 1 && lv.pending[0].Index == lseq {
		iv := lv.pending[0]
		held = &iv
		lv.pending = nil
	}
	lv.mu.Unlock()
	lv.deliver(true)
	lv.mu.Lock()
	d.Live = true
	d.Intervals = append([]Interval(nil), lv.delivered...)
	if held != nil {
		d.Intervals = append(d.Intervals, *held)
	}
	answered, sent := append([]time.Duration(nil), lv.answered...), lv.sent
	if lv.prober != nil {
		d.LoadedRTT = NewLoadedRTT(answered, sent)
	}
	lv.mu.Unlock()
	lv.noted = lv.noteFor(d.LoadedRTT)
	if held != nil {
		lv.ss.late = &lateProbe{lv: lv, seq: lseq, sentAt: sentAt, iv: *held, answered: answered, sent: sent}
	}
	return d
}

// noteFor añade la nota de bufferbloat si aplica y devuelve si la añadió.
func (lv *liveRun) noteFor(l *LoadedRTT) bool {
	if note := BufferbloatNote(lv.initAvg, l, lv.label()); note != "" {
		lv.ss.addNote(note)
		return true
	}
	return false
}

// lateProbe es la sonda de la última muestra de una fase, aún sin resolver.
type lateProbe struct {
	lv       *liveRun
	seq      int
	sentAt   time.Time
	iv       Interval
	answered []time.Duration
	sent     int
	dst      *Dir // resultado de la fase (se enlaza con bind)
}

// bind enlaza la sonda pendiente con el Dir ya guardado en el Result.
func (s *sess) bind(dst *Dir) {
	if s.late != nil && s.late.dst == nil {
		s.late.dst = dst
	}
}

// latePong trata un pong suelto: lo asigna a la última muestra de la fase anterior si es el de
// su sonda y llegó dentro de 1 s del envío; cualquier otro se descarta.
func (s *sess) latePong(seq int64) {
	l := s.late
	if l == nil || int64(l.seq) != seq {
		return
	}
	if rtt := time.Since(l.sentAt); rtt < probeTimeout {
		l.finish(rtt, true)
	} else {
		l.finish(0, false)
	}
	s.late = nil
}

// giveUpLate entrega la muestra pendiente sin RTT.
func (s *sess) giveUpLate() {
	if l := s.late; l != nil {
		s.late = nil
		l.finish(0, false)
	}
}

// finishLate, al terminar la prueba, lee el control como mucho hasta que venza el plazo de la
// sonda pendiente (solo si queda alguna). Tras un error de lectura la sesión se cierra, así que
// perder una línea parcial es inofensivo.
func (s *sess) finishLate() {
	for s.late != nil {
		s.cc.SetReadDeadline(s.late.sentAt.Add(probeTimeout))
		m, err := s.codec.Read()
		if err != nil || m.Type != proto.TypePong {
			s.giveUpLate()
			return
		}
		s.latePong(m.Seq)
	}
}

func (l *lateProbe) finish(rtt time.Duration, ok bool) {
	iv := l.iv
	if ok {
		iv.RTT, iv.HasRTT = rtt, true
		if d := l.dst; d != nil && len(d.Intervals) > 0 && d.Intervals[len(d.Intervals)-1].Index == iv.Index {
			d.Intervals[len(d.Intervals)-1] = iv
			d.LoadedRTT = NewLoadedRTT(append(l.answered, rtt), l.sent)
			if !l.lv.noted {
				l.lv.noteFor(d.LoadedRTT)
			}
		}
	}
	if cb := l.lv.onInterval; cb != nil {
		cb(l.lv.phase, iv)
	}
}

func (lv *liveRun) label() string {
	proto, dir := "TCP", "ida"
	if lv.udp {
		proto = "UDP"
	}
	if len(lv.phase) >= 3 && lv.phase[len(lv.phase)-3:] == "rev" {
		dir = "vuelta"
	}
	return proto + " " + dir
}

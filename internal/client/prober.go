package client

import (
	"context"
	"sync"
	"time"
)

const (
	probeInterval = time.Second
	probeTimeout  = time.Second
)

// Clock es el reloj inyectable del prober.
type Clock interface {
	Now() time.Time
	// After devuelve un canal que recibe tras d.
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// ProbeResult es el desenlace de la sonda Seq: OK con RTT, o sin respuesta.
type ProbeResult struct {
	Seq int
	RTT time.Duration
	OK  bool
}

// Prober envía la sonda k a k s del inicio de la fase (k < count), con una sola en vuelo
// y 1 s de plazo. Es una máquina de estados: Tick y Pong reciben la hora; Run la conduce.
type Prober struct {
	send  func(seq int) error
	start time.Time
	count int

	mu       sync.Mutex
	next     int
	inflight bool
	seq      int
	sentAt   time.Time
}

// NewProber crea un prober; send escribe el ping{seq} en el control.
func NewProber(start time.Time, count int, send func(seq int) error) *Prober {
	return &Prober{send: send, start: start, count: count}
}

// Tick expira la sonda vencida y envía la siguiente si toca. Devuelve las sondas resueltas
// (solo por falta de respuesta). Un error de send se devuelve y detiene el envío.
func (p *Prober) Tick(now time.Time) ([]ProbeResult, error) {
	p.mu.Lock()
	var out []ProbeResult
	if p.inflight && now.Sub(p.sentAt) >= probeTimeout {
		out = append(out, ProbeResult{Seq: p.seq})
		p.inflight = false
	}
	if p.inflight || p.next >= p.count || now.Before(p.start.Add(time.Duration(p.next)*probeInterval)) {
		p.mu.Unlock()
		return out, nil
	}
	seq := p.next
	p.seq, p.sentAt, p.inflight = seq, now, true
	p.next++
	p.mu.Unlock()

	// El envío va fuera del lock: una escritura lenta no bloquea a Pong.
	if err := p.send(seq); err != nil {
		p.mu.Lock()
		if p.inflight && p.seq == seq {
			p.inflight = false
		}
		p.next = p.count
		p.mu.Unlock()
		return out, err
	}
	return out, nil
}

// Pong registra un pong; ignora el tardío (ya expirado) o de otro seq.
func (p *Prober) Pong(seq int, now time.Time) (ProbeResult, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.inflight || seq != p.seq || now.Sub(p.sentAt) >= probeTimeout {
		return ProbeResult{}, false
	}
	p.inflight = false
	return ProbeResult{Seq: seq, RTT: now.Sub(p.sentAt), OK: true}, true
}

// InFlight indica si hay una sonda pendiente.
func (p *Prober) InFlight() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inflight
}

// Done indica que se enviaron todas y ninguna sigue en vuelo.
func (p *Prober) Done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.next >= p.count && !p.inflight
}

// nextDeadline es la próxima hora en que Tick puede hacer algo; ok=false si no hay más.
func (p *Prober) nextDeadline() (time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inflight {
		return p.sentAt.Add(probeTimeout), true
	}
	if p.next < p.count {
		return p.start.Add(time.Duration(p.next) * probeInterval), true
	}
	return time.Time{}, false
}

// Run llama a Tick en cada plazo hasta terminar o cancelar el contexto; onResult recibe
// las sondas sin respuesta. Los pong se alimentan aparte con Pong y despiertan por wake.
func (p *Prober) Run(ctx context.Context, clk Clock, wake <-chan struct{}, onResult func(ProbeResult)) error {
	if clk == nil {
		clk = realClock{}
	}
	for {
		res, err := p.Tick(clk.Now())
		for _, r := range res {
			onResult(r)
		}
		if err != nil {
			return err
		}
		dl, ok := p.nextDeadline()
		if !ok {
			return nil
		}
		wait := dl.Sub(clk.Now())
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
		case <-clk.After(wait):
		}
	}
}

package client_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/server"
)

// qEnv levanta un servidor cuyo turno queda retenido por el gancho hasta que el test lo suelte.
type qEnv struct {
	addr    string
	entered chan struct{} // un valor por turno concedido (en orden)
	free    chan struct{} // una señal suelta un turno retenido
}

func newQEnv(t *testing.T, opts ...server.Option) *qEnv {
	t.Helper()
	e := &qEnv{entered: make(chan struct{}, 64), free: make(chan struct{}, 64)}
	hook := server.WithTurnHook(func(ctx context.Context) {
		e.entered <- struct{}{}
		select {
		case <-e.free:
		case <-ctx.Done():
		}
	})
	e.addr = startWith(t, append([]server.Option{hook, server.WithMaxPerIP(0)}, opts...)...)
	return e
}

type qClient struct {
	mu     sync.Mutex
	pos    []int
	err    error
	res    client.Result
	done   chan struct{}
	cancel context.CancelFunc
	posCh  chan int
}

func (e *qEnv) launch(o client.Options) *qClient {
	ctx, cancel := context.WithCancel(context.Background())
	c := &qClient{done: make(chan struct{}), cancel: cancel, posCh: make(chan int, 16)}
	o.OnQueue = func(p int) { c.mu.Lock(); c.pos = append(c.pos, p); c.mu.Unlock(); c.posCh <- p }
	go func() {
		c.res, c.err = client.Run(ctx, e.addr, o)
		close(c.done)
	}()
	return c
}

func waitCh[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(15 * time.Second):
		t.Fatalf("tiempo agotado esperando %s", what)
	}
	panic("unreachable")
}

func (c *qClient) positions() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.pos...)
}

var quick = client.Options{Duration: 200 * time.Millisecond, Proto: "tcp", Streams: 1}

func TestQueueSevenClients(t *testing.T) {
	e := newQEnv(t)
	first := e.launch(quick)
	waitCh(t, e.entered, "turno del 1.º")
	var q []*qClient
	for i := 1; i <= 5; i++ { // posiciones 1..5, una a una para fijar el orden de llegada
		c := e.launch(quick)
		if p := waitCh(t, c.posCh, "posición"); p != i {
			t.Fatalf("cliente %d: posición %d", i+1, p)
		}
		q = append(q, c)
	}
	seventh := e.launch(quick)
	waitCh(t, seventh.done, "rechazo del 7.º")
	if seventh.err == nil || !strings.Contains(seventh.err.Error(), "cola llena") {
		t.Fatalf("7.º: %v", seventh.err)
	}
	if len(seventh.positions()) != 0 {
		t.Fatalf("el 7.º no debía entrar en cola: %v", seventh.positions())
	}
	// FIFO: cada turno concedido sigue el orden de llegada.
	e.free <- struct{}{}
	waitCh(t, first.done, "fin del 1.º")
	if first.err != nil {
		t.Fatal(first.err)
	}
	for i, c := range q {
		waitCh(t, e.entered, "turno de la cola")
		if i > 0 {
			select {
			case <-q[i-1].done:
			default:
				t.Fatalf("el cliente %d tomó turno antes de terminar el anterior", i+2)
			}
		}
		e.free <- struct{}{}
		waitCh(t, c.done, "fin de cola")
		if c.err != nil || !c.res.Fwd.Measured {
			t.Fatalf("cliente en cola %d: %+v %v", i+1, c.res.Fwd, c.err)
		}
	}
	// Los avisos de avance son decrecientes y terminan en 1.
	last := q[4].positions()
	if last[0] != 5 || last[len(last)-1] != 1 {
		t.Fatalf("avisos del último: %v", last)
	}
	for i := 1; i < len(last); i++ {
		if last[i] >= last[i-1] {
			t.Fatalf("avisos no decrecientes: %v", last)
		}
	}
}

func TestQueueCancelSecond(t *testing.T) {
	e := newQEnv(t)
	first := e.launch(quick)
	waitCh(t, e.entered, "turno del 1.º")
	a := e.launch(quick)
	waitCh(t, a.posCh, "posición de a")
	b := e.launch(quick)
	waitCh(t, b.posCh, "posición de b")
	c := e.launch(quick)
	waitCh(t, c.posCh, "posición de c")
	b.cancel()
	waitCh(t, b.done, "salida de b")
	if b.err == nil {
		t.Fatal("el cancelado debe terminar con error")
	}
	if p := waitCh(t, c.posCh, "avance de c"); p != 2 {
		t.Fatalf("c debe avanzar a 2, fue %d", p)
	}
	e.free <- struct{}{} // termina el 1.º
	waitCh(t, first.done, "fin del 1.º")
	waitCh(t, e.entered, "turno de a")
	e.free <- struct{}{}
	waitCh(t, a.done, "fin de a")
	waitCh(t, e.entered, "turno de c") // b no recibe turno
	e.free <- struct{}{}
	waitCh(t, c.done, "fin de c")
	if a.err != nil || c.err != nil || first.err != nil {
		t.Fatalf("a=%v c=%v first=%v", a.err, c.err, first.err)
	}
	select {
	case <-e.entered:
		t.Fatal("el cancelado no debía obtener turno")
	default:
	}
}

// both usa una sola sesión: nadie se intercala entre fwd y rev.
func TestBothSingleTurn(t *testing.T) {
	e := newQEnv(t)
	o := client.Options{Duration: 300 * time.Millisecond, Proto: "tcp", Direction: "both", Streams: 1}
	a := e.launch(o)
	waitCh(t, e.entered, "turno de a")
	b := e.launch(quick)
	waitCh(t, b.posCh, "b en cola")
	e.free <- struct{}{}
	waitCh(t, a.done, "fin de a")
	if a.err != nil || !a.res.Fwd.Measured || !a.res.Rev.Measured {
		t.Fatalf("both: %+v %v", a.res, a.err)
	}
	// Solo se concedió un turno a a (fwd y rev juntos); el siguiente es el de b.
	waitCh(t, e.entered, "turno de b")
	select {
	case <-e.entered:
		t.Fatal("both consumió más de un turno")
	default:
	}
	e.free <- struct{}{}
	waitCh(t, b.done, "fin de b")
}

// Una espera en cola más larga que todos los plazos no corta al cliente.
func TestQueueLongWaitNoDeadlines(t *testing.T) {
	e := newQEnv(t, server.WithHandshakeTimeout(100*time.Millisecond))
	o := quick
	o.HandshakeWait = 100 * time.Millisecond
	first := e.launch(quick) // el gancho retiene su turno antes de "ready": plazo normal
	waitCh(t, e.entered, "turno del 1.º")
	w := e.launch(o)
	waitCh(t, w.posCh, "en cola")
	time.Sleep(700 * time.Millisecond) // 7 plazos de handshake
	select {
	case <-w.done:
		t.Fatalf("el cliente en cola terminó: %v", w.err)
	default:
	}
	e.free <- struct{}{}
	waitCh(t, first.done, "fin del 1.º")
	waitCh(t, e.entered, "turno del que esperaba")
	e.free <- struct{}{}
	waitCh(t, w.done, "fin del que esperaba")
	if w.err != nil || first.err != nil {
		t.Fatalf("w=%v first=%v", w.err, first.err)
	}
}

// Un cliente colgado sin estar en cola sigue cortándose por plazo.
func TestHangWithoutQueueStillCut(t *testing.T) {
	e := newQEnv(t, server.WithHandshakeTimeout(100*time.Millisecond))
	c, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("el servidor debía cortar: %v", err)
	}
}

// El turno se libera aunque el cliente en turno se vaya a mitad.
func TestTurnReleasedOnCancel(t *testing.T) {
	e := newQEnv(t)
	long := client.Options{Duration: 20 * time.Second, Proto: "tcp", Streams: 1}
	a := e.launch(long)
	waitCh(t, e.entered, "turno de a")
	e.free <- struct{}{} // a ya tiene el turno: b hará cola sin esperar nada más
	b := e.launch(quick)
	waitCh(t, b.posCh, "b en cola")
	a.cancel()
	waitCh(t, a.done, "salida de a")
	waitCh(t, e.entered, "turno de b tras cancelar a")
	e.free <- struct{}{}
	waitCh(t, b.done, "fin de b")
	if b.err != nil {
		t.Fatal(b.err)
	}
}

// Un cliente autenticado no puede acaparar la cola desde una misma IP (límite explícito de 2).
func TestPerIPLimit(t *testing.T) {
	e := &qEnv{entered: make(chan struct{}, 8), free: make(chan struct{}, 8)}
	e.addr = startWith(t, server.WithMaxPerIP(2), server.WithTurnHook(func(ctx context.Context) {
		e.entered <- struct{}{}
		select {
		case <-e.free:
		case <-ctx.Done():
		}
	}))
	a := e.launch(quick)
	waitCh(t, e.entered, "turno de a")
	b := e.launch(quick)
	waitCh(t, b.posCh, "b en cola")
	c := e.launch(quick)
	waitCh(t, c.done, "rechazo de c")
	if c.err == nil || !strings.Contains(c.err.Error(), "IP") {
		t.Fatalf("c: %v", c.err)
	}
	e.free <- struct{}{}
	waitCh(t, e.entered, "turno de b")
	e.free <- struct{}{}
	waitCh(t, a.done, "fin de a")
	waitCh(t, b.done, "fin de b")
}

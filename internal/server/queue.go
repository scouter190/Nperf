package server

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"nperf/internal/proto"
)

// Límites de la cola: el servidor mide una prueba a la vez; hasta MaxQueue clientes esperan.
const (
	MaxQueue = 5
)

var (
	errQueueFull = errors.New(proto.ErrQueueFull)
	errIPLimit   = errors.New(proto.ErrIPLimit)
)

// waiter es un cliente autenticado en espera de turno.
type waiter struct {
	pos     int
	upd     chan struct{} // cap 1: la posición cambió
	granted chan struct{} // se cierra al concederle el turno
}

// turnQueue es una cola FIFO con un solo turno activo. Solo entran clientes autenticados.
// Para que un cliente autenticado no acapare puestos abriendo muchas conexiones, se limita
// por IP de origen, solo si se configura WithMaxPerIP (por defecto sin límite; detrás de un NAT compartido la IP agrupa a varios usuarios; es un límite
// conocido y configurable con WithMaxPerIP).
type turnQueue struct {
	mu       sync.Mutex
	busy     bool
	waiters  []*waiter
	perIP    map[string]int
	maxWait  int
	maxPerIP int // 0 = sin límite
}

// enter reserva el turno (w == nil) o un puesto en la cola.
func (q *turnQueue) enter(ip string) (*waiter, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.maxPerIP > 0 && q.perIP[ip] >= q.maxPerIP {
		return nil, errIPLimit
	}
	if !q.busy && len(q.waiters) == 0 {
		q.busy = true
		q.perIP[ip]++
		return nil, nil
	}
	if len(q.waiters) >= q.maxWait {
		return nil, errQueueFull
	}
	w := &waiter{pos: len(q.waiters) + 1, upd: make(chan struct{}, 1), granted: make(chan struct{})}
	q.waiters = append(q.waiters, w)
	q.perIP[ip]++
	return w, nil
}

func (q *turnQueue) dropIP(ip string) {
	if q.perIP[ip]--; q.perIP[ip] <= 0 {
		delete(q.perIP, ip)
	}
}

// renumber actualiza las posiciones (con q.mu tomado) y avisa a quien cambió.
func (q *turnQueue) renumber() {
	for i, w := range q.waiters {
		if w.pos != i+1 {
			w.pos = i + 1
			select {
			case w.upd <- struct{}{}:
			default:
			}
		}
	}
}

// release devuelve el turno y se lo pasa al primero de la cola.
func (q *turnQueue) release(ip string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.dropIP(ip)
	if len(q.waiters) == 0 {
		q.busy = false
		return
	}
	w := q.waiters[0]
	q.waiters = q.waiters[1:]
	close(w.granted) // busy sigue true: el turno pasa directo
	q.renumber()
}

// cancel saca a w de la cola. Devuelve false si ya tenía el turno (el llamador debe liberarlo).
func (q *turnQueue) cancel(w *waiter, ip string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, x := range q.waiters {
		if x == w {
			q.waiters = append(q.waiters[:i], q.waiters[i+1:]...)
			q.dropIP(ip)
			q.renumber()
			return true
		}
	}
	return false
}

func (q *turnQueue) position(w *waiter) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return w.pos
}

// acquireTurn espera el turno tras la solicitud del cliente, avisando "queue" con la posición
// (sin plazos de lectura mientras espera). Devuelve la función que libera el turno; si el
// cliente se va, el servidor se cancela o la cola rechaza, devuelve error y no queda nada retenido.
func (s *Server) acquireTurn(ctx context.Context, c net.Conn, codec *proto.Codec) (func(), error) {
	ip, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	w, err := s.q.enter(ip)
	if err != nil {
		codec.Write(proto.Message{Type: proto.TypeError, Error: err.Error()})
		s.logf("aviso: " + err.Error() + " (" + c.RemoteAddr().String() + ")")
		return nil, err
	}
	release := func() { s.q.release(ip) }
	if w != nil {
		c.SetReadDeadline(time.Time{}) // la espera en cola no cuenta para ningún plazo
		gone := make(chan error, 1)
		s.wg.Add(1)
		go func() { // el cliente no envía nada hasta "ready": cualquier lectura es una salida
			defer s.wg.Done()
			m, err := codec.Read()
			if err == nil {
				err = errors.New("mensaje inesperado en cola: " + m.Type)
			}
			gone <- err
		}()
		if err := codec.Write(proto.Message{Type: proto.TypeQueue, Position: s.q.position(w)}); err != nil {
			s.leaveQueue(c, w, ip, gone)
			return nil, err
		}
	wait:
		for {
			select {
			case <-w.upd:
				if err := codec.Write(proto.Message{Type: proto.TypeQueue, Position: s.q.position(w)}); err != nil {
					s.leaveQueue(c, w, ip, gone)
					return nil, err
				}
			case <-w.granted:
				break wait
			case err := <-gone: // cliente desconectado
				if !s.q.cancel(w, ip) {
					release()
				}
				return nil, err
			case <-ctx.Done():
				s.leaveQueue(c, w, ip, gone)
				return nil, ctx.Err()
			}
		}
		// Detiene el lector sin consumir nada (el cliente calla hasta "ready").
		c.SetReadDeadline(time.Now())
		if err := <-gone; !errors.Is(err, os.ErrDeadlineExceeded) {
			release()
			return nil, err
		}
	}
	if s.turnHook != nil {
		s.turnHook(ctx)
	}
	c.SetReadDeadline(time.Now().Add(s.hsWait)) // plazo único hasta recibir params
	ready := proto.Message{Type: proto.TypeReady}
	if s.linkFn != nil {
		ready.Link = s.linkFn(c.LocalAddr())
	}
	if err := codec.Write(ready); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// leaveQueue abandona la cola (o libera el turno si ya se había concedido) y para el lector.
func (s *Server) leaveQueue(c net.Conn, w *waiter, ip string, gone chan error) {
	if !s.q.cancel(w, ip) {
		s.q.release(ip)
	}
	c.SetReadDeadline(time.Now())
	<-gone
}

// snapshot devuelve cuántos clientes esperan y si hay una prueba en curso (solo lectura: no
// ocupa turno ni entra en la cola).
func (q *turnQueue) snapshot() (busy bool, waiting int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.busy, len(q.waiters)
}

// Package meter mide throughput. Acepta cualquier time.Duration.
package meter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"nperf/internal/tcpinfo"
)

// Result es lo que mide quien recibe.
type Result struct {
	Bytes   int64
	Elapsed time.Duration
	// Retransmisiones TCP vistas por el emisor (segmentos; en Windows aproximado desde bytes).
	// Solo válido si RetransOK; si no, "no disponible".
	Retrans   uint64
	RetransOK bool
	// RetransEstimated: el valor es una estimación (Windows), se muestra con ~.
	RetransEstimated bool
}

// Mbps devuelve megabits por segundo.
func (r Result) Mbps() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Bytes) * 8 / r.Elapsed.Seconds() / 1e6
}

const writeGrace = 2 * time.Second

// SendTCP escribe en c durante d (o hasta cancelar ctx) y cierra la escritura.
func SendTCP(ctx context.Context, c net.Conn, d time.Duration) (Result, error) {
	buf := make([]byte, 128*1024)
	start := time.Now()
	end := start.Add(d)
	stop := context.AfterFunc(ctx, func() { c.SetWriteDeadline(time.Now()) })
	defer stop()
	var total int64
	var err error
	// Un solo deadline (antes del bucle) para no pisar el de la cancelación.
	c.SetWriteDeadline(end.Add(writeGrace))
	for time.Now().Before(end) && ctx.Err() == nil {
		var n int
		n, err = c.Write(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = errors.New("envío bloqueado: medición no fiable")
	}
	res := Result{Bytes: total, Elapsed: time.Since(start)}
	if tc, ok := c.(*net.TCPConn); ok {
		// Conexión nueva: el contador acumulado equivale a lo retransmitido en la prueba.
		if n, rerr := tcpinfo.Retransmits(tc); rerr == nil {
			res.Retrans, res.RetransOK, res.RetransEstimated = n, true, tcpinfo.Estimated
		}
		tc.CloseWrite()
	}
	return res, err
}

// ReceiveTCP lee hasta EOF y mide desde el primer byte hasta el último.
func ReceiveTCP(r io.Reader) (Result, error) {
	buf := make([]byte, 128*1024)
	var total int64
	var first, last time.Time
	for {
		n, err := r.Read(buf)
		if n > 0 {
			last = time.Now()
			if first.IsZero() {
				first = last
			}
			total += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{Bytes: total, Elapsed: last.Sub(first)}, err
		}
	}
	return Result{Bytes: total, Elapsed: last.Sub(first)}, nil
}

// SendTCPN envía en paralelo por todas las conexiones durante d y suma los bytes. Si un
// flujo falla, cancela los demás y devuelve ese primer error: no hay suma parcial válida.
func SendTCPN(ctx context.Context, conns []net.Conn, d time.Duration) (Result, error) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		total    Result
		firstErr error
	)
	total.RetransOK = len(conns) > 0
	for i, c := range conns {
		wg.Add(1)
		go func(i int, c net.Conn) {
			defer wg.Done()
			r, err := SendTCP(sctx, c, d)
			mu.Lock()
			defer mu.Unlock()
			total.Bytes += r.Bytes
			total.Retrans += r.Retrans
			total.RetransEstimated = total.RetransEstimated || r.RetransEstimated
			total.RetransOK = total.RetransOK && r.RetransOK
			if r.Elapsed > total.Elapsed {
				total.Elapsed = r.Elapsed
			}
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("falló el flujo %d de %d: %w", i+1, len(conns), err)
				cancel()
			}
		}(i, c)
	}
	wg.Wait()
	if firstErr != nil && ctx.Err() != nil {
		return total, ctx.Err()
	}
	return total, firstErr
}

// ReceiveTCPN lee todos los flujos en paralelo hasta EOF. La ventana de medición es común:
// del primer byte del primer flujo al último byte del último, así un flujo lento o que
// conecta tarde no distorsiona la duración. Si un flujo falla se llama a abort (si no es
// nil) para liberar a los demás y se devuelve el error: no hay suma parcial válida.
func ReceiveTCPN(rs []io.Reader, abort func()) (Result, error) {
	return receiveTCPN(rs, abort, nil)
}

// tcpSampleTick es el periodo de lectura de los contadores en ReceiveTCPNSamples.
const tcpSampleTick = 10 * time.Millisecond

// ReceiveTCPNSamples es ReceiveTCPN con muestreo: además del Result idéntico, entrega a out
// (sin bloquear) las muestras de 1 s de una fase de duración d. Cada flujo solo suma un
// contador atómico por lectura; una goroutine lee la suma y alimenta la Series. Antes de
// volver cierra la serie (Finish) y entrega la última muestra. out puede ser nil.
func ReceiveTCPNSamples(rs []io.Reader, abort func(), d time.Duration, out chan<- Sample) (Result, error) {
	tk := time.NewTicker(tcpSampleTick)
	defer tk.Stop()
	return receiveTCPN(rs, abort, &tcpSampler{
		series: NewSeries(d, out), now: time.Now, tick: tk.C,
	})
}

// tcpSampler agrupa la serie, el reloj y los bordes (inyectables en tests).
type tcpSampler struct {
	series *Series
	now    func() time.Time
	tick   <-chan time.Time
}

// run lee la suma de contadores en cada tick hasta stop; al final suma lo pendiente y cierra.
func (s *tcpSampler) run(counters []atomic.Int64, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	var prev int64
	poll := func(now time.Time) {
		var sum int64
		for i := range counters {
			sum += counters[i].Load()
		}
		if sum > prev {
			s.series.Add(now, sum-prev)
			prev = sum
		} else {
			s.series.Observe(now)
		}
	}
	for {
		select {
		case <-s.tick:
			poll(s.now())
		case <-stop:
			now := s.now()
			poll(now)
			s.series.Finish(now)
			return
		}
	}
}

func receiveTCPN(rs []io.Reader, abort func(), smp *tcpSampler) (Result, error) {
	var counters []atomic.Int64
	var stop, done chan struct{}
	if smp != nil {
		counters = make([]atomic.Int64, len(rs))
		stop, done = make(chan struct{}), make(chan struct{})
		go smp.run(counters, stop, done)
	}
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		total       int64
		first, last time.Time
		firstErr    error
	)
	for i, r := range rs {
		wg.Add(1)
		go func(i int, r io.Reader) {
			defer wg.Done()
			buf := make([]byte, 128*1024)
			var n64 int64
			var f, l time.Time
			var err error
			var ctr *atomic.Int64
			if counters != nil {
				ctr = &counters[i]
			}
			for {
				var n int
				n, err = r.Read(buf)
				if n > 0 {
					if ctr != nil {
						ctr.Add(int64(n))
					}
					l = time.Now()
					if f.IsZero() {
						f = l
					}
					n64 += int64(n)
				}
				if err != nil {
					break
				}
			}
			mu.Lock()
			defer mu.Unlock()
			total += n64
			if !f.IsZero() && (first.IsZero() || f.Before(first)) {
				first = f
			}
			if l.After(last) {
				last = l
			}
			if err != nil && err != io.EOF && firstErr == nil {
				firstErr = fmt.Errorf("falló el flujo %d de %d: %w", i+1, len(rs), err)
				if abort != nil {
					abort()
				}
			}
		}(i, r)
	}
	wg.Wait()
	if smp != nil {
		close(stop)
		<-done
	}
	res := Result{Bytes: total}
	if !first.IsZero() {
		res.Elapsed = last.Sub(first)
	}
	return res, firstErr
}

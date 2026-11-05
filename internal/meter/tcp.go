// Package meter mide throughput. Acepta cualquier time.Duration.
package meter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Result es lo que mide quien recibe.
type Result struct {
	Bytes   int64
	Elapsed time.Duration
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
	if tc, ok := c.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	return Result{Bytes: total, Elapsed: time.Since(start)}, err
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
	for i, c := range conns {
		wg.Add(1)
		go func(i int, c net.Conn) {
			defer wg.Done()
			r, err := SendTCP(sctx, c, d)
			mu.Lock()
			defer mu.Unlock()
			total.Bytes += r.Bytes
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
			for {
				var n int
				n, err = r.Read(buf)
				if n > 0 {
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
	res := Result{Bytes: total}
	if !first.IsZero() {
		res.Elapsed = last.Sub(first)
	}
	return res, firstErr
}

// Package meter mide throughput. Acepta cualquier time.Duration.
package meter

import (
	"context"
	"io"
	"net"
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

// SendTCP escribe en c durante d (o hasta cancelar ctx) y cierra la escritura.
func SendTCP(ctx context.Context, c net.Conn, d time.Duration) (Result, error) {
	buf := make([]byte, 128*1024)
	start := time.Now()
	end := start.Add(d)
	stop := context.AfterFunc(ctx, func() { c.SetWriteDeadline(time.Now()) })
	defer stop()
	var total int64
	var err error
	for time.Now().Before(end) && ctx.Err() == nil {
		c.SetWriteDeadline(end)
		var n int
		n, err = c.Write(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = nil // fin normal por plazo
	}
	if tc, ok := c.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	if err == nil {
		err = ctx.Err()
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

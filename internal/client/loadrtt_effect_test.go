package client_test

import (
	"context"
	"io"
	"math"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
)

// Criterio 4 de la spec 005: las sondas de RTT bajo carga no alteran la velocidad medida.
//
// En loopback la velocidad real depende de la CPU del runner y varía más de 1 % entre
// repeticiones, así que la velocidad la fija un token bucket compartido por todas las
// conexiones del cliente (DialContext): ~200 Mbps, muy por debajo de lo que da la CPU. Si el
// limitador manda, la diferencia entre grupos solo puede venir de las sondas. Se alternan
// 3 TEST! con sondas y 3 con NoLoadRTT; se comparan la mediana de Mbps por TEST! y la media de
// bytes por intervalo (sin el primero ni el último, que son parciales).

const (
	effectRate     = 200e6 / 8 // bytes/s
	effectBurst    = 30 * time.Millisecond
	effectDuration = 4 * time.Second
	effectRuns     = 6
	effectMaxDiff  = 0.01
)

// bucket reparte el tiempo de línea: cada escritura reserva su hueco y duerme hasta él menos
// una ráfaga, de modo que la granularidad del reloj del sistema no afecta a la tasa media.
type bucket struct {
	mu   sync.Mutex
	next time.Time
}

func (b *bucket) wait(n int) {
	b.mu.Lock()
	now := time.Now()
	if b.next.Before(now.Add(-effectBurst)) {
		b.next = now.Add(-effectBurst)
	}
	b.next = b.next.Add(time.Duration(float64(n) / effectRate * float64(time.Second)))
	at := b.next
	b.mu.Unlock()
	if d := time.Until(at); d > 0 {
		time.Sleep(d)
	}
}

// limitProxy es un proxy TCP delante del servidor que limita el sentido cliente->servidor con
// el bucket compartido. Se usa un proxy y no un net.Conn envuelto porque meter.SendTCP necesita
// un *net.TCPConn real (CloseWrite marca el fin del envío).
func limitProxy(t *testing.T, target string, b *bucket) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				s, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer s.Close()
				go func() {
					io.Copy(c, s)
					c.(*net.TCPConn).CloseWrite()
				}()
				buf := make([]byte, 16<<10)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						b.wait(n)
						if _, werr := s.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						s.(*net.TCPConn).CloseWrite()
						// Espera a que el servidor termine de contestar.
						io.Copy(io.Discard, s)
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func mean(v []float64) float64 {
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

func TestLoadRTTProbesDoNotAffectSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("dura ~30 s")
	}
	real, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	addr := limitProxy(t, real, &bucket{})

	var mbps, bytes [2][]float64 // 0 = con sondas, 1 = sin sondas
	for i := 0; i < effectRuns; i++ {
		g := i % 2
		res, err := client.Run(context.Background(), addr, client.Options{
			Duration: effectDuration, Direction: "fwd", Proto: "tcp", NoLoadRTT: g == 1,
		})
		if err != nil {
			t.Fatalf("TEST! %d: %v", i, err)
		}
		ivs := res.Fwd.Intervals
		if len(ivs) < 4 {
			t.Fatalf("TEST! %d: %d intervalos", i, len(ivs))
		}
		if (g == 0) != (res.Fwd.LoadedRTT != nil) {
			t.Fatalf("TEST! %d: grupo %d con LoadedRTT=%v", i, g, res.Fwd.LoadedRTT)
		}
		mbps[g] = append(mbps[g], res.Fwd.Mbps)
		for _, iv := range ivs[1 : len(ivs)-1] {
			bytes[g] = append(bytes[g], float64(iv.Bytes))
		}
	}

	mA, mB := median(mbps[0]), median(mbps[1])
	bA, bB := mean(bytes[0]), mean(bytes[1])
	dM := math.Abs(mA-mB) / math.Max(mA, mB)
	dB := math.Abs(bA-bB) / math.Max(bA, bB)
	t.Logf("con sondas: mediana %.2f Mbps, media %.0f B/intervalo; sin: %.2f Mbps, %.0f B (dif %.3f %% y %.3f %%)",
		mA, bA, mB, bB, dM*100, dB*100)
	if mA < 100 || mB < 100 {
		t.Fatalf("el limitador no fijó la velocidad (%.1f / %.1f Mbps)", mA, mB)
	}
	if dM >= effectMaxDiff {
		t.Errorf("mediana de Mbps difiere %.3f %% (>= 1 %%)", dM*100)
	}
	if dB >= effectMaxDiff {
		t.Errorf("media de bytes por intervalo difiere %.3f %% (>= 1 %%)", dB*100)
	}
}

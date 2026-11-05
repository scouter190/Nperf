package meter

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"runtime"
	"sync"
	"time"
)

// Formato del datagrama: seq uint64 | hora de envío (ns Unix) int64 | flags byte | relleno.
const (
	UDPHeaderLen   = 17
	DefaultUDPSize = 1200
	flagFin        = 1
	finCopies      = 3
)

// UDPSendStats es lo que cuenta el emisor (datagramas intentados, incluso si una envoltura los descarta).
type UDPSendStats struct {
	Packets int64
	Bytes   int64
	Elapsed time.Duration
}

// Mbps devuelve la tasa enviada en megabits por segundo.
func (s UDPSendStats) Mbps() float64 {
	return Result{Bytes: s.Bytes, Elapsed: s.Elapsed}.Mbps()
}

// UDPRecvStats es lo que mide el receptor.
type UDPRecvStats struct {
	Result                   // bytes recibidos y tiempo entre el primer y el último datagrama
	Received   int64         // datagramas de datos recibidos
	Expected   int64         // datagramas esperados según la secuencia
	Duplicates int64         // datagramas repetidos (no suman a Received)
	Lost       int64         // huecos de secuencia
	Jitter     time.Duration // RFC 3550
}

// LossPercent devuelve la pérdida en porcentaje.
func (s UDPRecvStats) LossPercent() float64 {
	if s.Expected <= 0 {
		return 0
	}
	return float64(s.Lost) * 100 / float64(s.Expected)
}

// clock permite probar sin tiempos reales.
type clock struct {
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration)
}

var realClock = clock{
	now: time.Now,
	sleep: func(ctx context.Context, d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
		}
	},
}

const (
	burstPackets = 10                    // tope de datagramas por iteración tras un atraso
	debtWindow   = 20 * time.Millisecond // deuda máxima conservada; lo que exceda se perdona
)

// SendUDP envía datagramas de size bytes a rateBps (bits por segundo) durante d.
// El ritmo sale de los bytes adeudados según el tiempo transcurrido, no de un sleep por paquete.
// Cada iteración envía como máximo burstPackets datagramas. La deuda por atraso se conserva
// hasta debtWindow (20 ms, cubre sleeps de 16 ms); solo lo que la excede (pausas largas reales) se descarta.
func SendUDP(ctx context.Context, w io.Writer, rateBps float64, size int, d time.Duration) (UDPSendStats, error) {
	return sendUDP(ctx, w, rateBps, size, d, realClock)
}

func sendUDP(ctx context.Context, w io.Writer, rateBps float64, size int, d time.Duration, clk clock) (UDPSendStats, error) {
	if size < UDPHeaderLen {
		size = DefaultUDPSize
	}
	if rateBps <= 0 {
		return UDPSendStats{}, errors.New("udp rate must be positive")
	}
	buf := make([]byte, size)
	bytesPerSec := rateBps / 8
	maxDebt := math.Max(float64(burstPackets*size), debtWindow.Seconds()*bytesPerSec)
	var seq uint64
	var sentBytes int64
	var forgiven float64 // deuda descartada
	start := clk.now()
	var err error
	for ctx.Err() == nil {
		elapsed := clk.now().Sub(start)
		if elapsed >= d {
			break
		}
		owed := bytesPerSec*elapsed.Seconds() - float64(sentBytes) - forgiven
		if owed < float64(size) {
			wait := time.Duration((float64(size) - owed) / bytesPerSec * float64(time.Second))
			if wait > d-elapsed {
				wait = d - elapsed
			}
			clk.sleep(ctx, wait)
			continue
		}
		if owed > maxDebt {
			forgiven += owed - maxDebt
			owed = maxDebt
		}
		for i := 0; i < burstPackets && owed >= float64(size); i++ {
			putHeader(buf, seq, 0, clk.now())
			if _, err = w.Write(buf); err != nil {
				break
			}
			seq++
			sentBytes += int64(size)
			owed -= float64(size)
		}
		runtime.Gosched() // cede entre iteraciones; la ráfaga total queda acotada por debtWindow
		if err != nil {
			break
		}
	}
	stats := UDPSendStats{Packets: int64(seq), Bytes: sentBytes, Elapsed: clk.now().Sub(start)}
	if err != nil {
		return stats, err
	}
	// Marca de fin, repetida por si alguna se pierde. No cuenta como dato.
	for i := 0; i < finCopies; i++ {
		putHeader(buf, seq, flagFin, clk.now())
		if _, werr := w.Write(buf); werr != nil {
			break
		}
	}
	return stats, ctx.Err()
}

func putHeader(buf []byte, seq uint64, flags byte, now time.Time) {
	binary.BigEndian.PutUint64(buf[0:8], seq)
	binary.BigEndian.PutUint64(buf[8:16], uint64(now.UnixNano()))
	buf[16] = flags
}

// PacketReader lo cumple *net.UDPConn.
type PacketReader interface {
	Read([]byte) (int, error)
	SetReadDeadline(time.Time) error
}

// maxSeq razonable para el bitmap de duplicados (2^28 datagramas); más allá se ignora.
const maxTrackedSeq = 1 << 28

// ReceiveUDP lee datagramas hasta la marca de fin, hasta que pasen idle sin recibir
// (tras el primer datagrama) o hasta cancelar ctx. Los duplicados se cuentan aparte.
func ReceiveUDP(ctx context.Context, r PacketReader, idle time.Duration) (UDPRecvStats, error) {
	return receiveUDP(ctx, r, idle, realClock)
}

func receiveUDP(ctx context.Context, r PacketReader, idle time.Duration, clk clock) (UDPRecvStats, error) {
	if idle <= 0 {
		idle = time.Second
	}
	stop := context.AfterFunc(ctx, func() { r.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, 65536)
	var st UDPRecvStats
	var first, last time.Time
	var maxSeq uint64
	var haveSeq bool
	var finSeq uint64
	var haveFin bool
	var seen []uint64  // bitmap de secuencias vistas
	var jitter float64 // ns
	var prevTransit int64
	var havePrev bool
	var err error
	for {
		if !first.IsZero() {
			r.SetReadDeadline(time.Now().Add(idle))
		}
		var n int
		n, err = r.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() && ctx.Err() == nil && !first.IsZero() {
				err = nil // silencio tras el primer datagrama: fin
			}
			break
		}
		if n < UDPHeaderLen {
			continue
		}
		now := clk.now()
		seq := binary.BigEndian.Uint64(buf[0:8])
		sent := int64(binary.BigEndian.Uint64(buf[8:16]))
		if buf[16]&flagFin != 0 {
			finSeq, haveFin = seq, true
			break
		}
		if seq >= maxTrackedSeq {
			continue
		}
		word, bit := seq/64, seq%64
		for uint64(len(seen)) <= word {
			seen = append(seen, 0)
		}
		if seen[word]&(1<<bit) != 0 {
			st.Duplicates++
			continue
		}
		seen[word] |= 1 << bit
		if first.IsZero() {
			first = now
		}
		last = now
		st.Received++
		st.Bytes += int64(n)
		if !haveSeq || seq > maxSeq {
			maxSeq, haveSeq = seq, true
		}
		transit := now.UnixNano() - sent
		if havePrev {
			dd := transit - prevTransit
			if dd < 0 {
				dd = -dd
			}
			jitter += (float64(dd) - jitter) / 16
		}
		prevTransit, havePrev = transit, true
	}
	if haveSeq {
		st.Expected = int64(maxSeq) + 1
	}
	if haveFin && int64(finSeq) > st.Expected && finSeq < maxTrackedSeq {
		st.Expected = int64(finSeq)
	}
	if st.Expected > st.Received {
		st.Lost = st.Expected - st.Received
	}
	st.Jitter = time.Duration(jitter)
	if !first.IsZero() {
		st.Elapsed = last.Sub(first)
	}
	if err == nil {
		return st, nil
	}
	if ctx.Err() != nil {
		return st, ctx.Err()
	}
	return st, err
}

// lossyWriter descarta una fracción fija de los datagramas (para pruebas).
type lossyWriter struct {
	w    io.Writer
	frac float64
	mu   sync.Mutex
	acc  float64
}

// NewLossyWriter envuelve w y descarta, de forma determinista, la fracción frac (0..1) de las escrituras.
// Informa éxito al llamador, como haría la red al perder un datagrama.
func NewLossyWriter(w io.Writer, frac float64) io.Writer {
	return &lossyWriter{w: w, frac: frac}
}

func (l *lossyWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.acc += l.frac
	drop := l.acc >= 1-1e-9
	if drop {
		l.acc -= 1
	}
	l.mu.Unlock()
	if drop {
		return len(p), nil
	}
	return l.w.Write(p)
}

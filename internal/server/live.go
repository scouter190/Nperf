package server

import (
	"net"
	"sync"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

// maxPongs es el tope de pong que se contestan en una fase live: duración en s + 10.
func maxPongs(d time.Duration) int { return int(d/time.Second) + 10 }

// liveWriteWait es el plazo de cada escritura de muestras y pongs: un cliente que deja de leer
// sin cerrar no puede retener el turno.
var liveWriteWait = 5 * time.Second

// liveWriter escribe en el control con plazo, serializando muestras y pongs. Si una escritura
// falla (plazo vencido o conexión rota) cierra la conexión: la fase falla como con un cliente caído.
type liveWriter struct {
	mu    sync.Mutex
	c     net.Conn
	codec *proto.Codec
}

func (w *liveWriter) write(m proto.Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.c.SetWriteDeadline(time.Now().Add(liveWriteWait))
	err := w.codec.Write(m)
	w.c.SetWriteDeadline(time.Time{})
	if err != nil {
		w.c.Close()
	}
	return err
}

// startSamples crea la cola de muestras de una fase live en fwd y la goroutine que las escribe
// por el control. La función devuelta cierra la cola y espera a que todas estén escritas: debe
// llamarse antes de enviar "result". Sin live devuelve (nil, no-op) y no se envía nada.
func (s *Server) startSamples(w *liveWriter, live bool) (chan<- meter.Sample, func()) {
	if !live {
		return nil, func() {}
	}
	ch := make(chan meter.Sample, proto.MaxSamples+10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		failed := false
		for sm := range ch {
			if failed {
				continue // sigue vaciando; el control ya falló
			}
			mbps := sm.Mbps()                                       // duración mínima 1 ms (relojes gruesos)
			durMS := (sm.Duration.Nanoseconds() + 999999) / 1000000 // hacia arriba
			if durMS < 1 && sm.Bytes > 0 {
				durMS = 1
			}
			m := proto.SampleMessage(proto.Sample{
				Index: sm.Index, TMS: sm.Start.Milliseconds(), DurationMS: durMS,
				Bytes: sm.Bytes, Mbps: mbps, Received: sm.Received, Expected: sm.Expected,
				Lost: sm.Lost, JitterMS: float64(sm.Jitter) / float64(time.Millisecond),
				LossPct: lossPct(sm),
			})
			if w.write(m) != nil {
				failed = true
			}
		}
	}()
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch); <-done }) }
}

func lossPct(sm meter.Sample) float64 {
	if !sm.UDP || sm.Expected <= 0 {
		return 0
	}
	return float64(sm.Lost) * 100 / float64(sm.Expected)
}

// answerPings contesta con pong los ping que llegan entre fases de una sesión live. Devuelve el
// primer mensaje que no es un ping.
func answerPings(codec *proto.Codec, m proto.Message, err error) (proto.Message, error) {
	for n := 0; err == nil && m.Type == proto.TypePing && n < proto.MaxPings; n++ {
		if err = codec.Write(proto.Message{Type: proto.TypePong, Seq: m.Seq}); err == nil {
			m, err = codec.Read()
		}
	}
	return m, err
}

// writerFor devuelve el liveWriter único de la conexión de control (compartido por el emisor
// de muestras y el lector de pings de la fase).
func (s *Server) writerFor(c net.Conn, codec *proto.Codec) *liveWriter {
	w, _ := s.lws.LoadOrStore(codec, &liveWriter{c: c, codec: codec})
	return w.(*liveWriter)
}

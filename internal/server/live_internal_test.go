package server

import (
	"net"
	"testing"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

// Un cliente que no lee el control no debe bloquear el vaciado de muestras ni el turno.
func TestLiveWriteDeadlineUnblocksFlush(t *testing.T) {
	old := liveWriteWait
	liveWriteWait = 100 * time.Millisecond
	defer func() { liveWriteWait = old }()
	c1, c2 := net.Pipe() // net.Pipe no tiene buffer: sin lector, Write bloquea
	defer c2.Close()
	s := New()
	w := s.writerFor(c1, proto.NewCodec(c1))
	out, flush := s.startSamples(w, true)
	out <- meter.Sample{Index: 0, Duration: time.Second, Bytes: 1000}
	out <- meter.Sample{Index: 1, Duration: time.Second, Bytes: 1000}
	done := make(chan struct{})
	go func() { flush(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("flush bloqueado por un cliente que no lee")
	}
	if _, err := c1.Write([]byte("x")); err == nil {
		t.Error("la conexión debía quedar cerrada tras vencer el plazo")
	}
}

// Una ventana de ~0 ms con bytes se informa con DurationMS >= 1 y se redondea hacia arriba.
func TestLiveSampleDurationAtLeast1ms(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	s := New()
	w := s.writerFor(c1, proto.NewCodec(c1))
	out, flush := s.startSamples(w, true)
	rd := proto.NewCodec(c2)
	got := make(chan []proto.Message, 1)
	go func() {
		var ms []proto.Message
		for i := 0; i < 2; i++ {
			m, err := rd.Read()
			if err != nil {
				break
			}
			ms = append(ms, m)
		}
		got <- ms
	}()
	out <- meter.Sample{Index: 0, Duration: 0, Bytes: 1000}
	out <- meter.Sample{Index: 1, Duration: 1500 * time.Microsecond, Bytes: 1000}
	flush()
	ms := <-got
	if len(ms) != 2 || ms[0].DurationMS != 1 || ms[1].DurationMS != 2 {
		t.Fatalf("duraciones inesperadas: %+v", ms)
	}
	if _, err := proto.SampleFromMessage(ms[0], -1, 2000); err != nil {
		t.Fatal(err)
	}
}

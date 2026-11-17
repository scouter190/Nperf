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

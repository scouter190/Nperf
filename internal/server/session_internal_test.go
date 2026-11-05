package server

import (
	"net"
	"testing"
)

// closePhase cierra las conexiones que quedaron en el canal y rechaza las posteriores.
func TestClosePhaseDrainsAndRejects(t *testing.T) {
	ss := &session{ch: make(chan dataConn, 4)}
	ss.limit.Store(2)
	c1, p1 := net.Pipe()
	defer p1.Close()
	done := make(chan struct{})
	if !ss.admit(dataConn{conn: c1, done: done}) {
		t.Fatal("debía admitirse")
	}
	ss.closePhase()
	select {
	case <-done:
	default:
		t.Fatal("done no se cerró")
	}
	if _, err := c1.Write([]byte{1}); err == nil {
		t.Fatal("la conexión debía estar cerrada")
	}
	c2, p2 := net.Pipe()
	defer p2.Close()
	defer c2.Close()
	if ss.admit(dataConn{conn: c2, done: make(chan struct{})}) {
		t.Fatal("no debía admitirse tras la fase")
	}
}

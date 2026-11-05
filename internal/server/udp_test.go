package server

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/proto"
)

func startTest(t *testing.T) (*Server, *net.TCPAddr) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := New()
	done := make(chan struct{})
	go func() { s.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return s, ln.Addr().(*net.TCPAddr)
}

func udpConn(t *testing.T, a *net.TCPAddr) *net.UDPConn {
	t.Helper()
	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: a.IP, Port: a.Port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// greet envía un saludo y dice si llegó el acuse.
func greet(c *net.UDPConn, id string) bool {
	c.Write([]byte(UDPMagic + id))
	c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	return err == nil && string(buf[:n]) == UDPMagic+"ok"
}

func sizes(s *Server) (int, int) {
	s.udp.mu.Lock()
	defer s.udp.mu.Unlock()
	return len(s.udp.byID), len(s.udp.byAddr)
}

func TestUDPGreeting(t *testing.T) {
	s, a := startTest(t)
	// Sesión UDP real por el canal de control.
	cc, err := net.Dial("tcp", a.String())
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	codec := proto.NewCodec(cc)
	codec.Write(proto.Message{Type: proto.TypeHello})
	hello, err := codec.Read()
	if err != nil {
		t.Fatal(err)
	}
	codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoUDP, Direction: proto.DirFwd, DurationMS: 1000})
	if m, err := codec.Read(); err != nil || m.Type != proto.TypeStart {
		t.Fatalf("start: %+v %v", m, err)
	}

	c1, c2 := udpConn(t, a), udpConn(t, a)
	if greet(c1, "desconocido") {
		t.Error("id desconocido recibió acuse")
	}
	if _, n := sizes(s); n != 0 {
		t.Error("id desconocido se asoció")
	}
	if !greet(c1, hello.Session) {
		t.Fatal("saludo válido sin acuse")
	}
	if !greet(c1, hello.Session) {
		t.Error("reintento desde la misma dirección sin acuse")
	}
	if greet(c2, hello.Session) {
		t.Error("saludo desde otra dirección recibió acuse")
	}
	if _, n := sizes(s); n != 1 {
		t.Errorf("byAddr = %d, quería 1", n)
	}

	// Un dato y la marca de fin terminan la sesión.
	pkt := make([]byte, 100)
	c1.Write(pkt)
	pkt[16] = 1
	pkt[7] = 1
	c1.Write(pkt)
	if m, err := codec.Read(); err != nil || m.Type != proto.TypeResult {
		t.Fatalf("resultado: %+v %v", m, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		id, ad := sizes(s)
		if id == 0 && ad == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mapas no vacíos: byID=%d byAddr=%d", id, ad)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if greet(c1, hello.Session) {
		t.Error("sesión expirada recibió acuse")
	}
}

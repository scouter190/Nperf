package server_test

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/version"
)

type countLn struct {
	net.Listener
	n   atomic.Int64
	srv *server.Server
}

func (l *countLn) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.n.Add(1)
	}
	return c, err
}

func startCounting(t *testing.T) (*countLn, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New()
	cl := &countLn{Listener: ln, srv: srv}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.Serve(ctx, cl); close(done) }()
	return cl, func() { cancel(); <-done }
}

// params abre el canal de control, autentica y envía params; devuelve el códec y la respuesta.
func params(t *testing.T, addr string, dir string, streams int) (net.Conn, *proto.Codec, string, proto.Message) {
	t.Helper()
	cc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("paso dial (streams=%d, dir=%s): %v", streams, dir, err)
	}
	cc.SetDeadline(time.Now().Add(20 * time.Second))
	codec := proto.NewCodec(cc)
	session, err := proto.ClientHandshake(codec, "", version.Current)
	if err != nil {
		t.Fatalf("paso handshake (streams=%d, dir=%s): %v", streams, dir, err)
	}
	if err := proto.ClientTurn(codec, nil); err != nil {
		t.Fatalf("paso turno (streams=%d, dir=%s): %v", streams, dir, err)
	}
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoTCP, Direction: dir, DurationMS: 300, Streams: streams}); err != nil {
		t.Fatalf("paso write de params (streams=%d, dir=%s): %v", streams, dir, err)
	}
	m, err := codec.Read()
	if err != nil {
		t.Fatalf("paso read de la respuesta (streams=%d, dir=%s): %v", streams, dir, err)
	}
	return cc, codec, session, m
}

func TestStreamsOutOfRangeRejected(t *testing.T) {
	cl, stop := startCounting(t)
	defer stop()
	for _, n := range []int{-1, 65, 1000} {
		cc, _, _, m := params(t, cl.Addr().String(), proto.DirFwd, n)
		cc.Close()
		if m.Type != proto.TypeError || !strings.Contains(m.Error, "fuera de rango") {
			t.Errorf("streams=%d: %+v", n, m)
		}
	}
}

func TestExactDataConns(t *testing.T) {
	for _, dir := range []string{proto.DirFwd, proto.DirRev} {
		cl, stop := startCounting(t)
		cc, codec, session, m := params(t, cl.Addr().String(), dir, 8)
		if m.Type != proto.TypeStart {
			t.Fatalf("%s: %+v", dir, m)
		}
		var data []net.Conn
		for i := 0; i < 8; i++ {
			dc, err := net.Dial("tcp", cl.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			proto.NewCodec(dc).Write(proto.Message{Type: proto.TypeData, Session: session})
			data = append(data, dc)
		}
		deadline := time.Now().Add(5 * time.Second)
		for cl.srv.SessionDataConns(session) < 8 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if got := cl.srv.SessionDataConns(session); got != 8 {
			t.Fatalf("%s: el servidor registró %d de 8 conexiones", dir, got)
		}
		// Una novena conexión de datos debe ser rechazada (se cierra sin más).
		extra, _ := net.Dial("tcp", cl.Addr().String())
		proto.NewCodec(extra).Write(proto.Message{Type: proto.TypeData, Session: session})
		extra.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := extra.Read(make([]byte, 1)); err == nil {
			t.Errorf("%s: la novena conexión no se cerró", dir)
		}
		extra.Close()
		for _, dc := range data {
			if dir == proto.DirFwd {
				dc.Write(make([]byte, 1000))
				dc.Close()
			} else {
				go func(c net.Conn) {
					buf := make([]byte, 64*1024)
					for {
						if _, err := c.Read(buf); err != nil {
							return
						}
					}
				}(dc)
			}
		}
		res, err := codec.Read()
		if err != nil || res.Type != proto.TypeResult {
			t.Fatalf("%s: %+v %v", dir, res, err)
		}
		if dir == proto.DirFwd && res.Bytes != 8000 {
			t.Errorf("fwd: suma %d, esperado 8000", res.Bytes)
		}
		for _, dc := range data {
			dc.Close()
		}
		cc.Close()
		stop()
		if got := cl.n.Load(); got != 1+8+1 {
			t.Errorf("%s: aceptadas %d conexiones, esperado 10 (1 control + 8 datos + 1 rechazada)", dir, got)
		}
	}
}

func TestStreamFailsMidway(t *testing.T) {
	cl, stop := startCounting(t)
	defer stop()
	cc, codec, session, m := params(t, cl.Addr().String(), proto.DirFwd, 2)
	defer cc.Close()
	if m.Type != proto.TypeStart {
		t.Fatalf("%+v", m)
	}
	var data []*net.TCPConn
	for i := 0; i < 2; i++ {
		dc, err := net.Dial("tcp", cl.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		proto.NewCodec(dc).Write(proto.Message{Type: proto.TypeData, Session: session})
		data = append(data, dc.(*net.TCPConn))
	}
	data[0].Write(make([]byte, 5000))
	data[1].Write(make([]byte, 5000))
	time.Sleep(100 * time.Millisecond)
	data[1].SetLinger(0) // cierre con RST: el flujo falla a mitad
	data[1].Close()
	res, err := codec.Read()
	data[0].Close()
	if err != nil || res.Type != proto.TypeError || !strings.Contains(res.Error, "flujo") {
		t.Fatalf("esperaba error de flujo: %+v %v", res, err)
	}
}

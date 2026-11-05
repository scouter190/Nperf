package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/testutil"
	"nperf/internal/version"
)

// Un cliente que envía params y se va sin abrir conexiones de datos libera el turno al
// instante, sin esperar el plazo de la fase (duración + margen).
func TestTurnReleasedWhenClientLeavesBeforeData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, uc := testutil.Listen(t)
	go func() {
		testutil.ServeErr(t, ctx, server.New(server.WithUDPConn(uc)).Serve(ctx, ln))
	}()
	open := func() (net.Conn, *proto.Codec) {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(10 * time.Second))
		codec := proto.NewCodec(c)
		if _, err := proto.ClientHandshake(codec, "", version.Current); err != nil {
			t.Fatal(err)
		}
		if err := codec.Write(proto.Message{Type: proto.TypeTurn}); err != nil {
			t.Fatal(err)
		}
		return c, codec
	}
	a, ac := open()
	for { // hasta "ready"
		m, err := ac.Read()
		if err != nil {
			t.Fatal(err)
		}
		if m.Type == proto.TypeReady {
			break
		}
	}
	ac.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoTCP, Direction: proto.DirFwd, Streams: 1, DurationMS: 60000})
	for { // hasta "start"
		m, err := ac.Read()
		if err != nil {
			t.Fatal(err)
		}
		if m.Type == proto.TypeStart {
			break
		}
	}
	b, bc := open()
	defer b.Close()
	time.Sleep(100 * time.Millisecond)
	a.Close() // se va sin abrir el flujo de datos
	t0 := time.Now()
	for {
		m, err := bc.Read()
		if err != nil {
			t.Fatalf("b no obtuvo el turno: %v (tras %v)", err, time.Since(t0))
		}
		if m.Type == proto.TypeReady {
			break
		}
	}
}

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

// leaveMidMeasurement: A inicia una fase fwd larga, abre sus flujos de datos y se va a mitad
// de la medición; B (en cola) debe obtener el turno mucho antes de duración+margen.
func leaveMidMeasurement(t *testing.T, protoName string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, uc := testutil.Listen(t)
	srv := server.New(server.WithUDPConn(uc))
	go func() { testutil.ServeErr(t, ctx, srv.Serve(ctx, ln)) }()
	open := func() (net.Conn, *proto.Codec, string) {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(15 * time.Second))
		codec := proto.NewCodec(c)
		id, err := proto.ClientHandshake(codec, "", version.Current)
		if err != nil {
			t.Fatal(err)
		}
		if err := codec.Write(proto.Message{Type: proto.TypeTurn}); err != nil {
			t.Fatal(err)
		}
		return c, codec, id
	}
	waitFor := func(codec *proto.Codec, typ string) {
		for {
			m, err := codec.Read()
			if err != nil {
				t.Fatalf("esperando %s: %v", typ, err)
			}
			if m.Type == typ {
				return
			}
		}
	}
	a, ac, id := open()
	waitFor(ac, proto.TypeReady)
	ac.Write(proto.Message{Type: proto.TypeParams, Proto: protoName, Direction: proto.DirFwd, Streams: 1, DurationMS: 60000})
	waitFor(ac, proto.TypeStart)
	var data net.Conn
	if protoName == proto.ProtoTCP {
		var err error
		data, err = net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer data.Close()
		if err := proto.NewCodec(data).Write(proto.Message{Type: proto.TypeData, Session: id}); err != nil {
			t.Fatal(err)
		}
		for srv.SessionDataConns(id) < 1 { // el server ya tiene el flujo (medición en curso)
			time.Sleep(time.Millisecond)
		}
	}
	b, bc, _ := open()
	defer b.Close()
	a.Close() // se va a mitad de la medición
	t0 := time.Now()
	waitFor(bc, proto.TypeReady)
	if el := time.Since(t0); el > 10*time.Second {
		t.Fatalf("b tardó %v en obtener el turno", el)
	}
	if data != nil { // el server cierra el flujo de datos
		data.SetReadDeadline(time.Now().Add(10 * time.Second))
		buf := make([]byte, 1)
		if _, err := data.Read(buf); err == nil {
			t.Fatal("se esperaba el cierre del flujo de datos")
		} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
			t.Fatal("el flujo de datos no se cerró")
		}
	}
}

func TestTurnReleasedWhenClientLeavesMidTCPFwd(t *testing.T) { leaveMidMeasurement(t, proto.ProtoTCP) }
func TestTurnReleasedWhenClientLeavesMidUDPFwd(t *testing.T) { leaveMidMeasurement(t, proto.ProtoUDP) }

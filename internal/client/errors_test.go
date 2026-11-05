package client_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/testutil"
	"nperf/internal/version"
)

func start(t *testing.T) (string, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ln, uc := testutil.Listen(t)
	done := make(chan struct{})
	go func() {
		testutil.ServeErr(t, ctx, server.New(server.WithUDPConn(uc)).Serve(ctx, ln))
		close(done)
	}()
	return ln.Addr().String(), cancel, done
}

func TestSecondRun(t *testing.T) {
	addr, cancel, _ := start(t)
	defer cancel()
	for i := 0; i < 2; i++ {
		res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond})
		if err != nil || res.Fwd.Mbps <= 0 {
			t.Fatalf("prueba %d: %+v %v", i, res, err)
		}
	}
}

func TestUnreachable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if _, err := client.Run(context.Background(), addr, client.Options{Duration: time.Second}); err == nil {
		t.Fatal("se esperaba error")
	}
}

func TestUnsupportedMode(t *testing.T) {
	addr, cancel, _ := start(t)
	defer cancel()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	codec := proto.NewCodec(c)
	if _, err := proto.ClientHandshake(codec, "", version.Current); err != nil {
		t.Fatal(err)
	}
	if err := proto.ClientTurn(codec, nil); err != nil {
		t.Fatal(err)
	}
	codec.Write(proto.Message{Type: proto.TypeParams, Proto: "udp", Direction: "sideways"})
	m, err := codec.Read()
	if err != nil || m.Type != proto.TypeError {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	addr, cancel, done := start(t)
	c, err := net.Dial("tcp", addr) // conexión ociosa
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Serve no terminó al cancelar")
	}
}

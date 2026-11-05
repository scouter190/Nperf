package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/version"
)

// Un cliente autenticado que no manda params es desconectado al vencer el plazo.
func TestParamsTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.New(server.WithHandshakeTimeout(300*time.Millisecond)).Serve(ctx, ln)
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	codec := proto.NewCodec(c)
	if _, err := proto.ClientHandshake(codec, "", version.Current); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	t0 := time.Now()
	for { // hasta EOF (puede llegar antes un mensaje de error)
		if _, err := codec.Read(); err != nil {
			break
		}
	}
	if d := time.Since(t0); d > 3*time.Second {
		t.Fatalf("desconexión tardía: %v", d)
	}
}

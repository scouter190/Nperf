package client_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/server"
)

func TestLoopbackTCP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.New().Serve(ctx, ln)

	res, err := client.Run(ctx, ln.Addr().String(), client.Options{Duration: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mbps <= 0 || res.Received.Bytes <= 0 {
		t.Fatalf("throughput inválido: %+v", res)
	}
	t.Logf("%.0f Mbps", res.Mbps)
}

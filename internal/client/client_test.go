package client_test

import (
	"context"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/server"
	"nperf/internal/testutil"
)

func TestLoopbackTCP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, uc := testutil.Listen(t)
	go func() { testutil.ServeErr(t, ctx, server.New(server.WithUDPConn(uc)).Serve(ctx, ln)) }()

	res, err := client.Run(ctx, ln.Addr().String(), client.Options{Duration: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mbps <= 0 || res.Received.Bytes <= 0 {
		t.Fatalf("throughput inválido: %+v", res)
	}
	t.Logf("%.0f Mbps", res.Mbps)
}

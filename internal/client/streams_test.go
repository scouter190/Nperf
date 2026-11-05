package client_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
)

func TestStreamsRun(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	for _, dir := range []string{"fwd", "rev"} {
		res, err := client.Run(context.Background(), addr,
			client.Options{Duration: 400 * time.Millisecond, Direction: dir, Streams: 8})
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		d := res.Fwd
		if dir == "rev" {
			d = res.Rev
		}
		if !d.Measured || d.Mbps <= 0 || d.Received.Bytes != d.Sent.Bytes {
			t.Errorf("%s: %+v", dir, d)
		}
	}
}

func TestStreamsInvalidBeforeConnect(t *testing.T) {
	for _, n := range []int{-1, 65} {
		// Dirección inalcanzable: si validara después de conectar, el error sería otro.
		_, err := client.Run(context.Background(), "127.0.0.1:1", client.Options{Duration: time.Second, Streams: n})
		if err == nil || !strings.Contains(err.Error(), "-streams") {
			t.Errorf("streams=%d: %v", n, err)
		}
	}
	if client.ValidateStreams(0) == nil || client.ValidateStreams(1) != nil || client.ValidateStreams(64) != nil {
		t.Error("ValidateStreams")
	}
}

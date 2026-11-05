package meter

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func TestMbps(t *testing.T) {
	r := Result{Bytes: 125_000_000, Elapsed: time.Second}
	if got := r.Mbps(); got != 1000 {
		t.Fatalf("got %v", got)
	}
	if (Result{Bytes: 1}).Mbps() != 0 {
		t.Fatal("elapsed 0 debe dar 0")
	}
}

func TestReceiveTCP(t *testing.T) {
	r, err := ReceiveTCP(bytes.NewReader(make([]byte, 1000)))
	if err != nil || r.Bytes != 1000 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSendTCPShortDuration(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	go func() {
		buf := make([]byte, 64*1024)
		for {
			if _, err := b.Read(buf); err != nil {
				return
			}
		}
	}()
	res, err := SendTCP(context.Background(), a, 100*time.Millisecond)
	a.Close()
	if err != nil || res.Bytes <= 0 || res.Elapsed > 2*time.Second {
		t.Fatalf("%+v %v", res, err)
	}
}

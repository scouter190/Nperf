package meter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
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

type errReader struct{ n int }

func (e *errReader) Read(p []byte) (int, error) {
	if e.n > 0 {
		e.n--
		return len(p) / 2, nil
	}
	return 0, errors.New("conexión reiniciada")
}

func TestReceiveTCPNSum(t *testing.T) {
	var rs []io.Reader
	var want int64
	for i := 1; i <= 8; i++ {
		rs = append(rs, bytes.NewReader(make([]byte, i*1000)))
		want += int64(i * 1000)
	}
	r, err := ReceiveTCPN(rs, nil)
	if err != nil || r.Bytes != want {
		t.Fatalf("got %+v %v, want %d bytes", r, err, want)
	}
}

func TestReceiveTCPNFlowError(t *testing.T) {
	aborted := false
	_, err := ReceiveTCPN([]io.Reader{bytes.NewReader(make([]byte, 10)), &errReader{n: 2}}, func() { aborted = true })
	if err == nil || !aborted || !strings.Contains(err.Error(), "flujo 2 de 2") {
		t.Fatalf("err=%v aborted=%v", err, aborted)
	}
}

func TestSendTCPNSum(t *testing.T) {
	var conns []net.Conn
	var counts [3]atomic.Int64
	for i := 0; i < 3; i++ {
		a, b := net.Pipe()
		conns = append(conns, a)
		go func(i int) {
			buf := make([]byte, 64*1024)
			for {
				n, err := b.Read(buf)
				counts[i].Add(int64(n))
				if err != nil {
					return
				}
			}
		}(i)
	}
	res, err := SendTCPN(context.Background(), conns, 100*time.Millisecond)
	for _, c := range conns {
		c.Close()
	}
	if err != nil || res.Bytes <= 0 {
		t.Fatalf("%+v %v", res, err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := counts[0].Load() + counts[1].Load() + counts[2].Load(); got != res.Bytes {
		t.Fatalf("suma %d != %d", got, res.Bytes)
	}
}

func TestSendTCPNFlowError(t *testing.T) {
	a, b := net.Pipe()
	c, d := net.Pipe()
	go io.Copy(io.Discard, d)
	b.Close() // el flujo 1 falla al escribir
	_, err := SendTCPN(context.Background(), []net.Conn{a, c}, 2*time.Second)
	c.Close()
	if err == nil || !strings.Contains(err.Error(), "flujo 1 de 2") {
		t.Fatalf("err=%v", err)
	}
}

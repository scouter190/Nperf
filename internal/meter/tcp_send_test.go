package meter

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type partialConn struct {
	net.Conn
	calls int
}

func (p *partialConn) SetWriteDeadline(time.Time) error { return nil }
func (p *partialConn) Write(b []byte) (int, error) {
	p.calls++
	if p.calls < 3 {
		return len(b), nil
	}
	return 1000, errors.New("cierre")
}

func TestSendTCPCountsPartialWriteOnError(t *testing.T) {
	c := &partialConn{}
	r, err := SendTCP(context.Background(), c, time.Hour)
	if err == nil {
		t.Fatal("quería error")
	}
	if want := int64(2*128*1024 + 1000); r.Bytes != want {
		t.Fatalf("bytes %d, quería %d", r.Bytes, want)
	}
}

type blockConn struct{ net.Conn }

func (blockConn) SetWriteDeadline(time.Time) error { return nil }
func (blockConn) Write([]byte) (int, error)        { return 0, timeoutErr{} }

func TestSendTCPBlockedWriteIsError(t *testing.T) {
	_, err := SendTCP(context.Background(), blockConn{}, time.Hour)
	if err == nil {
		t.Fatal("quería error por envío bloqueado")
	}
}

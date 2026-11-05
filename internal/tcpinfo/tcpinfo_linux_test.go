//go:build linux && (amd64 || arm64)

package tcpinfo

import (
	"net"
	"testing"
)

func TestRetransmitsLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	<-done
	if _, err := Retransmits(c.(*net.TCPConn)); err != nil {
		t.Fatalf("TCP_INFO: %v", err)
	}
}

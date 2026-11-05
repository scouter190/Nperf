//go:build windows

package tcpinfo

import (
	"io"
	"net"
	"testing"
	"unsafe"
)

func TestTCPInfoLayout(t *testing.T) {
	if s := unsafe.Sizeof(tcpInfoV0{}); s != 88 {
		t.Fatalf("tamaño de TCP_INFO_v0 = %d, quería 88", s)
	}
}

// Estricto: el runner de Windows debe soportar SIO_TCP_INFO; cualquier error falla.
func TestRetransmitsLoopbackWindows(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			io.Copy(io.Discard, c)
			c.Close()
		}
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(make([]byte, 64*1024)); err != nil {
		t.Fatal(err)
	}
	if _, err := Retransmits(c.(*net.TCPConn)); err != nil {
		t.Fatalf("SIO_TCP_INFO falló: %v", err)
	}
}

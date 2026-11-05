package testutil

import (
	"net"
	"testing"
)

func TestListenSharesPort(t *testing.T) {
	for i := 0; i < 100; i++ {
		ln, uc := Listen(t)
		tp := ln.Addr().(*net.TCPAddr).Port
		up := uc.LocalAddr().(*net.UDPAddr).Port
		ln.Close()
		uc.Close()
		if tp != up {
			t.Fatalf("TCP %d != UDP %d", tp, up)
		}
	}
}

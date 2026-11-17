package probe

import (
	"context"
	"net"
	"testing"
)

// Criterio 4: Windows reintenta el SYN tras un RST; el puerto cerrado en
// loopback debe salir "cerrado" y no "filtrado" dentro del timeout.
func TestPortsClosedLoopbackWindows(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	res, err := Ports(context.Background(), &net.Dialer{}, loop, []int{port}, DefaultPortsTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].State != PortClosed {
		t.Fatalf("esperaba cerrado, obtuve %+v", res[0])
	}
}

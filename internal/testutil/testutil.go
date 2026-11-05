// Package testutil reúne ayudas de prueba compartidas.
package testutil

import (
	"context"
	"net"
	"testing"
)

// Listen abre un listener TCP y un socket UDP en el MISMO puerto, ya enlazados: se los pasa
// al server (server.WithUDPConn) para que no haya carrera entre comprobar y reabrir el puerto.
func Listen(t testing.TB) (net.Listener, *net.UDPConn) {
	t.Helper()
	var err error
	for i := 0; i < 50; i++ {
		var ln net.Listener
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			break
		}
		ta := ln.Addr().(*net.TCPAddr)
		var uc *net.UDPConn
		if uc, err = net.ListenUDP("udp", &net.UDPAddr{IP: ta.IP, Port: ta.Port}); err == nil {
			return ln, uc
		}
		ln.Close()
	}
	t.Fatal(err)
	return nil, nil
}

// ServeErr falla la prueba si Serve terminó con error sin que se haya cancelado ctx.
func ServeErr(t testing.TB, ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		t.Errorf("Serve: %v", err)
	}
}

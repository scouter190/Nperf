// Package testutil reúne ayudas de prueba compartidas.
package testutil

import (
	"context"
	"net"
	"strconv"
	"testing"
)

// Listen abre un socket UDP y un listener TCP en el MISMO puerto, ya enlazados: se los pasa
// al server (server.WithUDPConn) para que no haya carrera entre comprobar y reabrir el puerto.
// Se abre primero el UDP: Windows reserva bloques de puertos para UDP (excluded port ranges,
// p. ej. Hyper-V/WinNAT) y los puertos TCP efímeros consecutivos caen en el mismo bloque, así
// que partir de TCP daba WSAEACCES en los 50 intentos. El UDP en el puerto 0 recibe del SO un
// puerto permitido; si el TCP en ese número falla, se reintenta con otro (hasta 50 veces).
func Listen(t testing.TB) (net.Listener, *net.UDPConn) {
	t.Helper()
	var udpErr, tcpErr error
	lastPort := 0
	for i := 0; i < 50; i++ {
		uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			udpErr = err
			continue
		}
		port := uc.LocalAddr().(*net.UDPAddr).Port
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return ln, uc
		}
		tcpErr, lastPort = err, port
		uc.Close()
	}
	t.Fatalf("no se pudo abrir UDP y TCP en el mismo puerto tras 50 intentos (último puerto %d): UDP: %v; TCP: %v", lastPort, udpErr, tcpErr)
	return nil, nil
}

// ServeErr falla la prueba si Serve terminó con error sin que se haya cancelado ctx.
func ServeErr(t testing.TB, ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		t.Errorf("Serve: %v", err)
	}
}

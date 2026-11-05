// Package testutil reúne ayudas de prueba compartidas.
package testutil

import (
	"context"
	"math/rand"
	"net"
	"strconv"
	"testing"
)

// Listen abre un socket UDP y un listener TCP en el MISMO puerto, ya enlazados: se los pasa
// al server (server.WithUDPConn) para que no haya carrera entre comprobar y reabrir el puerto.
// Los puertos son aleatorios en 20000-40000, fuera del rango dinámico de Windows donde suelen
// estar las reservas (excluded port ranges de Hyper-V/WinNAT); se reintenta hasta 200 veces.
func Listen(t testing.TB) (net.Listener, *net.UDPConn) {
	t.Helper()
	var udpErr, tcpErr error
	lastPort := 0
	for i := 0; i < 200; i++ {
		port := 20000 + rand.Intn(20001)
		uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			udpErr, lastPort = err, port
			continue
		}
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return ln, uc
		}
		tcpErr, lastPort = err, port
		uc.Close()
	}
	t.Fatalf("no se pudo abrir UDP y TCP en el mismo puerto tras 200 intentos (último puerto %d): UDP: %v; TCP: %v", lastPort, udpErr, tcpErr)
	return nil, nil
}

// ServeErr falla la prueba si Serve terminó con error sin que se haya cancelado ctx.
func ServeErr(t testing.TB, ctx context.Context, err error) {
	if err != nil && ctx.Err() == nil {
		t.Errorf("Serve: %v", err)
	}
}

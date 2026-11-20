package client

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGreetNoAck(t *testing.T) {
	greetTries, greetWait, greetTotal = 3, 20*time.Millisecond, 100*time.Millisecond
	defer func() { greetTries, greetWait, greetTotal = 16, 300*time.Millisecond, 5*time.Second }()
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	uc, err := net.Dial("udp", sink.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	err = greetUDP(context.Background(), uc, "x")
	if err == nil || !strings.Contains(err.Error(), "no respondió") {
		t.Fatalf("error = %v", err)
	}
}

// Regresión T1 (spec 007). Causa confirmada por lectura del código: greetUDP mide el presupuesto
// en intentos (greetTries=10 x greetWait=300 ms = 3 s), no en tiempo (H1), y cada lectura que no
// es el acuse consume un intento y reenvía el saludo al instante sin esperar (H2). Con CPU
// saturada (-race, UDP de varios Gbps en loopback) el despachador del servidor tarda >3 s en
// responder, o los datagramas de la fase anterior agotan los intentos en milisegundos.
// Estos tests usan los valores reales de greetTries/greetWait (no usar t.Parallel: son globales).

// fakeGreetServer lee saludos en un socket UDP de loopback y ejecuta onHello(cliente, conn) en la
// primera recepción. Devuelve la dirección y el contador de saludos recibidos.
func fakeGreetServer(t *testing.T, onHello func(c net.Addr, s *net.UDPConn)) (string, *atomic.Int32) {
	t.Helper()
	s, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var hellos atomic.Int32
	go func() {
		buf := make([]byte, 128)
		for {
			_, a, err := s.ReadFrom(buf)
			if err != nil {
				return
			}
			if hellos.Add(1) == 1 && onHello != nil {
				go onHello(a, s)
			}
		}
	}()
	return s.LocalAddr().String(), &hellos
}

func dialGreet(t *testing.T, addr string) net.Conn {
	t.Helper()
	uc, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { uc.Close() })
	return uc
}

// Regresión corregida en T2.
func TestGreetLateAck(t *testing.T) {
	addr, _ := fakeGreetServer(t, func(c net.Addr, s *net.UDPConn) {
		time.Sleep(3500 * time.Millisecond)
		s.WriteTo([]byte(udpMagic+"ok"), c)
	})
	if err := greetUDP(context.Background(), dialGreet(t, addr), "x"); err != nil {
		t.Fatalf("el acuse a los 3,5 s debe aceptarse: %v", err)
	}
}

// Regresión corregida en T2.
func TestGreetGarbageBeforeAck(t *testing.T) {
	addr, _ := fakeGreetServer(t, func(c net.Addr, s *net.UDPConn) {
		for i := 0; i < 50; i++ {
			s.WriteTo([]byte("basura"), c)
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond)
		s.WriteTo([]byte(udpMagic+"ok"), c)
	})
	if err := greetUDP(context.Background(), dialGreet(t, addr), "x"); err != nil {
		t.Fatalf("la basura antes del acuse no debe agotar la espera: %v", err)
	}
}

// Puerto mudo: falla con el mensaje conocido y en tiempo acotado (pasa hoy y tras T2).
func TestGreetMutePortBounded(t *testing.T) {
	addr, hellos := fakeGreetServer(t, nil)
	start := time.Now()
	err := greetUDP(context.Background(), dialGreet(t, addr), "x")
	if err == nil || !strings.Contains(err.Error(), "no respondió al saludo UDP") {
		t.Fatalf("error = %v", err)
	}
	if d := time.Since(start); d > 6*time.Second {
		t.Fatalf("tardó %v (>6 s)", d)
	}
	if hellos.Load() == 0 {
		t.Fatal("no se envió ningún saludo")
	}
}

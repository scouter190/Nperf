package client_test

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
)

func TestStatusAvailableWithoutTests(t *testing.T) {
	addr := startWith(t)
	r, err := client.Status(context.Background(), addr, client.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.State != proto.StateAvailable || r.Queue != 0 {
		t.Fatalf("respuesta %+v", r)
	}
}

func TestStatusBusyWithQueueKeepsQueueIntact(t *testing.T) {
	e := newQEnv(t)
	first := e.launch(quick)
	waitCh(t, e.entered, "turno del 1.º")
	var q []*qClient
	for i := 1; i <= 2; i++ { // uno a uno: fija el orden de llegada
		c := e.launch(quick)
		if p := waitCh(t, c.posCh, "posición"); p != i {
			t.Fatalf("posición %d, quería %d", p, i)
		}
		q = append(q, c)
	}
	for i := 0; i < 5; i++ {
		start := time.Now()
		r, err := client.Status(context.Background(), e.addr, client.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("respuesta en %v (> 1 s)", d)
		}
		if r.State != proto.StateBusy || r.Queue != 2 {
			t.Fatalf("respuesta %+v", r)
		}
	}
	// La consulta no ocupa turno ni cambia posiciones: nadie recibió avisos nuevos.
	for i, c := range q {
		if p := c.positions(); len(p) != 1 || p[0] != i+1 {
			t.Errorf("cliente en cola %d: posiciones %v", i+1, p)
		}
	}
	select {
	case <-e.entered:
		t.Fatal("la consulta provocó un turno nuevo")
	default:
	}
	// Liberan en orden y todos terminan bien.
	e.free <- struct{}{}
	waitCh(t, first.done, "fin del 1.º")
	for _, c := range q {
		waitCh(t, e.entered, "turno de la cola")
		e.free <- struct{}{}
		waitCh(t, c.done, "fin del cliente en cola")
		if c.err != nil {
			t.Fatal(c.err)
		}
	}
}

func TestStatusRawReplyAndNoFurtherAccess(t *testing.T) {
	e := newQEnv(t)
	first := e.launch(quick)
	waitCh(t, e.entered, "turno del 1.º")
	defer func() { e.free <- struct{}{}; waitCh(t, first.done, "fin del 1.º") }()

	c, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(`{"type":"status"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 256)
	one := make([]byte, 1)
	for {
		if _, err := c.Read(one); err != nil {
			t.Fatal(err)
		}
		if one[0] == '\n' {
			break
		}
		buf = append(buf, one[0])
	}
	var m map[string]any
	if err := json.Unmarshal(buf, &m); err != nil {
		t.Fatalf("respuesta no es JSON: %s", buf)
	}
	if len(m) != 2 || m["state"] != "busy" || m["queue"] != float64(0) {
		t.Fatalf("la respuesta cruda debe traer solo state y queue: %s", buf)
	}
	// Tras responder, la conexión se cierra y no admite hello/params.
	c.Write([]byte(`{"type":"hello","version":"1.0.0"}` + "\n"))
	c.Write([]byte(`{"type":"params","proto":"tcp","direction":"fwd","duration_ms":1000}` + "\n"))
	if n, err := c.Read(one); err == nil {
		t.Fatalf("la conexión siguió activa tras status (%d bytes)", n)
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("la conexión no se cerró tras status")
	}
}

func TestStatusOldServerWithoutSupport(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { // servidor viejo: ignora lo que no es hello y cierra
				buf := make([]byte, 64)
				c.Read(buf)
				c.Close()
			}()
		}
	}()
	_, err = client.Status(context.Background(), ln.Addr().String(), client.Options{})
	if err == nil || !strings.Contains(err.Error(), "no admite la consulta de estado") {
		t.Fatalf("error = %v", err)
	}
}

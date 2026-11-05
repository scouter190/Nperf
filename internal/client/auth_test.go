package client_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
	"nperf/internal/server"
)

func startWith(t *testing.T, opts ...server.Option) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { server.New(opts...).Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String()
}

type logSink struct {
	mu sync.Mutex
	b  []string
}

func (l *logSink) add(m string) { l.mu.Lock(); l.b = append(l.b, m); l.mu.Unlock() }
func (l *logSink) all() string  { l.mu.Lock(); defer l.mu.Unlock(); return strings.Join(l.b, "\n") }

func TestKeyOK(t *testing.T) {
	addr := startWith(t, server.WithKey("secreto"))
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond, Key: "secreto"})
	if err != nil || res.Mbps <= 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestWrongKeyNoLoad(t *testing.T) {
	for _, p := range []string{"tcp", "udp"} {
		var log logSink
		addr := startWith(t, server.WithKey("secreto"), server.WithLog(log.add))
		res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond, Key: "otra", Proto: p})
		if err == nil || !strings.Contains(err.Error(), "clave incorrecta") {
			t.Fatalf("%s: error = %v", p, err)
		}
		if res.Sent.Bytes != 0 || res.Mbps != 0 {
			t.Errorf("%s: hubo carga: %+v", p, res)
		}
		if !strings.Contains(log.all(), "clave incorrecta") {
			t.Errorf("%s: el servidor no registró el fallo: %q", p, log.all())
		}
	}
}

// Servidor con clave y cliente sin clave (y al revés) también falla.
func TestEmptyKeyMismatch(t *testing.T) {
	addr := startWith(t, server.WithKey("secreto"))
	if _, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond}); err == nil ||
		!strings.Contains(err.Error(), "clave incorrecta") {
		t.Fatalf("error = %v", err)
	}
	addr = startWith(t)
	if _, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond, Key: "x"}); err == nil ||
		!strings.Contains(err.Error(), "clave incorrecta") {
		t.Fatalf("error = %v", err)
	}
}

// Un servidor que no conoce la clave no puede hacerse pasar por el real.
func TestClientRejectsServerWithoutKey(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		codec := proto.NewCodec(c)
		h, _ := codec.Read()
		codec.Write(proto.Message{Type: proto.TypeHello, Version: h.Version, Nonce: "abc"})
		codec.Read()
		codec.Write(proto.Message{Type: proto.TypeAuth, MAC: "falso", Session: "s"})
		io.Copy(io.Discard, c)
	}()
	_, err := client.Run(context.Background(), ln.Addr().String(), client.Options{Duration: 200 * time.Millisecond, Key: "secreto"})
	if err == nil || !strings.Contains(err.Error(), "clave incorrecta") {
		t.Fatalf("error = %v", err)
	}
}

// El relay captura los bytes del canal de control; la clave no debe aparecer.
func TestKeyNeverOnWire(t *testing.T) {
	const key = "clave-super-secreta-12345"
	addr := startWith(t, server.WithKey(key))
	rl, _ := net.Listen("tcp", "127.0.0.1:0")
	defer rl.Close()
	var mu sync.Mutex
	var captured bytes.Buffer
	go func() {
		for {
			c, err := rl.Accept()
			if err != nil {
				return
			}
			s, err := net.Dial("tcp", addr)
			if err != nil {
				c.Close()
				return
			}
			w := &lockedWriter{mu: &mu, b: &captured}
			go func() { io.Copy(io.MultiWriter(s, w), c); s.Close() }()
			go func() { io.Copy(io.MultiWriter(c, w), s); c.Close() }()
		}
	}()
	res, err := client.Run(context.Background(), rl.Addr().String(), client.Options{Duration: 200 * time.Millisecond, Key: key})
	if err != nil || res.Mbps <= 0 {
		t.Fatalf("%+v %v", res, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.Len() == 0 || !bytes.Contains(captured.Bytes(), []byte(`"type":"auth"`)) {
		t.Fatalf("el relay no capturó el handshake (%d bytes)", captured.Len())
	}
	if bytes.Contains(captured.Bytes(), []byte(key)) {
		t.Fatal("la clave apareció en el canal de control")
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func TestMajorVersionMismatch(t *testing.T) {
	for _, p := range []string{"tcp", "udp"} {
		var log logSink
		addr := startWith(t, server.WithVersion("2.0.0"), server.WithLog(log.add))
		res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond, Proto: p, Version: "1.4.0"})
		if err == nil || !strings.Contains(err.Error(), "versión incompatible") {
			t.Fatalf("%s: cliente: %v", p, err)
		}
		if res.Sent.Bytes != 0 || res.Mbps != 0 {
			t.Errorf("%s: se midió: %+v", p, res)
		}
		if !strings.Contains(log.all(), "versión incompatible") {
			t.Errorf("%s: sin aviso en el servidor: %q", p, log.all())
		}
	}
}

// Un servidor viejo que no valida versión: el cliente avisa por su cuenta.
func TestClientDetectsIncompatibleServer(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		codec := proto.NewCodec(c)
		codec.Read()
		codec.Write(proto.Message{Type: proto.TypeHello, Version: "3.0.0", Nonce: "n"})
		io.Copy(io.Discard, c)
	}()
	_, err := client.Run(context.Background(), ln.Addr().String(), client.Options{Duration: 200 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "versión incompatible") {
		t.Fatalf("error = %v", err)
	}
}

func TestMinorVersionCompatible(t *testing.T) {
	addr := startWith(t, server.WithVersion("0.9.5"))
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond, Version: "0.1.0"})
	if err != nil || res.Mbps <= 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

// Una conexión de datos con id no emitido por un handshake válido se rechaza.
func TestDataConnUnauthenticatedRejected(t *testing.T) {
	addr := startWith(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	proto.NewCodec(c).Write(proto.Message{Type: proto.TypeData, Session: "0123456789abcdef"})
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("la conexión debía cerrarse (EOF), err = %v", err)
	}
	// Tampoco sirve el id de un handshake a medias (sin auth no hay sesión).
	c2, _ := net.Dial("tcp", addr)
	defer c2.Close()
	codec := proto.NewCodec(c2)
	codec.Write(proto.Message{Type: proto.TypeHello, Version: "0.1.0"})
	h, err := codec.Read()
	if err != nil {
		t.Fatal(err)
	}
	c3, _ := net.Dial("tcp", addr)
	defer c3.Close()
	proto.NewCodec(c3).Write(proto.Message{Type: proto.TypeData, Session: h.Nonce})
	c3.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c3.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("err = %v", err)
	}
}

// Un cliente que envía hello y se calla no retiene la conexión más allá del plazo.
func TestHandshakeTimeout(t *testing.T) {
	addr := startWith(t, server.WithHandshakeTimeout(300*time.Millisecond))
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	codec := proto.NewCodec(c)
	codec.Write(proto.Message{Type: proto.TypeHello, Version: "0.1.0"})
	if _, err := codec.Read(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("el servidor debía cerrar (EOF), err = %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cierre tardío: %v", time.Since(start))
	}
}

// Package server atiende el canal de control y las conexiones de datos.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

const Version = "0.1.0"

type Server struct {
	mu       sync.Mutex
	sessions map[string]chan dataConn
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
	udp      udpState
}

// margin es la holgura sobre la duración antes de abortar una lectura.
const margin = 10 * time.Second

type dataConn struct {
	r    io.Reader
	conn net.Conn
	done chan struct{}
}

func New() *Server {
	return &Server{sessions: map[string]chan dataConn{}, conns: map[net.Conn]struct{}{},
		udp: udpState{byID: map[string]*udpSession{}, byAddr: map[netip.AddrPort]*udpSession{}}}
}

// Serve acepta conexiones hasta que ctx se cancele; al volver, todas las
// conexiones están cerradas y sus goroutines terminadas.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	stop := context.AfterFunc(ctx, func() {
		ln.Close()
		s.mu.Lock()
		for c := range s.conns {
			c.Close()
		}
		s.mu.Unlock()
	})
	defer stop()
	defer s.wg.Wait()
	if err := s.startUDP(ctx, ln); err != nil {
		ln.Close()
		return err
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		s.mu.Lock()
		if ctx.Err() != nil {
			s.mu.Unlock()
			c.Close()
			continue
		}
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { s.mu.Lock(); delete(s.conns, c); s.mu.Unlock() }()
			s.handle(ctx, c)
		}()
	}
}

func (s *Server) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	codec := proto.NewCodec(c)
	first, err := codec.Read()
	if err != nil {
		return
	}
	c.SetReadDeadline(time.Time{})
	switch first.Type {
	case proto.TypeHello:
		s.control(ctx, c, codec)
	case proto.TypeData:
		s.mu.Lock()
		ch := s.sessions[first.Session]
		s.mu.Unlock()
		if ch == nil {
			return
		}
		done := make(chan struct{})
		select {
		case ch <- dataConn{r: codec.Reader(), conn: c, done: done}:
			select { // el canal de control mide y luego libera la conexión
			case <-done:
			case <-ctx.Done():
			}
		default:
		}
	}
}

func (s *Server) control(ctx context.Context, c net.Conn, codec *proto.Codec) {
	fail := func(msg string) { codec.Write(proto.Message{Type: proto.TypeError, Error: msg}) }
	id := newID()
	ch := make(chan dataConn, 1)
	s.mu.Lock()
	s.sessions[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.sessions, id); s.mu.Unlock() }()

	if err := codec.Write(proto.Message{Type: proto.TypeHello, Version: Version, Session: id}); err != nil {
		return
	}
	p, err := codec.Read()
	if err != nil || p.Type != proto.TypeParams {
		fail("se esperaban parámetros")
		return
	}
	if p.Proto == proto.ProtoUDP && p.Direction == proto.DirFwd {
		s.controlUDP(ctx, codec, id, p)
		return
	}
	if p.Proto != proto.ProtoTCP || p.Direction != proto.DirFwd {
		fail(fmt.Sprintf("modo no soportado: %s/%s", p.Proto, p.Direction))
		return
	}
	if err := codec.Write(proto.Message{Type: proto.TypeStart}); err != nil {
		return
	}
	d := time.Duration(p.DurationMS) * time.Millisecond
	select {
	case dc := <-ch:
		dc.conn.SetReadDeadline(time.Now().Add(d + margin))
		res, err := meter.ReceiveTCP(dc.r)
		close(dc.done)
		if err != nil {
			fail("error de medición: " + err.Error())
			return
		}
		codec.Write(proto.Message{Type: proto.TypeResult, Bytes: res.Bytes,
			DurationMS: res.Elapsed.Milliseconds(), Mbps: res.Mbps()})
	case <-time.After(d + margin):
		fail("no llegó la conexión de datos")
	case <-ctx.Done():
	}
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

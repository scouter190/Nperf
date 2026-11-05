// Package server atiende el canal de control y las conexiones de datos.
package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
	"nperf/internal/version"
)

type Server struct {
	mu       sync.Mutex
	sessions map[string]*session
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
	udp      udpState
	key      string
	ver      string
	logf     func(string)
	hsWait   time.Duration
}

// Option configura el servidor.
type Option func(*Server)

// WithKey fija la clave compartida (vacía = sin clave; el cliente debe usar también vacía).
func WithKey(k string) Option { return func(s *Server) { s.key = k } }

// WithLog fija dónde se registran avisos (versión incompatible, clave incorrecta).
func WithLog(f func(string)) Option { return func(s *Server) { s.logf = f } }

// WithHandshakeTimeout fija el plazo total del handshake (por defecto 10 s).
func WithHandshakeTimeout(d time.Duration) Option { return func(s *Server) { s.hsWait = d } }

// WithVersion reemplaza la versión anunciada (para pruebas).
func WithVersion(v string) Option { return func(s *Server) { s.ver = v } }

// margin es la holgura sobre la duración antes de abortar una lectura.
const margin = 10 * time.Second

// session liga las conexiones de datos a una sesión autenticada; limit es el número de
// flujos negociado (0 hasta que llegan los params: las conexiones antes de eso se rechazan).
type session struct {
	ch    chan dataConn
	limit atomic.Int64
	n     atomic.Int64
}

type dataConn struct {
	r    io.Reader
	conn net.Conn
	done chan struct{}
}

func New(opts ...Option) *Server {
	s := &Server{ver: version.Current, hsWait: 10 * time.Second, logf: func(string) {}, sessions: map[string]*session{}, conns: map[net.Conn]struct{}{},
		udp: udpState{byID: map[string]*udpSession{}, byAddr: map[netip.AddrPort]*udpSession{}}}
	for _, o := range opts {
		o(s)
	}
	return s
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
	c.SetReadDeadline(time.Now().Add(s.hsWait)) // cubre hello y todo el handshake
	codec := proto.NewCodec(c)
	first, err := codec.Read()
	if err != nil {
		return
	}
	switch first.Type {
	case proto.TypeHello:
		s.control(ctx, c, codec, first)
	case proto.TypeData:
		c.SetReadDeadline(time.Time{})
		s.mu.Lock()
		ss := s.sessions[first.Session]
		s.mu.Unlock()
		if ss == nil || ss.n.Add(1) > ss.limit.Load() {
			return // sesión desconocida o más conexiones de las negociadas
		}
		ch := ss.ch
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

func (s *Server) control(ctx context.Context, c net.Conn, codec *proto.Codec, first proto.Message) {
	fail := func(msg string) { codec.Write(proto.Message{Type: proto.TypeError, Error: msg}) }
	// El id de sesión solo se emite tras autenticar; hasta entonces no existe en s.sessions.
	id, err := proto.ServerHandshake(codec, first, s.key, s.ver)
	if err != nil {
		s.logf("aviso: " + err.Error() + " (" + c.RemoteAddr().String() + ")")
		return
	}
	c.SetReadDeadline(time.Time{}) // handshake terminado
	ss := &session{ch: make(chan dataConn, proto.MaxStreams)}
	s.mu.Lock()
	s.sessions[id] = ss
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.sessions, id); s.mu.Unlock() }()

	p, err := codec.Read()
	if err != nil || p.Type != proto.TypeParams {
		fail("se esperaban parámetros")
		return
	}
	if p.Direction != proto.DirFwd && p.Direction != proto.DirRev {
		fail(fmt.Sprintf("modo no soportado: %s/%s", p.Proto, p.Direction))
		return
	}
	if p.Proto == proto.ProtoUDP {
		s.controlUDP(ctx, codec, id, p)
		return
	}
	if p.Proto != proto.ProtoTCP {
		fail(fmt.Sprintf("modo no soportado: %s/%s", p.Proto, p.Direction))
		return
	}
	streams := p.Streams
	if streams == 0 {
		streams = 1
	}
	if streams < 1 || streams > proto.MaxStreams {
		fail(fmt.Sprintf("número de flujos fuera de rango: %d (permitido 1-%d)", p.Streams, proto.MaxStreams))
		return
	}
	ss.limit.Store(int64(streams))
	if err := codec.Write(proto.Message{Type: proto.TypeStart}); err != nil {
		return
	}
	d := time.Duration(p.DurationMS) * time.Millisecond
	timeout := time.After(d + margin)
	dcs := make([]dataConn, 0, streams)
	release := func() {
		for _, dc := range dcs {
			close(dc.done)
		}
	}
	for len(dcs) < streams {
		select {
		case dc := <-ss.ch:
			dcs = append(dcs, dc)
		case <-timeout:
			release()
			fail(fmt.Sprintf("llegaron %d de %d conexiones de datos", len(dcs), streams))
			return
		case <-ctx.Done():
			release()
			return
		}
	}
	defer release()
	conns := make([]net.Conn, len(dcs))
	for i, dc := range dcs {
		conns[i] = dc.conn
	}
	if p.Direction == proto.DirRev {
		sctx, stopWatch := s.watchControl(ctx, codec)
		res, err := meter.SendTCPN(sctx, conns, d)
		cancelled := sctx.Err() != nil
		stopWatch()
		if cancelled {
			return // servidor cancelado o cliente desconectado
		}
		if err != nil {
			fail("error de envío: " + err.Error())
			return
		}
		codec.Write(proto.Message{Type: proto.TypeResult, Bytes: res.Bytes,
			DurationMS: res.Elapsed.Milliseconds(), Mbps: res.Mbps()})
		return
	}
	rs := make([]io.Reader, len(dcs))
	for i, dc := range dcs {
		dc.conn.SetReadDeadline(time.Now().Add(d + margin))
		rs[i] = dc.r
	}
	res, err := meter.ReceiveTCPN(rs, func() {
		for _, c := range conns {
			c.Close()
		}
	})
	if err != nil {
		fail("error de medición: " + err.Error())
		return
	}
	codec.Write(proto.Message{Type: proto.TypeResult, Bytes: res.Bytes,
		DurationMS: res.Elapsed.Milliseconds(), Mbps: res.Mbps()})
}

// watchControl devuelve un contexto que se cancela si ctx se cancela o el cliente cierra el
// canal de control (en rev el cliente no envía nada más tras params). stop libera el contexto.
func (s *Server) watchControl(ctx context.Context, codec *proto.Codec) (context.Context, func()) {
	sctx, cancel := context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		codec.Read() // vuelve al cerrarse la conexión (handle la cierra al terminar)
		cancel()
	}()
	return sctx, cancel
}

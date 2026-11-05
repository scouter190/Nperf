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
	udpConn  *net.UDPConn // opcional: socket UDP preabierto (WithUDPConn)
	mu       sync.Mutex
	sessions map[string]*session
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
	udp      udpState
	key      string
	ver      string
	logf     func(string)
	hsWait   time.Duration
	q        turnQueue
	turnHook func(context.Context)
}

// Option configura el servidor.
type Option func(*Server)

// WithKey fija la clave compartida (vacía = sin clave; el cliente debe usar también vacía).
func WithKey(k string) Option { return func(s *Server) { s.key = k } }

// WithLog fija dónde se registran avisos (versión incompatible, clave incorrecta).
func WithLog(f func(string)) Option { return func(s *Server) { s.logf = f } }

// WithUDPConn entrega al server un socket UDP ya abierto (mismo puerto que el listener TCP)
// en lugar de abrirlo en Serve. Sin esta opción, Serve lo abre por su cuenta.
func WithUDPConn(uc *net.UDPConn) Option { return func(s *Server) { s.udpConn = uc } }

// WithHandshakeTimeout fija el plazo total del handshake (por defecto 10 s).
func WithHandshakeTimeout(d time.Duration) Option { return func(s *Server) { s.hsWait = d } }

// WithMaxPerIP fija cuántas pruebas (en curso + en cola) admite una misma IP (0 = sin límite;
// por defecto 0). Evita que un cliente autenticado acapare la cola.
func WithMaxPerIP(n int) Option { return func(s *Server) { s.q.maxPerIP = n } }

// WithTurnHook registra una función que se ejecuta al concederse el turno, antes de "ready"
// (sirve a las pruebas para simular sesiones lentas de forma determinista).
func WithTurnHook(f func(context.Context)) Option { return func(s *Server) { s.turnHook = f } }

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
	s := &Server{ver: version.Current, hsWait: 10 * time.Second, logf: func(string) {}, q: turnQueue{maxWait: MaxQueue, perIP: map[string]int{}}, sessions: map[string]*session{}, conns: map[net.Conn]struct{}{},
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

// drainWait acota el drenado tras un cierre del canal de control.
const drainWait = time.Second

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
	// Robustez ante RST con datos pendientes (no es el arreglo de ningún test).
	// Cierre ordenado: half-close y drenado breve para que el SO envíe FIN y no RST
	// (RST por datos sin leer haría perder al cliente el mensaje de error).
	defer func() {
		if ctx.Err() != nil {
			return
		}
		if tc, ok := c.(interface{ CloseWrite() error }); ok {
			tc.CloseWrite()
		}
		c.SetReadDeadline(time.Now().Add(drainWait))
		io.Copy(io.Discard, io.LimitReader(c, 1<<20))
	}()
	// El id de sesión solo se emite tras autenticar; hasta entonces no existe en s.sessions.
	id, err := proto.ServerHandshake(codec, first, s.key, s.ver)
	if err != nil {
		s.logf("aviso: " + err.Error() + " (" + c.RemoteAddr().String() + ")")
		return
	}
	c.SetReadDeadline(time.Now().Add(s.hsWait)) // plazo de la solicitud de turno
	if m, err := codec.Read(); err != nil || m.Type != proto.TypeTurn {
		fail("se esperaba la solicitud de turno")
		return
	}
	// Solo clientes autenticados llegan a la cola. El turno se retiene hasta que termina la
	// sesión (todas sus fases, p. ej. fwd y rev) y se libera siempre al salir.
	release, err := s.acquireTurn(ctx, c, codec)
	if err != nil {
		return
	}
	defer release()
	ss := &session{ch: make(chan dataConn, proto.MaxStreams)}
	s.mu.Lock()
	s.sessions[id] = ss
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.sessions, id); s.mu.Unlock() }()

	p, err := codec.Read()
	// Ping/pong antes de params: mide el RTT del canal de control sin carga.
	for pings := 0; err == nil && p.Type == proto.TypePing && pings < proto.MaxPings; pings++ {
		if err = codec.Write(proto.Message{Type: proto.TypePong}); err == nil {
			p, err = codec.Read()
		}
	}
	// Una sesión es una secuencia de fases (params -> start -> result) con el turno retenido.
	for {
		if err != nil || p.Type != proto.TypeParams {
			fail("se esperaban parámetros")
			return
		}
		c.SetReadDeadline(time.Time{}) // params recibido
		if !s.phase(ctx, c, codec, id, ss, p) {
			return
		}
		c.SetReadDeadline(time.Now().Add(s.hsWait)) // siguiente fase o cierre del cliente
		if p, err = codec.Read(); err != nil {
			return // EOF: el cliente terminó la sesión
		}
	}
}

// phase mide una fase; devuelve true si terminó bien y la sesión puede seguir.
func (s *Server) phase(ctx context.Context, c net.Conn, codec *proto.Codec, id string, ss *session, p proto.Message) bool {
	fail := func(msg string) { codec.Write(proto.Message{Type: proto.TypeError, Error: msg}) }
	if p.Direction != proto.DirFwd && p.Direction != proto.DirRev {
		fail(fmt.Sprintf("modo no soportado: %s/%s", p.Proto, p.Direction))
		return false
	}
	if p.Proto == proto.ProtoUDP {
		if p.Direction == proto.DirRev && p.RateBps > MaxUDPRate {
			fail("tasa UDP demasiado alta: el máximo aceptado es 10 Gbps")
			return false
		}
		return s.controlUDP(ctx, c, codec, id, p)
	}
	if p.Proto != proto.ProtoTCP {
		fail(fmt.Sprintf("modo no soportado: %s/%s", p.Proto, p.Direction))
		return false
	}
	streams := p.Streams
	if streams == 0 {
		streams = 1
	}
	if streams < 1 || streams > proto.MaxStreams {
		fail(fmt.Sprintf("número de flujos fuera de rango: %d (permitido 1-%d)", p.Streams, proto.MaxStreams))
		return false
	}
	ss.n.Store(0)
	ss.limit.Store(int64(streams))
	defer ss.limit.Store(0) // entre fases no se aceptan conexiones de datos
	if err := codec.Write(proto.Message{Type: proto.TypeStart}); err != nil {
		return false
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
			return false
		case <-ctx.Done():
			release()
			return false
		}
	}
	defer release()
	conns := make([]net.Conn, len(dcs))
	for i, dc := range dcs {
		conns[i] = dc.conn
	}
	if p.Direction == proto.DirRev {
		sctx, stopWatch := s.watchControl(ctx, c, codec)
		res, err := meter.SendTCPN(sctx, conns, d)
		cancelled := sctx.Err() != nil
		stopWatch() // antes de responder: el lector no debe robar la siguiente fase
		if cancelled {
			return false // servidor cancelado o cliente desconectado
		}
		if err != nil {
			fail("error de envío: " + err.Error())
			return false
		}
		msg := proto.Message{Type: proto.TypeResult, Bytes: res.Bytes,
			DurationMS: res.Elapsed.Milliseconds(), Mbps: res.Mbps()}
		if res.RetransOK {
			msg.Retrans = &res.Retrans
			msg.RetransEst = res.RetransEstimated
		}
		return codec.Write(msg) == nil
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
		return false
	}
	return codec.Write(proto.Message{Type: proto.TypeResult, Bytes: res.Bytes,
		DurationMS: res.Elapsed.Milliseconds(), Mbps: res.Mbps()}) == nil
}

// watchControl devuelve un contexto que se cancela si ctx se cancela o el cliente cierra el
// canal de control (en rev el cliente no envía nada más tras params). stop (idempotente) libera
// el contexto, detiene el lector sin consumir mensajes y deja la conexión sin plazo de lectura.
func (s *Server) watchControl(ctx context.Context, c net.Conn, codec *proto.Codec) (context.Context, func()) {
	sctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(done)
		codec.Read() // vuelve al cerrarse la conexión o por el plazo que pone stop
		cancel()
	}()
	var once sync.Once
	return sctx, func() {
		once.Do(func() {
			cancel()
			c.SetReadDeadline(time.Now())
			<-done
			c.SetReadDeadline(time.Time{})
		})
	}
}

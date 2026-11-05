// Package client orquesta una medición contra un servidor.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"nperf/internal/hrtime"
	"nperf/internal/meter"
	"nperf/internal/proto"
	"nperf/internal/version"
)

// udpMagic es el prefijo del saludo UDP (igual que server.UDPMagic).
const udpMagic = "NPUDP1 "

const margin = 10 * time.Second

type Options struct {
	Duration  time.Duration // por sentido
	Proto     string        // "tcp" (por defecto), "udp" o "both" (RTT, TCP y UDP)
	Direction string        // "fwd" (por defecto), "rev" o "both" (fwd y luego rev)
	UDPRate   float64       // bps; 0 = DefaultUDPRate (en "both": 90 % del TCP del mismo sentido)
	Key       string        // clave compartida; vacía solo es válida si el servidor tampoco tiene
	Streams   int           // flujos TCP paralelos; 0 = proto.DefaultStreams (1-proto.MaxStreams)
	Version   string        // vacío = version.Current (se cambia solo en pruebas)
	// OnQueue se llama con la posición en cola (1 = el siguiente) cada vez que el servidor la
	// informa; mientras espera en cola no rigen plazos de lectura.
	OnQueue func(pos int)
	// HandshakeWait es el plazo de conexión, handshake y obtención de turno cuando no hay cola
	// (0 = 10 s). La espera en cola no cuenta.
	HandshakeWait time.Duration
	// DialTimeout acota cada conexión TCP (control y datos); 0 = 5 s.
	DialTimeout time.Duration
	// DialContext reemplaza al marcado TCP (pruebas); nil = net.Dialer.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// errDialTimeout marca el vencimiento del plazo de conexión (el mensaje ya trae host y puerto).
type errDialTimeout struct{ addr string }

func (e errDialTimeout) Error() string {
	return fmt.Sprintf("no se pudo conectar a %s (tiempo agotado)", e.addr)
}

const defaultDialTimeout = 5 * time.Second

// dialTCP conecta con plazo propio; si vence, el error lo dice en claro.
func dialTCP(ctx context.Context, o *Options, addr string) (net.Conn, error) {
	d := o.DialTimeout
	if d == 0 {
		d = defaultDialTimeout
	}
	dctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	dial := o.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	c, err := dial(dctx, "tcp", addr)
	if err != nil && ctx.Err() == nil && dctx.Err() != nil {
		return nil, errDialTimeout{addr}
	}
	return c, err
}

// ValidateStreams comprueba el número de flujos (1-proto.MaxStreams).
func ValidateStreams(n int) error {
	if n < 1 || n > proto.MaxStreams {
		return fmt.Errorf("-streams debe estar entre 1 y %d (se pidió %d)", proto.MaxStreams, n)
	}
	return nil
}

// DefaultUDPRate es la tasa UDP cuando no hay TCP previo: 100 Mbps.
const DefaultUDPRate = 100e6

// UDPResult son las métricas UDP de una prueba.
type UDPResult struct {
	SentMbps float64 // tasa enviada por el emisor
	Packets  int64   // datagramas enviados
	Expected int64
	Lost     int64
	LossPct  float64
	JitterMS float64
}

// UDP derivado en la secuencia completa ("both").
const (
	UDPFraction   = 0.9  // fracción del throughput TCP del mismo sentido
	MaxDerivedUDP = 10e9 // tope (bps) de la tasa derivada; ver docs/decisiones.md sección 16
	rttPings      = 10
)

// DeriveUDPRate devuelve la tasa UDP (bps) para un TCP medido en Mbps: 90 % acotado a
// MaxDerivedUDP. Devuelve 0 si el TCP no dio una medición utilizable.
func DeriveUDPRate(tcpMbps float64) float64 {
	r := tcpMbps * 1e6 * UDPFraction
	if math.IsNaN(r) || r < 8*meter.DefaultUDPSize {
		return 0
	}
	return math.Min(r, MaxDerivedUDP)
}

// RTT es el tiempo de ida y vuelta medido por el canal de control antes de cualquier carga.
// Se reportan mínimo, media y máximo de Samples pings (el mínimo es la mejor estimación de
// la latencia de la red; la media incluye la variación).
type RTT struct {
	Min, Avg, Max time.Duration
	Samples       int
}

// Dir es el resultado de un sentido. Measured=false significa "no medido" (no se pidió),
// distinto de una medición de 0.
type Dir struct {
	Measured bool
	Sent     meter.Result // lo enviado por el emisor
	Received meter.Result // lo que midió el receptor
	Mbps     float64      // lo medido por el receptor
	UDP      *UDPResult
}

// Result representa ambos sentidos. Fwd es cliente -> servidor y Rev servidor -> cliente.
// Sent, Received, Mbps y UDP repiten el primer sentido medido (atajo para pruebas de un solo sentido).
//
// Con Proto "both": RTT se mide primero; Fwd/Rev son TCP; UDPFwd/UDPRev son UDP (con la tasa
// pedida en UDPRateFwd/UDPRateRev, bps). Orden de resultados: RTT, TCP fwd, TCP rev, UDP fwd,
// UDP rev. Si el TCP de un sentido da 0 y no hay -udp-rate, su UDP queda "no medido" y se
// explica en Notes.
type Result struct {
	Fwd, Rev               Dir
	RTT                    *RTT
	UDPFwd, UDPRev         Dir
	UDPRateFwd, UDPRateRev float64
	Notes                  []string
	Sent                   meter.Result
	Received               meter.Result
	Mbps                   float64
	UDP                    *UDPResult
}

func (r *Result) set(dir string, d Dir) {
	first := !r.Fwd.Measured && !r.Rev.Measured
	if dir == proto.DirRev {
		r.Rev = d
	} else {
		r.Fwd = d
	}
	if first {
		r.Sent, r.Received, r.Mbps, r.UDP = d.Sent, d.Received, d.Mbps, d.UDP
	}
}

// Run mide TCP o UDP en el sentido pedido. En both hace fwd y luego rev, cada uno con la
// duración completa, dentro de una única sesión autenticada (un solo turno del servidor).
func Run(ctx context.Context, addr string, o Options) (Result, error) {
	var res Result
	var dirs []string
	switch o.Direction {
	case "", proto.DirFwd:
		dirs = []string{proto.DirFwd}
	case proto.DirRev:
		dirs = []string{proto.DirRev}
	case proto.DirBoth:
		dirs = []string{proto.DirFwd, proto.DirRev}
	default:
		return res, fmt.Errorf("sentido no soportado: %q", o.Direction)
	}
	if o.Streams == 0 {
		o.Streams = proto.DefaultStreams
	}
	if err := ValidateStreams(o.Streams); err != nil {
		return res, err
	}
	if o.Proto != "" && o.Proto != proto.ProtoTCP && o.Proto != proto.ProtoUDP && o.Proto != proto.ProtoBoth {
		return res, fmt.Errorf("protocolo no soportado: %q", o.Proto)
	}
	ss, err := openSession(ctx, addr, &o)
	if err != nil {
		return res, err
	}
	defer ss.close()
	if o.Proto == proto.ProtoBoth {
		return runFull(ctx, ss, o, dirs)
	}
	for _, dir := range dirs {
		d, err := runPhase(ctx, ss, o, dir, nil)
		if err != nil {
			return res, err
		}
		res.set(dir, d)
	}
	return res, nil
}

// runFull hace RTT (antes de cualquier carga), TCP en cada sentido y luego UDP en cada
// sentido, todo en la misma sesión. Un error de cualquier fase aborta toda la medición.
func runFull(ctx context.Context, ss *sess, o Options, dirs []string) (Result, error) {
	var res Result
	tcp := o
	tcp.Proto = proto.ProtoTCP
	rtt := new(RTT)
	for i, dir := range dirs {
		var r *RTT
		if i == 0 {
			r = rtt
		}
		d, err := runPhase(ctx, ss, tcp, dir, r)
		if err != nil {
			return res, err
		}
		res.set(dir, d)
	}
	res.RTT = rtt
	udp := o
	udp.Proto = proto.ProtoUDP
	for _, dir := range dirs {
		tcpDir := res.Fwd
		if dir == proto.DirRev {
			tcpDir = res.Rev
		}
		rate := o.UDPRate
		if rate == 0 {
			if rate = DeriveUDPRate(tcpDir.Mbps); rate == 0 {
				res.Notes = append(res.Notes, fmt.Sprintf(
					"UDP %s no medido: el TCP del mismo sentido no dio una tasa utilizable (%.3f Mbps); use -udp-rate", dir, tcpDir.Mbps))
				continue
			}
		}
		udp.UDPRate = rate
		d, err := runPhase(ctx, ss, udp, dir, nil)
		if err != nil {
			return res, err
		}
		if dir == proto.DirRev {
			res.UDPRev, res.UDPRateRev = d, rate
		} else {
			res.UDPFwd, res.UDPRateFwd = d, rate
		}
	}
	return res, nil
}

// measureRTT envía ppings por el canal de control y mide cada pong.
func measureRTT(codec *proto.Codec) (*RTT, error) {
	r := &RTT{}
	var sum time.Duration
	for i := 0; i < rttPings; i++ {
		t0 := hrtime.Now()
		if err := codec.Write(proto.Message{Type: proto.TypePing}); err != nil {
			return nil, err
		}
		if _, err := expect(codec, proto.TypePong); err != nil {
			return nil, err
		}
		d := hrtime.Since(t0)
		if i == 0 || d < r.Min {
			r.Min = d
		}
		if d > r.Max {
			r.Max = d
		}
		sum += d
		r.Samples++
	}
	r.Avg = sum / time.Duration(r.Samples)
	return r, nil
}

// sess es una sesión autenticada con el turno del servidor, que admite varias fases.
type sess struct {
	cc     net.Conn
	codec  *proto.Codec
	id     string
	addr   string
	dialer net.Dialer
	stop   func() bool
}

func (s *sess) close() { s.stop(); s.cc.Close() }

func openSession(ctx context.Context, addr string, o *Options) (*sess, error) {
	s := &sess{addr: addr}
	hs := o.HandshakeWait
	if hs == 0 {
		hs = 10 * time.Second
	}
	cc, err := dialTCP(ctx, o, addr)
	if err != nil {
		if errors.As(err, new(errDialTimeout)) {
			return nil, err
		}
		return nil, fmt.Errorf("no se pudo conectar a %s: %w", addr, err)
	}
	s.cc = cc
	s.stop = context.AfterFunc(ctx, func() { cc.Close() })
	s.codec = proto.NewCodec(cc)
	cc.SetDeadline(time.Now().Add(hs)) // handshake y solicitud de turno
	if o.Version == "" {
		o.Version = version.Current
	}
	// Handshake antes de cualquier carga o saludo UDP.
	if s.id, err = proto.ClientHandshake(s.codec, o.Key, o.Version); err != nil {
		s.close()
		return nil, err
	}
	// En cola no rige ningún plazo (la espera la decide el servidor); al llegar el turno
	// vuelven los plazos normales.
	err = proto.ClientTurn(s.codec, func(pos int) {
		cc.SetDeadline(time.Time{})
		if o.OnQueue != nil {
			o.OnQueue(pos)
		}
	})
	if err != nil {
		s.close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	cc.SetDeadline(time.Now().Add(hs))
	return s, nil
}

func runPhase(ctx context.Context, ss *sess, o Options, dir string, rtt *RTT) (Dir, error) {
	var res Dir
	cc, codec, session, addr, dialer := ss.cc, ss.codec, ss.id, ss.addr, &ss.dialer
	if rtt != nil { // antes de params: ni datos ni UDP han empezado
		m, err := measureRTT(codec)
		if err != nil {
			return res, err
		}
		*rtt = *m
	}
	if o.Proto == "" {
		o.Proto = proto.ProtoTCP
	}
	// Plazo del canal de control en esta fase: duración + margen.
	cc.SetDeadline(time.Now().Add(o.Duration + margin))
	rate := o.UDPRate
	if rate == 0 {
		rate = DefaultUDPRate
	}
	params := proto.Message{Type: proto.TypeParams, Proto: o.Proto,
		Direction: dir, DurationMS: o.Duration.Milliseconds(), Streams: 1}
	if o.Proto == proto.ProtoTCP {
		params.Streams = o.Streams
	}
	if o.Proto == proto.ProtoUDP && dir == proto.DirRev {
		params.RateBps = rate
	}
	if err := codec.Write(params); err != nil {
		return res, err
	}
	if _, err := expect(codec, proto.TypeStart); err != nil {
		return res, err
	}

	if o.Proto == proto.ProtoUDP {
		return runUDP(ctx, dialer, addr, codec, session, o, rate, dir)
	}
	conns := make([]net.Conn, 0, o.Streams)
	var connsMu sync.Mutex // closeAll puede correr desde AfterFunc mientras se abren flujos
	closeAll := func() {
		connsMu.Lock()
		defer connsMu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	}
	defer closeAll()
	stopData := context.AfterFunc(ctx, closeAll)
	defer stopData()
	for i := 0; i < o.Streams; i++ {
		dc, err := dialTCP(ctx, &o, addr)
		if err != nil {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			return res, fmt.Errorf("no se pudo abrir el flujo de datos %d de %d: %w", i+1, o.Streams, err)
		}
		connsMu.Lock()
		conns = append(conns, dc)
		connsMu.Unlock()
		if err := proto.NewCodec(dc).Write(proto.Message{Type: proto.TypeData, Session: session}); err != nil {
			return res, err
		}
	}
	if dir == proto.DirRev {
		rs := make([]io.Reader, len(conns))
		for i, c := range conns {
			c.SetReadDeadline(time.Now().Add(o.Duration + margin))
			rs[i] = c
		}
		rr, err := meter.ReceiveTCPN(rs, closeAll)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if err != nil {
			return res, err
		}
		r, err := expect(codec, proto.TypeResult)
		if err != nil {
			return res, err
		}
		sent := meter.Result{Bytes: r.Bytes, Elapsed: time.Duration(r.DurationMS) * time.Millisecond}
		if r.Retrans != nil { // lo informa el emisor (servidor)
			sent.Retrans, sent.RetransOK, sent.RetransEstimated = *r.Retrans, true, r.RetransEst
		}
		return Dir{Measured: true, Received: rr, Mbps: rr.Mbps(), Sent: sent}, nil
	}
	var err error
	res.Sent, err = meter.SendTCPN(ctx, conns, o.Duration)
	if err != nil {
		return res, err
	}
	r, err := expect(codec, proto.TypeResult)
	if err != nil {
		return res, err
	}
	res.Measured = true
	res.Received = meter.Result{Bytes: r.Bytes, Elapsed: time.Duration(r.DurationMS) * time.Millisecond}
	res.Mbps = r.Mbps
	return res, nil
}

func expect(c *proto.Codec, typ string) (proto.Message, error) {
	m, err := c.Read()
	if err != nil {
		return m, fmt.Errorf("canal de control cerrado: %w", err)
	}
	if m.Type == proto.TypeError {
		return m, fmt.Errorf("el servidor respondió: %s", m.Error)
	}
	if m.Type != typ {
		return m, fmt.Errorf("mensaje inesperado %q (se esperaba %q)", m.Type, typ)
	}
	return m, nil
}

func runUDP(ctx context.Context, dialer *net.Dialer, addr string, codec *proto.Codec, session string, o Options, rate float64, dir string) (Dir, error) {
	var res Dir
	uc, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return res, fmt.Errorf("no se pudo abrir el socket UDP: %w", err)
	}
	defer uc.Close()
	stop := context.AfterFunc(ctx, func() { uc.Close() })
	defer stop()
	if err := greetUDP(ctx, uc, session); err != nil {
		return res, err
	}
	if dir == proto.DirRev {
		uc.SetReadDeadline(time.Now().Add(o.Duration + margin))
		st, err := meter.ReceiveUDP(ctx, uc, time.Second)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if err != nil {
			return res, err
		}
		if st.Received == 0 {
			return res, fmt.Errorf("no llegaron datagramas UDP del servidor")
		}
		r, err := expect(codec, proto.TypeResult)
		if err != nil {
			return res, err
		}
		return Dir{Measured: true, Received: st.Result, Mbps: st.Mbps(),
			Sent: meter.Result{Bytes: r.Bytes, Elapsed: time.Duration(r.DurationMS) * time.Millisecond},
			UDP: &UDPResult{SentMbps: r.Mbps, Packets: r.Packets, Expected: st.Expected, Lost: st.Lost,
				LossPct: st.LossPercent(), JitterMS: float64(st.Jitter) / float64(time.Millisecond)}}, nil
	}
	st, err := meter.SendUDP(ctx, uc, rate, meter.DefaultUDPSize, o.Duration)
	if err != nil {
		return res, err
	}
	res.Sent = meter.Result{Bytes: st.Bytes, Elapsed: st.Elapsed}
	r, err := expect(codec, proto.TypeResult)
	if err != nil {
		return res, err
	}
	res.Measured = true
	res.Received = meter.Result{Bytes: r.Bytes, Elapsed: time.Duration(r.DurationMS) * time.Millisecond}
	res.Mbps = r.Mbps
	res.UDP = &UDPResult{SentMbps: st.Mbps(), Packets: st.Packets, Expected: r.Expected,
		Lost: r.Lost, LossPct: r.LossPct, JitterMS: r.JitterMS}
	return res, nil
}

// greetUDP asocia el socket UDP con la sesión: envía el saludo hasta recibir el acuse.
var (
	greetTries = 10
	greetWait  = 300 * time.Millisecond
)

func greetUDP(ctx context.Context, uc net.Conn, session string) error {
	hello := []byte(udpMagic + session)
	buf := make([]byte, 64)
	for i := 0; i < greetTries; i++ {
		if _, err := uc.Write(hello); err != nil {
			return fmt.Errorf("no se pudo enviar el saludo UDP: %w", err)
		}
		uc.SetReadDeadline(time.Now().Add(greetWait))
		n, err := uc.Read(buf)
		if err == nil && string(buf[:n]) == udpMagic+"ok" {
			uc.SetReadDeadline(time.Time{})
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("el servidor no respondió al saludo UDP (¿puerto UDP bloqueado?)")
}

// ParseRate convierte "50M", "1G", "800k" o un número en bps a bits por segundo (base 1000).
func ParseRate(s string) (float64, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, fmt.Errorf("tasa UDP vacía")
	}
	mult := 1.0
	switch t[len(t)-1] {
	case 'k', 'K':
		mult = 1e3
	case 'm', 'M':
		mult = 1e6
	case 'g', 'G':
		mult = 1e9
	}
	if mult != 1 {
		t = t[:len(t)-1]
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("tasa UDP inválida %q (ejemplos: 50M, 1G, 100000000)", s)
	}
	v *= mult
	if v < 8*meter.DefaultUDPSize { // al menos un datagrama por segundo
		return 0, fmt.Errorf("tasa UDP demasiado baja o no positiva: %q", s)
	}
	return v, nil
}

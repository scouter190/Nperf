// Package client orquesta una medición contra un servidor.
package client

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

const Version = "0.1.0"

// udpMagic es el prefijo del saludo UDP (igual que server.UDPMagic).
const udpMagic = "NPUDP1 "

const margin = 10 * time.Second

type Options struct {
	Duration time.Duration
	Proto    string  // "tcp" (por defecto) o "udp"
	UDPRate  float64 // bps; 0 = DefaultUDPRate
}

// DefaultUDPRate es la tasa UDP cuando no hay TCP previo: 100 Mbps.
const DefaultUDPRate = 100e6

// UDPResult son las métricas UDP de una prueba.
type UDPResult struct {
	SentMbps float64 // tasa enviada por el cliente
	Packets  int64   // datagramas enviados
	Expected int64
	Lost     int64
	LossPct  float64
	JitterMS float64
}

type Result struct {
	Sent     meter.Result // lo enviado por el cliente
	Received meter.Result // lo que midió el servidor
	Mbps     float64      // lo medido por el servidor
	UDP      *UDPResult
}

// Run mide un flujo TCP o UDP fwd (cliente -> servidor).
func Run(ctx context.Context, addr string, o Options) (Result, error) {
	var res Result
	var dialer net.Dialer
	cc, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return res, fmt.Errorf("no se pudo conectar a %s: %w", addr, err)
	}
	defer cc.Close()
	stop := context.AfterFunc(ctx, func() { cc.Close() })
	defer stop()
	// Plazo global del canal de control: duración + margen.
	cc.SetDeadline(time.Now().Add(o.Duration + margin))
	codec := proto.NewCodec(cc)

	if err := codec.Write(proto.Message{Type: proto.TypeHello, Version: Version}); err != nil {
		return res, err
	}
	hello, err := expect(codec, proto.TypeHello)
	if err != nil {
		return res, err
	}
	if o.Proto == "" {
		o.Proto = proto.ProtoTCP
	}
	if o.Proto != proto.ProtoTCP && o.Proto != proto.ProtoUDP {
		return res, fmt.Errorf("protocolo no soportado: %q", o.Proto)
	}
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: o.Proto,
		Direction: proto.DirFwd, DurationMS: o.Duration.Milliseconds(), Streams: 1}); err != nil {
		return res, err
	}
	if _, err := expect(codec, proto.TypeStart); err != nil {
		return res, err
	}

	if o.Proto == proto.ProtoUDP {
		return runUDP(ctx, &dialer, addr, codec, hello.Session, o, res)
	}
	dc, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return res, fmt.Errorf("no se pudo abrir el flujo de datos: %w", err)
	}
	defer dc.Close()
	stopData := context.AfterFunc(ctx, func() { dc.Close() })
	defer stopData()
	if err := proto.NewCodec(dc).Write(proto.Message{Type: proto.TypeData, Session: hello.Session}); err != nil {
		return res, err
	}
	res.Sent, err = meter.SendTCP(ctx, dc, o.Duration)
	if err != nil {
		return res, err
	}
	r, err := expect(codec, proto.TypeResult)
	if err != nil {
		return res, err
	}
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

func runUDP(ctx context.Context, dialer *net.Dialer, addr string, codec *proto.Codec, session string, o Options, res Result) (Result, error) {
	rate := o.UDPRate
	if rate == 0 {
		rate = DefaultUDPRate
	}
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
	st, err := meter.SendUDP(ctx, uc, rate, meter.DefaultUDPSize, o.Duration)
	if err != nil {
		return res, err
	}
	res.Sent = meter.Result{Bytes: st.Bytes, Elapsed: st.Elapsed}
	r, err := expect(codec, proto.TypeResult)
	if err != nil {
		return res, err
	}
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

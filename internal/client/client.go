// Package client orquesta una medición contra un servidor.
package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

const Version = "0.1.0"

const margin = 10 * time.Second

type Options struct {
	Duration time.Duration
}

type Result struct {
	Sent     meter.Result // lo enviado por el cliente
	Received meter.Result // lo que midió el servidor
	Mbps     float64
}

// Run mide un flujo TCP fwd (cliente -> servidor).
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
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoTCP,
		Direction: proto.DirFwd, DurationMS: o.Duration.Milliseconds(), Streams: 1}); err != nil {
		return res, err
	}
	if _, err := expect(codec, proto.TypeStart); err != nil {
		return res, err
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

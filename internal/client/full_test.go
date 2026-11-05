package client_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
	"nperf/internal/version"
)

func TestDeriveUDPRate(t *testing.T) {
	if got := client.DeriveUDPRate(100); got != 90e6 {
		t.Errorf("100 Mbps -> %v, quería 90e6", got)
	}
	if got := client.DeriveUDPRate(40000); got != client.MaxDerivedUDP {
		t.Errorf("tope: %v", got)
	}
	if got := client.DeriveUDPRate(0); got != 0 {
		t.Errorf("0 Mbps -> %v, quería 0", got)
	}
}

func TestFullSequence(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 500 * time.Millisecond, Proto: "both", Direction: "both"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RTT == nil || res.RTT.Samples != 10 || res.RTT.Min < 0 || res.RTT.Min > res.RTT.Avg || res.RTT.Avg > res.RTT.Max || res.RTT.Max <= 0 {
		t.Fatalf("RTT inválido: %+v", res.RTT)
	}
	for name, d := range map[string]client.Dir{"tcp fwd": res.Fwd, "tcp rev": res.Rev, "udp fwd": res.UDPFwd, "udp rev": res.UDPRev} {
		if !d.Measured || d.Mbps <= 0 {
			t.Errorf("%s no medido: %+v", name, d)
		}
	}
	if res.Fwd.UDP != nil || res.Rev.UDP != nil || res.UDPFwd.UDP == nil || res.UDPRev.UDP == nil {
		t.Errorf("TCP/UDP mezclados")
	}
	// UDP = 90 % del TCP del mismo sentido (con tope).
	for _, c := range []struct {
		name      string
		tcp, rate float64
	}{{"fwd", res.Fwd.Mbps, res.UDPRateFwd}, {"rev", res.Rev.Mbps, res.UDPRateRev}} {
		want := client.DeriveUDPRate(c.tcp)
		if c.rate < want*0.95 || c.rate > want*1.05 {
			t.Errorf("%s: tasa UDP %.0f, quería %.0f (90%% de %.0f Mbps)", c.name, c.rate, want, c.tcp)
		}
	}
}

func TestFullUDPRateWins(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 500 * time.Millisecond, Proto: "both", Direction: "fwd", UDPRate: 20e6})
	if err != nil {
		t.Fatal(err)
	}
	if res.UDPRateFwd != 20e6 || !res.UDPFwd.Measured || res.Rev.Measured || res.UDPRev.Measured {
		t.Fatalf("udp-rate: %+v", res)
	}
}

// El RTT se mide por el canal de control antes de params (antes de cualquier carga).
func TestRTTBeforeLoad(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	seq := make(chan []string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		codec := proto.NewCodec(c)
		first, _ := codec.Read()
		if _, err := proto.ServerHandshake(codec, first, "", version.Current); err != nil {
			seq <- nil
			return
		}
		var types []string
		for {
			m, err := codec.Read()
			if err != nil {
				break
			}
			types = append(types, m.Type)
			if m.Type == proto.TypePing {
				codec.Write(proto.Message{Type: proto.TypePong})
				continue
			}
			codec.Write(proto.Message{Type: proto.TypeError, Error: "fin"})
			break
		}
		seq <- types
	}()
	_, err = client.Run(context.Background(), ln.Addr().String(), client.Options{Duration: time.Second, Proto: "both", Direction: "fwd"})
	if err == nil {
		t.Fatal("se esperaba el error del servidor falso")
	}
	types := <-seq
	if len(types) < 2 || types[len(types)-1] != proto.TypeParams {
		t.Fatalf("secuencia: %v", types)
	}
	for _, ty := range types[:len(types)-1] {
		if ty != proto.TypePing {
			t.Fatalf("antes de params solo ping: %v", types)
		}
	}
}

package client_test

import (
	"context"
	"math"
	"testing"
	"time"

	"nperf/internal/client"
)

func TestParseRate(t *testing.T) {
	ok := map[string]float64{"50M": 50e6, "50m": 50e6, "1G": 1e9, "1.5G": 1.5e9, "800k": 800e3,
		"100000000": 1e8, " 20M ": 20e6}
	for in, want := range ok {
		got, err := client.ParseRate(in)
		if err != nil || math.Abs(got-want) > 1 {
			t.Errorf("ParseRate(%q) = %v, %v; quería %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "M", "abc", "-5M", "0", "0M", "NaN", "Inf", "5X", "1e400", "10"} {
		if got, err := client.ParseRate(in); err == nil {
			t.Errorf("ParseRate(%q) = %v; quería error", in, got)
		}
	}
}

func TestUDPRate(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	for _, c := range []struct {
		name string
		rate float64
		want float64
	}{{"50M", 50e6, 50}, {"default", 0, 100}} {
		res, err := client.Run(context.Background(), addr,
			client.Options{Duration: 2 * time.Second, Proto: "udp", UDPRate: c.rate})
		if err != nil || res.Fwd.UDP == nil {
			t.Fatalf("%s: %+v %v", c.name, res, err)
		}
		if math.Abs(res.Fwd.UDP.SentMbps-c.want) > c.want*0.05 {
			t.Errorf("%s: enviado %.1f Mbps, quería %.0f ±5 %%", c.name, res.Fwd.UDP.SentMbps, c.want)
		}
		if res.Fwd.Received.Bytes <= 0 || res.Fwd.Mbps <= 0 {
			t.Errorf("%s: el servidor no midió nada: %+v", c.name, res)
		}
		t.Logf("%s: enviado %.1f Mbps, recibido %.1f, jitter %.3f ms, pérdida %.2f %%",
			c.name, res.Fwd.UDP.SentMbps, res.Fwd.Mbps, res.Fwd.UDP.JitterMS, res.Fwd.UDP.LossPct)
	}
	// TCP sigue funcionando en el mismo servidor.
	if res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond}); err != nil || res.Fwd.Mbps <= 0 {
		t.Fatalf("TCP tras UDP: %+v %v", res, err)
	}
}

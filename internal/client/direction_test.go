package client_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/server"
)

func TestDirections(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	for _, p := range []string{"tcp", "udp"} {
		for _, dir := range []string{"fwd", "rev", "both"} {
			res, err := client.Run(context.Background(), addr,
				client.Options{Duration: 500 * time.Millisecond, Proto: p, Direction: dir, UDPRate: 20e6})
			if err != nil {
				t.Fatalf("%s/%s: %v", p, dir, err)
			}
			wantF, wantR := dir != "rev", dir != "fwd"
			if res.Fwd.Measured != wantF || res.Rev.Measured != wantR {
				t.Errorf("%s/%s: medidos fwd=%v rev=%v", p, dir, res.Fwd.Measured, res.Rev.Measured)
			}
			for name, d := range map[string]client.Dir{"fwd": res.Fwd, "rev": res.Rev} {
				if d.Measured && (d.Mbps <= 0 || d.Received.Bytes <= 0) {
					t.Errorf("%s/%s %s: sin throughput: %+v", p, dir, name, d)
				}
				if !d.Measured && (d.Mbps != 0 || d.Received.Bytes != 0 || d.UDP != nil) {
					t.Errorf("%s/%s %s: no medido con datos: %+v", p, dir, name, d)
				}
				if d.Measured && (d.UDP != nil) != (p == "udp") {
					t.Errorf("%s/%s %s: UDP inconsistente", p, dir, name)
				}
			}
			t.Logf("%s/%s fwd=%.0f rev=%.0f Mbps", p, dir, res.Fwd.Mbps, res.Rev.Mbps)
		}
	}
}

func TestRevWrongKey(t *testing.T) {
	addr := startWith(t, server.WithKey("secreto"))
	if res, err := client.Run(context.Background(), addr,
		client.Options{Duration: 200 * time.Millisecond, Direction: "rev", Key: "otra"}); err == nil || res.Rev.Measured {
		t.Fatalf("rev con clave incorrecta: %+v %v", res, err)
	}
	if res, err := client.Run(context.Background(), addr,
		client.Options{Duration: 200 * time.Millisecond, Direction: "rev", Key: "secreto"}); err != nil || !res.Rev.Measured {
		t.Fatalf("rev con clave correcta: %+v %v", res, err)
	}
}

func TestCancelMidRev(t *testing.T) {
	for _, p := range []string{"tcp", "udp"} {
		addr, cancel, done := start(t)
		time.Sleep(50 * time.Millisecond)
		base := runtime.NumGoroutine()
		ctx, cc := context.WithCancel(context.Background())
		go func() { time.Sleep(500 * time.Millisecond); cc() }()
		t0 := time.Now()
		_, err := client.Run(ctx, addr, client.Options{Duration: 20 * time.Second, Proto: p, Direction: "rev", UDPRate: 20e6})
		if err == nil || time.Since(t0) > 5*time.Second {
			t.Fatalf("%s: cancelación: %v tras %v", p, err, time.Since(t0))
		}
		// El servidor debe dejar de enviar sin esperar los 20 s.
		deadline := time.Now().Add(3 * time.Second)
		for runtime.NumGoroutine() > base && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if n := runtime.NumGoroutine(); n > base {
			t.Errorf("%s: goroutines colgadas: %d > %d", p, n, base)
		}
		cancel()
		<-done
	}
}

// En TCP rev el emisor es el servidor: su contador viaja en el mensaje de resultado. En
// plataformas sin soporte llega "no disponible" (RetransOK false), también válido.
func TestRetransReported(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	for _, dir := range []string{"fwd", "rev"} {
		res, err := client.Run(context.Background(), addr,
			client.Options{Duration: 300 * time.Millisecond, Proto: "tcp", Direction: dir})
		if err != nil {
			t.Fatal(err)
		}
		d := res.Fwd
		if dir == "rev" {
			d = res.Rev
		}
		if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") && !d.Sent.RetransOK {
			t.Errorf("%s: retransmisiones no disponibles en Linux", dir)
		}
		if !d.Sent.RetransOK && d.Sent.Retrans != 0 {
			t.Errorf("%s: valor con no disponible", dir)
		}
	}
}

package client_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
)

type phaseLog struct {
	mu sync.Mutex
	n  map[string]int
}

func (p *phaseLog) add(phase string, _ client.Interval) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.n == nil {
		p.n = map[string]int{}
	}
	p.n[phase]++
}

func (p *phaseLog) count(phase string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n[phase]
}

func sumBytes(ivs []client.Interval) (n int64) {
	for _, iv := range ivs {
		n += iv.Bytes
	}
	return n
}

func checkLive(t *testing.T, name string, d client.Dir, wantN int, exactBytes bool, probes bool, log *phaseLog, phase string) {
	t.Helper()
	if !d.Measured || !d.Live || len(d.Intervals) != wantN {
		t.Fatalf("%s: measured=%v live=%v intervalos=%d (quería %d)", name, d.Measured, d.Live, len(d.Intervals), wantN)
	}
	for i, iv := range d.Intervals {
		if iv.Index != i || iv.Duration <= 0 {
			t.Errorf("%s: intervalo %d inválido: %+v", name, i, iv)
		}
	}
	if sum := sumBytes(d.Intervals); sum <= 0 || (exactBytes && sum != d.Received.Bytes) {
		t.Errorf("%s: suma de bytes %d vs recibidos %d", name, sum, d.Received.Bytes)
	}
	if log.count(phase) != wantN {
		t.Errorf("%s: OnInterval %d veces (quería %d)", name, log.count(phase), wantN)
	}
	if probes {
		if d.LoadedRTT == nil || d.LoadedRTT.Samples < 1 || d.LoadedRTT.Min <= 0 || d.LoadedRTT.Min > d.LoadedRTT.Max {
			t.Errorf("%s: RTT bajo carga inválido: %+v", name, d.LoadedRTT)
		}
		if !d.Intervals[0].HasRTT {
			t.Errorf("%s: la sonda 0 debía dar RTT a la muestra 0", name)
		}
	} else if d.LoadedRTT != nil {
		t.Errorf("%s: LoadedRTT sin sondas: %+v", name, d.LoadedRTT)
	}
}

func TestLiveTCPFwdRev(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	var log phaseLog
	var ready, rtt int
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 1500 * time.Millisecond, Direction: "both", Proto: "tcp",
		OnInterval: log.add, OnReady: func(_, _ *proto.Link) { ready++ }, OnRTT: func(client.RTT) { rtt++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready != 1 || rtt != 0 {
		t.Errorf("OnReady=%d OnRTT=%d", ready, rtt)
	}
	checkLive(t, "fwd", res.Fwd, 2, true, true, &log, "tcp_fwd")
	checkLive(t, "rev", res.Rev, 2, true, true, &log, "tcp_rev")
}

func TestLiveFullBoth(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	var log phaseLog
	rtt := 0
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 1500 * time.Millisecond, Direction: "both", Proto: "both",
		OnInterval: log.add, OnRTT: func(r client.RTT) { rtt += r.Samples },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rtt != 10 || res.RTT == nil {
		t.Errorf("OnRTT acumuló %d muestras", rtt)
	}
	checkLive(t, "tcp fwd", res.Fwd, 2, true, true, &log, "tcp_fwd")
	checkLive(t, "tcp rev", res.Rev, 2, true, true, &log, "tcp_rev")
	checkLive(t, "udp fwd", res.UDPFwd, 2, false, true, &log, "udp_fwd")
	checkLive(t, "udp rev", res.UDPRev, 2, false, true, &log, "udp_rev")
	for _, d := range []client.Dir{res.UDPFwd, res.UDPRev} {
		if !d.Intervals[0].UDP || d.Intervals[0].Expected <= 0 {
			t.Errorf("muestra UDP sin datos de datagramas: %+v", d.Intervals[0])
		}
	}
}

func TestLiveNoLoadRTT(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	var log phaseLog
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 1500 * time.Millisecond, Direction: "both", Proto: "tcp", NoLoadRTT: true, OnInterval: log.add,
	})
	if err != nil {
		t.Fatal(err)
	}
	checkLive(t, "fwd", res.Fwd, 2, true, false, &log, "tcp_fwd")
	checkLive(t, "rev", res.Rev, 2, true, false, &log, "tcp_rev")
	for _, iv := range append(res.Fwd.Intervals, res.Rev.Intervals...) {
		if iv.HasRTT {
			t.Fatalf("NoLoadRTT con RTT en la muestra: %+v", iv)
		}
	}
}

func TestLiveCancelClosesCleanly(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	for _, dir := range []string{"fwd", "rev"} {
		ctx, stop := context.WithCancel(context.Background())
		time.AfterFunc(700*time.Millisecond, stop)
		t0 := time.Now()
		_, err := client.Run(ctx, addr, client.Options{Duration: 20 * time.Second, Direction: dir})
		if err == nil || time.Since(t0) > 5*time.Second {
			t.Fatalf("%s: err=%v tras %v", dir, err, time.Since(t0))
		}
		stop()
	}
}

// oldServer es un proxy delante del servidor real que se hace pasar por uno previo a "live":
// quita "live" de params (el servidor real no lo ve) y registra los tipos que el cliente
// escribe en el control.
func oldServer(t *testing.T, target string) (addr string, seen func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	udpRelay(t, ln.Addr().String(), target)
	var mu sync.Mutex
	var types []string
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				s, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer s.Close()
				go func() { io.Copy(c, s); c.Close() }()
				br := bufio.NewReader(c)
				first, err := br.ReadBytes('\n')
				if err != nil {
					return
				}
				s.Write(first)
				if bytes.Contains(first, []byte(`"type":"data"`)) {
					io.Copy(s, br)
					return
				}
				mu.Lock()
				types = append(types, "first")
				mu.Unlock()
				for {
					line, err := br.ReadBytes('\n')
					if err != nil {
						return
					}
					mu.Lock()
					for _, ty := range []string{"params", "ping", "pong", "sample"} {
						if bytes.Contains(line, []byte(`"type":"`+ty+`"`)) {
							types = append(types, ty)
						}
					}
					mu.Unlock()
					line = bytes.Replace(line, []byte(`,"live":true`), nil, 1)
					if _, err := s.Write(line); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), types...)
	}
}

func TestLiveServerWithoutLive(t *testing.T) {
	real, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	addr, seen := oldServer(t, real)
	var log phaseLog
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 700 * time.Millisecond, Direction: "both", Proto: "tcp", OnInterval: log.add,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]client.Dir{"fwd": res.Fwd, "rev": res.Rev} {
		if !d.Measured || d.Mbps <= 0 || d.Live || d.Intervals != nil || d.LoadedRTT != nil {
			t.Errorf("%s: %+v", name, d)
		}
	}
	if log.count("tcp_fwd")+log.count("tcp_rev") != 0 {
		t.Error("OnInterval sin live")
	}
	afterParams := false
	for _, ty := range seen() {
		if ty == "params" {
			afterParams = true
		} else if afterParams && ty != "first" {
			t.Fatalf("mensaje %q tras params con un servidor sin live: %v", ty, seen())
		}
	}
	if !afterParams {
		t.Fatal("el proxy no vio params")
	}
	if strings.Contains(strings.Join(res.Notes, " "), "bufferbloat") {
		t.Error("nota sin sondas")
	}
}

// udpRelay reenvía el UDP de listenAddr (mismo puerto que el proxy TCP) al servidor real.
func udpRelay(t *testing.T, listenAddr, target string) {
	t.Helper()
	pc, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	up := map[string]net.Conn{}
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 65536)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			mu.Lock()
			c := up[from.String()]
			if c == nil {
				if c, err = net.Dial("udp", target); err != nil {
					mu.Unlock()
					continue
				}
				up[from.String()] = c
				t.Cleanup(func() { c.Close() })
				go func() {
					b := make([]byte, 65536)
					for {
						n, err := c.Read(b)
						if err != nil {
							return
						}
						pc.WriteTo(b[:n], from)
					}
				}()
			}
			mu.Unlock()
			c.Write(buf[:n])
		}
	}()
}

func TestLiveServerWithoutLiveUDP(t *testing.T) {
	real, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	addr, seen := oldServer(t, real)
	var log phaseLog
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 700 * time.Millisecond, Direction: "both", Proto: "udp", UDPRate: 20e6, OnInterval: log.add,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]client.Dir{"fwd": res.Fwd, "rev": res.Rev} {
		if !d.Measured || d.Mbps <= 0 || d.UDP == nil || d.UDP.Expected <= 0 || d.Live || d.Intervals != nil || d.LoadedRTT != nil {
			t.Errorf("%s: %+v", name, d)
		}
	}
	if log.count("udp_fwd")+log.count("udp_rev") != 0 {
		t.Error("OnInterval sin live")
	}
	afterParams := false
	for _, ty := range seen() {
		if ty == "params" {
			afterParams = true
		} else if afterParams && ty != "first" {
			t.Fatalf("mensaje %q tras params con un servidor sin live: %v", ty, seen())
		}
	}
	if !afterParams {
		t.Fatal("el proxy no vio params")
	}
}

// badSampleProxy inyecta, antes de cada "sample" del servidor, una copia fuera de rango.
func badSampleProxy(t *testing.T, target string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	udpRelay(t, ln.Addr().String(), target)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				s, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer s.Close()
				go func() { io.Copy(s, c); s.Close() }()
				br := bufio.NewReader(s)
				for {
					line, err := br.ReadBytes('\n')
					if err != nil {
						return
					}
					if bytes.Contains(line, []byte(`"type":"sample"`)) {
						var m map[string]any
						if json.Unmarshal(line, &m) == nil {
							m["mbps"] = 1e12 // fuera de rango, mismo índice
							bad, _ := json.Marshal(m)
							c.Write(append(bad, '\n'))
						}
					}
					c.Write(line)
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestLiveInvalidPeerSampleIsDiscarded(t *testing.T) {
	real, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	addr := badSampleProxy(t, real)
	var log phaseLog
	res, err := client.Run(context.Background(), addr, client.Options{
		Duration: 1500 * time.Millisecond, Direction: "fwd", Proto: "tcp", OnInterval: log.add,
	})
	if err != nil {
		t.Fatalf("una muestra inválida abortó el TEST!: %v", err)
	}
	checkLive(t, "fwd", res.Fwd, 2, true, true, &log, "tcp_fwd")
	if !strings.Contains(strings.Join(res.Notes, " "), "muestras descartadas por valores no válidos") {
		t.Errorf("sin nota de descartadas: %v", res.Notes)
	}
}

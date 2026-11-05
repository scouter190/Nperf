package app

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
	"nperf/internal/report"
	"nperf/internal/version"
)

// StartTest inicia la prueba contra el destino elegido (RTT, TCP y UDP, 4 flujos) en una
// goroutine. Falla si no hay destino válido, la configuración es inválida o ya hay una prueba.
func (a *App) StartTest() error {
	p, err := a.Target()
	if err != nil {
		return err
	}
	cfg, err := a.cfg()
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.test.Running {
		a.mu.Unlock()
		return errors.New("ya hay una prueba en curso")
	}
	dir, dur := a.direction, a.duration
	ctx, cancel := context.WithCancel(context.Background())
	a.gen++
	gen := a.gen
	done := make(chan struct{})
	a.cancel, a.testDone = cancel, done
	a.test = TestState{Running: true, Status: TestQueued}
	a.mu.Unlock()

	// set aplica un cambio solo si la prueba sigue vigente y en curso.
	set := func(fn func(*TestState)) {
		a.mu.Lock()
		if a.gen == gen && a.test.Running && a.test.Status != TestCancelling {
			fn(&a.test)
		}
		a.mu.Unlock()
	}
	port := p.Port
	addr := net.JoinHostPort(p.IP.String(), strconv.Itoa(port))
	o := client.Options{
		Duration: time.Duration(dur) * time.Second, Proto: proto.ProtoBoth, Direction: dir,
		Key: cfg.Key, Streams: proto.DefaultStreams, Version: version.Current,
		OnQueue: func(pos int) {
			set(func(t *TestState) { t.Status, t.Position, t.Phase = TestQueued, pos, "" })
		},
		OnPhase: func(ph string) {
			set(func(t *TestState) { t.Status, t.Position, t.Phase = TestRunning, 0, ph })
		},
	}
	go func() {
		defer close(done)
		defer cancel()
		res, err := client.Run(ctx, addr, o)
		var rp report.Report
		if err == nil {
			rp = report.Build(report.Params{Host: p.IP.String(), Port: port, Proto: proto.ProtoBoth,
				Direction: dir, Duration: o.Duration, Streams: o.Streams, Version: version.Current}, res)
		}
		a.mu.Lock()
		if a.gen == gen && a.test.Running {
			switch {
			case a.test.Status == TestCancelling:
				a.test = TestState{Status: TestCancelled}
			case err != nil:
				a.test = TestState{Status: TestError, Error: err.Error()}
			default:
				a.test = TestState{Status: TestDone, Report: &rp}
			}
		}
		a.mu.Unlock()
	}()
	return nil
}

// CancelTest pide cancelar y vuelve de inmediato: el estado pasa a "cancelando" y la goroutine
// lo deja en "cancelled" al terminar. No hace nada si no hay prueba en curso.
func (a *App) CancelTest() {
	a.mu.Lock()
	cancel := a.cancel
	if a.test.Running && a.test.Status != TestCancelling {
		a.test = TestState{Running: true, Status: TestCancelling, Label: "Cancelando…"}
	}
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// WaitTest espera a que termine la goroutine de la prueba (hasta timeout); true si terminó o no había.
func (a *App) WaitTest(timeout time.Duration) bool {
	a.mu.Lock()
	done := a.testDone
	a.mu.Unlock()
	if done == nil {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

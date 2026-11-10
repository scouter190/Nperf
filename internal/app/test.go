package app

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"time"

	"nperf/internal/client"
	"nperf/internal/diag"
	"nperf/internal/netinfo"
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
		LocalLink: func(ip netip.Addr) *proto.Link { return a.localLink(ip) },
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
		var dg *Diagnosis
		if err == nil {
			dg = buildDiagnosis(res, a.Alias(), p.Alias, dir)
			rp = report.Build(report.Params{Host: p.IP.String(), Port: port, Proto: proto.ProtoBoth,
				Direction: dir, Duration: o.Duration, Streams: o.Streams, Version: version.Current}, res)
		}
		if a.testHook != nil {
			a.testHook()
		}
		a.mu.Lock()
		if a.gen == gen && a.test.Running {
			switch {
			case a.test.Status == TestCancelling:
				a.test = TestState{Status: TestCancelled}
			case err != nil:
				a.test = TestState{Status: TestError, Error: err.Error()}
			default:
				a.test = TestState{Status: TestDone, Report: &rp, Diagnosis: dg}
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

// localLink resume el enlace local para la IP local de la conexión (nil = desconocido).
func (a *App) localLink(ip netip.Addr) *proto.Link {
	if a.d.Net == nil {
		return nil
	}
	return linkForIP(a.d.Net, ip)
}

// DirDiagnosis es el semáforo de un sentido (Color vacío = sin semáforo).
type DirDiagnosis struct {
	Color   string `json:"color"`
	Percent int    `json:"percent"`
	Text    string `json:"text"`
	Partial bool   `json:"partial"`
}

// Diagnosis reúne el diagnóstico por sentido (nil = sentido no pedido) y los enlaces de
// ambos extremos (nil = desconocido).
type Diagnosis struct {
	Fwd    *DirDiagnosis `json:"fwd,omitempty"`
	Rev    *DirDiagnosis `json:"rev,omitempty"`
	Local  *proto.Link   `json:"local,omitempty"`
	Remote *proto.Link   `json:"remote,omitempty"`
}

func diagEnd(alias string, l *proto.Link) diag.End {
	e := diag.End{Alias: alias, Signal: -1}
	if l == nil {
		return e
	}
	e.Known = true
	switch l.Kind {
	case string(netinfo.KindEthernet):
		e.Kind = diag.KindWired
	case string(netinfo.KindWiFi):
		e.Kind = diag.KindWiFi
	case string(netinfo.KindOther):
		e.Kind = diag.KindOther
	default:
		e.Known = false
	}
	if l.TxBps > 0 {
		e.TxBps = uint64(l.TxBps)
	}
	if l.RxBps > 0 {
		e.RxBps = uint64(l.RxBps)
	}
	if l.SignalPct > 0 {
		e.Signal = l.SignalPct
	}
	return e
}

// buildDiagnosis evalúa el semáforo TCP de cada sentido pedido. En fwd emite el cliente
// (local) y recibe el servidor; en rev al revés.
func buildDiagnosis(res client.Result, localAlias, peerAlias, dir string) *Diagnosis {
	loc, rem := diagEnd(localAlias, res.LocalLink), diagEnd(peerAlias, res.RemoteLink)
	mk := func(d client.Dir, s, r diag.End) *DirDiagnosis {
		bps := -1.0
		if d.Measured {
			bps = d.Mbps * 1e6
		}
		x := diag.Evaluate(bps, s, r)
		return &DirDiagnosis{Color: string(x.Color), Percent: x.Percent, Text: x.Text, Partial: x.Partial}
	}
	dg := &Diagnosis{Local: res.LocalLink, Remote: res.RemoteLink}
	if dir != proto.DirRev {
		dg.Fwd = mk(res.Fwd, loc, rem)
	}
	if dir != proto.DirFwd {
		dg.Rev = mk(res.Rev, rem, loc)
	}
	return dg
}

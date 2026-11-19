package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"nperf/internal/netinfo"
	"nperf/internal/probe"
)

// Tipos de diagnóstico.
const (
	ProbePing  = "ping"
	ProbeTrace = "trace"
	ProbeMTU   = "mtu"
	ProbePorts = "ports"
)

// Estados del diagnóstico (ProbeState.Status).
const (
	ProbeIdle       = "idle"
	ProbeRunning    = "running"
	ProbeDone       = "done"
	ProbeError      = "error"
	ProbeCancelling = "cancelling"
	ProbeCancelled  = "cancelled"
)

// Mensajes de exclusión mutua entre TEST! y diagnósticos (decisiones §19).
const (
	ProbeBlockedByTest  = "hay un TEST! en curso: espere a que termine para lanzar un diagnóstico"
	TestBlockedByProbe  = "hay un diagnóstico en curso: espere a que termine para iniciar un TEST!"
	probeAlreadyRunning = "ya hay un diagnóstico en curso"
)

// CombinedState es el diagnóstico combinado (internal/probe.Diagnose).
type CombinedState struct {
	Messages []string `json:"messages"`
	Healthy  bool     `json:"healthy"`
}

// ProbeState es el estado del diagnóstico. Result es el JSON de internal/probe (solo en "done").
type ProbeState struct {
	Running  bool            `json:"running"`
	Status   string          `json:"status"`
	Kind     string          `json:"kind,omitempty"`
	TargetID string          `json:"target_id,omitempty"`
	Label    string          `json:"label,omitempty"`
	Error    string          `json:"error,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Combined *CombinedState  `json:"combined,omitempty"`
}

// probeTarget es el destino resuelto en texto: nombre pedido y, si ya es IP, la dirección.
func (a *App) probeTarget(id string) (string, error) {
	if id == "" {
		return "", errors.New("elija un equipo para el diagnóstico")
	}
	if id == a.d.SelfID {
		return "", errors.New("el destino no puede ser Este equipo")
	}
	cfg, _ := a.cfg()
	for _, d := range a.noAgentDevices(cfg) {
		if d.ID == id {
			return d.IPs[0], nil
		}
	}
	for _, p := range a.d.Table.Snapshot() {
		if p.InstanceID == id {
			return p.IP.String(), nil
		}
	}
	return "", errors.New("el equipo elegido ya no está en la lista")
}

func (a *App) router() probe.Router {
	if a.d.Router != nil {
		return a.d.Router
	}
	return a.d.Probe.Router
}

// StartProbe inicia un diagnóstico (ping, trace, mtu o ports) con los valores por defecto
// contra el equipo id, en una goroutine. Rechaza si hay un TEST! o ya otro diagnóstico.
func (a *App) StartProbe(kind, id string) error {
	switch kind {
	case ProbePing, ProbeTrace, ProbeMTU:
		if a.d.Probe.Echoer == nil {
			return errors.New("los diagnósticos ICMP no están disponibles en este sistema")
		}
	case ProbePorts:
		if a.d.Probe.Dialer == nil {
			return errors.New("la prueba de puertos no está disponible")
		}
	default:
		return fmt.Errorf("diagnóstico desconocido %q: use ping, trace, mtu o ports", kind)
	}
	raw, err := a.probeTarget(id)
	if err != nil {
		return err
	}
	tgt, err := probe.ParseTarget(raw)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.test.Running {
		a.mu.Unlock()
		return errors.New(ProbeBlockedByTest)
	}
	if a.probe.Running {
		a.mu.Unlock()
		return errors.New(probeAlreadyRunning)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.probeGen++
	gen := a.probeGen
	done := make(chan struct{})
	a.probeCancel, a.probeDone = cancel, done
	a.probe = ProbeState{Running: true, Status: ProbeRunning, Kind: kind, TargetID: id}
	a.mu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		res, cmb, err := a.runProbe(ctx, kind, tgt)
		if a.probeHook != nil {
			a.probeHook()
		}
		a.mu.Lock()
		if a.probeGen != gen || !a.probe.Running {
			a.mu.Unlock()
			return
		}
		var final ProbeState
		switch {
		case a.probe.Status == ProbeCancelling || ctx.Err() != nil:
			final = ProbeState{Status: ProbeCancelled, Kind: kind, TargetID: id}
		case err != nil:
			final = ProbeState{Status: ProbeError, Kind: kind, TargetID: id, Error: err.Error()}
		default:
			final = ProbeState{Status: ProbeDone, Kind: kind, TargetID: id, Result: res, Combined: cmb}
		}
		a.mu.Unlock()

		// Se guarda antes de publicar, para que el registro sea igual a State().
		herr := a.saveRecord(a.probeRecord(final, tgt.Raw))

		a.mu.Lock()
		a.histWarn = histWarning(herr)
		if a.probeGen == gen && a.probe.Running {
			a.probe = final
		}
		a.mu.Unlock()
	}()
	return nil
}

// runProbe resuelve el destino y ejecuta el diagnóstico con los valores por defecto.
func (a *App) runProbe(ctx context.Context, kind string, tgt probe.Target) (json.RawMessage, *CombinedState, error) {
	env := a.d.Probe
	if env.Clock == nil {
		env.Clock = probe.RealClock{}
	}
	ip, err := probe.Resolve(ctx, tgt, env.Resolver)
	if err != nil {
		return nil, nil, err
	}
	rt := a.router()
	var rte probe.Route
	haveRoute := false
	if rt != nil {
		local, _ := rt.LocalPrefixes()
		if err := probe.CheckAddr(ip, local); err != nil {
			return nil, nil, err
		}
		if r, err := rt.Route(ip); err == nil {
			rte, haveRoute = r, true
		}
	}
	dest := probe.Dest{Name: tgt.Raw, IP: ip}
	var buf bytes.Buffer
	var in *probe.CombinedInput
	switch kind {
	case ProbePing:
		st, replies, err := probe.Ping(ctx, env, ip, probe.DefaultPing())
		if err != nil {
			return nil, nil, err
		}
		if err := probe.PingJSON(&buf, dest, st, replies); err != nil {
			return nil, nil, err
		}
		// Solo es el tramo 1 si el destino está en enlace directo o es la puerta de enlace.
		if haveRoute && (!rte.NextHop.IsValid() || rte.NextHop == ip) {
			in = &probe.CombinedInput{Hop1: st}
		}
	case ProbeTrace:
		res, err := probe.Trace(ctx, env.Echoer, ip, probe.DefaultTrace())
		if err != nil {
			return nil, nil, err
		}
		if err := probe.TraceJSON(&buf, dest, res); err != nil {
			return nil, nil, err
		}
		if haveRoute && len(res.Hops) > 0 && res.Hops[0].Stats.Sent > 0 {
			in = &probe.CombinedInput{Hop1: res.Hops[0].Stats}
		}
	case ProbeMTU:
		res, err := probe.MTU(ctx, env.Echoer, ip, rte.MTU, probe.DefaultTimeout)
		if err != nil {
			return nil, nil, err
		}
		if err := probe.MTUJSON(&buf, dest, res); err != nil {
			return nil, nil, err
		}
		if res.Available && res.MTU < 1500 {
			return json.RawMessage(bytes.TrimSpace(buf.Bytes())),
				&CombinedState{Messages: []string{probe.ReducedMTUMessage(res.MTU)}}, nil
		}
	case ProbePorts:
		o := probe.DefaultPortsOptions()
		res, err := probe.Ports(ctx, env.Dialer, ip, o.Ports, o.Timeout)
		if err != nil {
			return nil, nil, err
		}
		if err := probe.PortsJSON(&buf, dest, res); err != nil {
			return nil, nil, err
		}
	}
	var cmb *CombinedState
	if in != nil {
		in.Dest = tgt.Raw
		if in.Dest == "" {
			in.Dest = ip.String()
		}
		in.Kind, in.SignalPct = a.localKind(rte.Source)
		c := probe.Diagnose(*in)
		cmb = &CombinedState{Messages: c.Messages, Healthy: c.Healthy}
	}
	return json.RawMessage(bytes.TrimSpace(buf.Bytes())), cmb, nil
}

// localKind devuelve el tipo de interfaz y la señal WiFi de la interfaz con la IP src
// ("" y nil = desconocido).
func (a *App) localKind(src netip.Addr) (netinfo.Kind, *int) {
	if a.d.Net == nil || !src.IsValid() {
		return "", nil
	}
	l := linkForIP(a.d.Net, src)
	if l == nil {
		return "", nil
	}
	var sig *int
	if l.SignalPct > 0 {
		s := l.SignalPct
		sig = &s
	}
	return netinfo.Kind(strings.TrimSpace(l.Kind)), sig
}

// CancelProbe pide cancelar y vuelve de inmediato ("Cancelando…"); no hace nada sin diagnóstico.
func (a *App) CancelProbe() {
	a.mu.Lock()
	cancel := a.probeCancel
	if a.probe.Running && a.probe.Status != ProbeCancelling {
		p := a.probe
		a.probe = ProbeState{Running: true, Status: ProbeCancelling, Kind: p.Kind, TargetID: p.TargetID, Label: "Cancelando…"}
	}
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// WaitProbe espera a que termine la goroutine del diagnóstico (hasta timeout); true si terminó o no había.
func (a *App) WaitProbe(timeout time.Duration) bool {
	a.mu.Lock()
	done := a.probeDone
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

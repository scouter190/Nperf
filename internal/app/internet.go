package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"nperf/internal/config"
	"nperf/internal/history"
	"nperf/internal/inet"
	"nperf/internal/ndt7"
	"nperf/internal/probe"
)

// Tipos de prueba hacia internet (mismo hueco de estado que los diagnósticos).
const (
	ProbeInternet = "internet"
	ProbeSpeed    = "speed"
)

// Valores de OpenPrivacy.
const (
	PrivacyMLab  = "mlab"
	PrivacyNPerf = "nperf"
)

// ErrSpeedNoConsent: StartSpeed sin consentimiento explícito.
var ErrSpeedNoConsent = errors.New("hace falta aceptar el aviso de M-Lab en cada prueba de velocidad")

// launchProbe arranca run en el hueco de diagnósticos con la misma exclusión (TEST!/uno a la vez)
// y guarda el resultado en el historial antes de publicarlo.
func (a *App) launchProbe(kind string, run func(context.Context) (json.RawMessage, *CombinedState, error),
	rec func(ProbeState) *history.Record) error {
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
	a.probe = ProbeState{Running: true, Status: ProbeRunning, Kind: kind}
	a.mu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		res, cmb, err := run(ctx)
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
			final = ProbeState{Status: ProbeCancelled, Kind: kind}
		case err != nil:
			final = ProbeState{Status: ProbeError, Kind: kind, Error: err.Error(), Result: res}
		default:
			final = ProbeState{Status: ProbeDone, Kind: kind, Result: res, Combined: cmb}
		}
		a.mu.Unlock()

		herr := a.saveRecord(rec(final))

		a.mu.Lock()
		a.histWarn = histWarning(herr)
		if a.probeGen == gen && a.probe.Running {
			a.probe = final
		}
		a.mu.Unlock()
	}()
	return nil
}

// internetRecord arma el registro de una prueba hacia internet (latencia o velocidad).
func (a *App) internetRecord(kind string, p ProbeState, params any) *history.Record {
	var selfIP string
	if a.d.SelfIPs != nil {
		if ips := a.d.SelfIPs(); len(ips) > 0 {
			selfIP = ips[0]
		}
	}
	r := &history.Record{Kind: kind, Source: endpoint(a.Alias(), selfIP, nil), Params: rawJSON(params)}
	switch p.Status {
	case ProbeDone:
		r.Status = history.StatusDone
		r.Result = p.Result
		if p.Combined != nil {
			r.Diagnosis = rawJSON(p.Combined)
		}
	case ProbeCancelled:
		r.Status = history.StatusCancelled
	default:
		r.Status = history.StatusError
		r.Error = cleanText(p.Error, 1000)
		if len(p.Result) > 0 {
			r.Result = p.Result
		}
	}
	return r
}

// StartInternet inicia la prueba de latencia hacia internet (puerta de enlace + destinos).
func (a *App) StartInternet() error {
	if a.d.Probe.Echoer == nil {
		return errors.New("los diagnósticos ICMP no están disponibles en este sistema")
	}
	cfg, _ := a.cfg()
	strs := cfg.Targets()
	targets, err := inet.ParseTargets(strs)
	if err != nil {
		return err
	}
	return a.launchProbe(ProbeInternet, func(ctx context.Context) (json.RawMessage, *CombinedState, error) {
		env := a.d.Probe
		po := a.d.Ping
		if po.Count == 0 {
			po = probe.DefaultPing()
		}
		out, err := inet.RunWith(ctx, env, a.gateway(), targets, po)
		if err != nil {
			return nil, nil, err
		}
		var buf bytes.Buffer
		if err := inet.ReportJSON(&buf, out); err != nil {
			return nil, nil, err
		}
		msgs := append([]string{out.Verdict.Message}, out.Verdict.Notes...)
		msgs = append(msgs, out.TraceNotes...)
		return json.RawMessage(bytes.TrimSpace(buf.Bytes())),
			&CombinedState{Messages: msgs, Healthy: out.Verdict.Kind == inet.Healthy}, nil
	}, func(p ProbeState) *history.Record {
		return a.internetRecord(history.KindInternet, p, map[string]any{"targets": strs})
	})
}

// StartSpeed inicia la prueba de velocidad con M-Lab. Sin consent == true no hace nada
// (ni tráfico ni consumo del límite); el consentimiento no se guarda: cada prueba lo exige.
func (a *App) StartSpeed(consent bool) error {
	if !consent {
		return ErrSpeedNoConsent
	}
	if a.d.Speed == nil {
		return errors.New("la prueba de velocidad no está disponible")
	}
	o := *a.d.Speed
	o.Consent = true
	return a.launchProbe(ProbeSpeed, func(ctx context.Context) (json.RawMessage, *CombinedState, error) {
		r, err := ndt7.Run(ctx, o)
		if err != nil {
			// El motivo (p. ej. el límite diario) va en el error del estado.
			return nil, nil, fmt.Errorf("%w", err)
		}
		b, jerr := r.JSON()
		if jerr != nil {
			return nil, nil, jerr
		}
		return json.RawMessage(bytes.TrimSpace(b)), nil, nil
	}, func(p ProbeState) *history.Record {
		return a.internetRecord(history.KindSpeed, p, map[string]string{"provider": "M-Lab NDT7"})
	})
}

// SetInternetTargets valida y guarda los destinos públicos (vacío = valores por defecto).
func (a *App) SetInternetTargets(targets []string) error {
	addrs, err := inet.ParseTargets(targets)
	if err != nil {
		return err
	}
	var strs []string
	if len(targets) > 0 {
		for _, ad := range addrs {
			strs = append(strs, ad.String())
		}
	}
	return a.update(func(c *config.Config) { c.InternetTargets = strs })
}

// InternetTargets devuelve los destinos efectivos.
func (a *App) InternetTargets() []string {
	cfg, _ := a.cfg()
	return cfg.Targets()
}

// OpenPrivacy abre una de las dos políticas de privacidad (URL fijas).
func (a *App) OpenPrivacy(which string) error {
	var url string
	switch which {
	case PrivacyMLab:
		url = ndt7.MLabPrivacyURL
	case PrivacyNPerf:
		url = ndt7.NPerfPrivacyURL
	default:
		return fmt.Errorf("política desconocida %q: use %s o %s", which, PrivacyMLab, PrivacyNPerf)
	}
	if a.d.OpenURL == nil {
		return errors.New("no se puede abrir el navegador en este sistema")
	}
	return a.d.OpenURL(url)
}

package inet

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"nperf/internal/probe"
)

// TCPTimeout es el plazo de cada conexión TCP 443 de respaldo.
const TCPTimeout = 2 * time.Second

// TCPPort es el puerto del respaldo cuando no hay respuesta ICMP.
const TCPPort = "443"

// Target es el resultado de un destino público.
type Target struct {
	Addr     netip.Addr
	Stats    probe.Stats
	TCPTried bool
	TCP443   bool
}

// Outcome es el resultado completo de la prueba de latencia.
type Outcome struct {
	Gateway      netip.Addr // inválida si no se detectó
	GatewayStats probe.Stats
	Targets      []Target
	Verdict      Result
	Trace        *probe.TraceResult // solo con veredicto "proveedor"
	TraceNotes   []string
}

// Run mide gateway (inválida = sin puerta de enlace) y targets en paralelo
// con probe.DefaultPing y calcula el veredicto. Al cancelar ctx vuelve de
// inmediato con ctx.Err().
func Run(ctx context.Context, env probe.Env, gateway netip.Addr, targets []netip.Addr) (Outcome, error) {
	return RunWith(ctx, env, gateway, targets, probe.DefaultPing())
}

// RunWith es Run con las opciones de ping indicadas.
func RunWith(ctx context.Context, env probe.Env, gateway netip.Addr, targets []netip.Addr, opts probe.PingOptions) (Outcome, error) {
	out := Outcome{Gateway: gateway, Targets: make([]Target, len(targets))}
	var wg sync.WaitGroup
	if gateway.IsValid() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.GatewayStats = pingStats(ctx, env, gateway, opts)
		}()
	}
	for i, a := range targets {
		out.Targets[i].Addr = a
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.Targets[i].Stats = pingStats(ctx, env, a, opts)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return out, err
	}

	stats := make([]probe.Stats, len(targets))
	anyICMP := false
	for i, t := range out.Targets {
		stats[i] = t.Stats
		anyICMP = anyICMP || responded(t.Stats)
	}
	tcp := make([]bool, len(targets))
	if !anyICMP && env.Dialer != nil {
		for i := range targets {
			out.Targets[i].TCPTried = true
			wg.Add(1)
			go func() {
				defer wg.Done()
				tcp[i] = dial443(ctx, env.Dialer, targets[i])
			}()
		}
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return out, err
		}
		for i := range tcp {
			out.Targets[i].TCP443 = tcp[i]
		}
	}

	out.Verdict = Verdict(out.GatewayStats, stats, tcp)
	if out.Verdict.Kind == Provider && out.Verdict.Degraded >= 0 {
		res, err := probe.Trace(ctx, env.Echoer, targets[out.Verdict.Degraded], probe.DefaultTrace())
		if cerr := ctx.Err(); cerr != nil {
			return out, cerr
		}
		if err != nil {
			out.TraceNotes = []string{"traceroute no disponible: " + err.Error()}
		} else {
			out.Trace = &res
			out.TraceNotes = probe.Analyze(res)
		}
	}
	return out, nil
}

// pingStats nunca falla: un error de envío deja las estadísticas parciales.
func pingStats(ctx context.Context, env probe.Env, dst netip.Addr, opts probe.PingOptions) probe.Stats {
	st, _, _ := probe.Ping(ctx, env, dst, opts)
	return st
}

func dial443(ctx context.Context, d probe.Dialer, a netip.Addr) bool {
	c, cancel := context.WithTimeout(ctx, TCPTimeout)
	defer cancel()
	conn, err := d.DialContext(c, "tcp", net.JoinHostPort(a.String(), TCPPort))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

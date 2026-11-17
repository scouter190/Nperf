package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"time"

	"nperf/internal/probe"
)

// probeSystem crea el entorno de sondeo; las pruebas lo sustituyen.
var probeSystem = probe.System

type flagSet struct{ fs *flag.FlagSet }

// probeCmd agrupa lo común de ping, trace, mtu y ports.
type probeCmd struct {
	name    string
	usage   string
	jsonOut *bool
}

func (c probeCmd) fail(code int, err error) int {
	if *c.jsonOut {
		probe.JSONError(stdout, err)
	} else {
		fmt.Fprintln(stderr, "error:", err)
	}
	return code
}

// splitHost separa el destino si va antes de los flags (como en status).
func splitHost(args []string) (string, []string) {
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		return args[0], args[1:]
	}
	return "", args
}

// target valida el destino y resuelve su IP; code != 0 si hay que terminar.
func (c probeCmd) target(ctx context.Context, host string, env probe.Env) (probe.Dest, int) {
	t, err := probe.ParseTarget(host)
	if err != nil {
		return probe.Dest{}, c.fail(exitUsage, err)
	}
	ip, err := probe.Resolve(ctx, t, env.Resolver)
	if err != nil {
		return probe.Dest{}, c.fail(exitError, err)
	}
	var local []netip.Prefix
	if env.Router != nil {
		local, _ = env.Router.LocalPrefixes()
	}
	if err := probe.CheckAddr(ip, local); err != nil {
		return probe.Dest{}, c.fail(exitUsage, err)
	}
	return probe.Dest{Name: t.Name, IP: ip}, 0
}

// begin analiza los flags y el destino. ok=false: terminar con code.
func (c *probeCmd) begin(args []string, fs flagSet, validate func() error) (host string, code int, ok bool) {
	host, args = splitHost(args)
	if code, done := parse(fs.fs, args); done {
		return "", code, false
	}
	if host == "" && fs.fs.NArg() > 0 {
		host = fs.fs.Arg(0)
	}
	if host == "" {
		return "", c.fail(exitUsage, fmt.Errorf("falta el destino. Uso: %s", c.usage)), false
	}
	if fs.fs.NArg() > 1 || (fs.fs.NArg() == 1 && fs.fs.Arg(0) != host) {
		return "", c.fail(exitUsage, fmt.Errorf("argumentos inesperados. Use -h para ver la ayuda")), false
	}
	if err := validate(); err != nil {
		return "", c.fail(exitUsage, err), false
	}
	return host, 0, true
}

func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}

func runProbe(ctx context.Context, cmd string, args []string) int {
	switch cmd {
	case "ping":
		return runPing(ctx, args)
	case "trace":
		return runTrace(ctx, args)
	case "mtu":
		return runMTU(ctx, args)
	default:
		return runPorts(ctx, args)
	}
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

func (c probeCmd) runErr(ctx context.Context, err error) int {
	if interrupted(ctx, err) {
		return c.fail(exitError, errors.New("cancelado por el usuario"))
	}
	return c.fail(exitError, fmt.Errorf("no se pudo completar el diagnóstico: %w", err))
}

func runPing(ctx context.Context, args []string) int {
	fs := newFlagSet("ping", "nperf ping <host> [opciones]")
	count := fs.Int("count", probe.DefaultCount, fmt.Sprintf("número de ecos, 1-%d (por defecto: %d)", probe.MaxCount, probe.DefaultCount))
	interval := fs.Duration("interval", probe.DefaultInterval, "tiempo entre ecos, mínimo 200ms (por defecto: 500ms)")
	timeout := fs.Duration("timeout", probe.DefaultTimeout, "espera por eco, 200ms-5s (por defecto: 1s)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	c := probeCmd{"ping", "nperf ping <host> [opciones]", jsonOut}
	opts := probe.PingOptions{}
	host, code, ok := c.begin(args, flagSet{fs}, func() error {
		opts = probe.PingOptions{Count: *count, Interval: *interval, Timeout: *timeout}
		return opts.Validate()
	})
	if !ok {
		return code
	}
	env, closeEnv := probeSystem()
	defer closeEnv()
	dest, code := c.target(ctx, host, env)
	if code != 0 {
		return code
	}
	st, replies, err := probe.Ping(ctx, env, dest.IP, opts)
	if err != nil {
		return c.runErr(ctx, err)
	}
	if *jsonOut {
		probe.PingJSON(stdout, dest, st, replies)
	} else {
		probe.PingText(stdout, dest, st, replies)
	}
	if st.Received == 0 {
		return exitError
	}
	return exitOK
}

func runTrace(ctx context.Context, args []string) int {
	fs := newFlagSet("trace", "nperf trace <host> [opciones]")
	hops := fs.Int("max-hops", probe.DefaultMaxHops, fmt.Sprintf("máximo de saltos, 1-%d (por defecto: %d)", probe.MaxHopsLimit, probe.DefaultMaxHops))
	timeout := fs.Duration("timeout", probe.DefaultTimeout, "espera por sonda, 200ms-5s (por defecto: 1s)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	c := probeCmd{"trace", "nperf trace <host> [opciones]", jsonOut}
	opts := probe.TraceOptions{}
	host, code, ok := c.begin(args, flagSet{fs}, func() error {
		opts = probe.TraceOptions{MaxHops: *hops, Timeout: *timeout}
		return opts.Validate()
	})
	if !ok {
		return code
	}
	env, closeEnv := probeSystem()
	defer closeEnv()
	dest, code := c.target(ctx, host, env)
	if code != 0 {
		return code
	}
	res, err := probe.Trace(ctx, env.Echoer, dest.IP, opts)
	if err != nil {
		return c.runErr(ctx, err)
	}
	if *jsonOut {
		probe.TraceJSON(stdout, dest, res)
	} else {
		probe.TraceText(stdout, dest, res)
	}
	return exitOK
}

func runMTU(ctx context.Context, args []string) int {
	fs := newFlagSet("mtu", "nperf mtu <host> [opciones]")
	timeout := fs.Duration("timeout", probe.DefaultTimeout, "espera por sonda, 200ms-5s (por defecto: 1s)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	c := probeCmd{"mtu", "nperf mtu <host> [opciones]", jsonOut}
	host, code, ok := c.begin(args, flagSet{fs}, func() error {
		return probe.TraceOptions{MaxHops: probe.MinHops, Timeout: *timeout}.Validate()
	})
	if !ok {
		return code
	}
	env, closeEnv := probeSystem()
	defer closeEnv()
	dest, code := c.target(ctx, host, env)
	if code != 0 {
		return code
	}
	ifMTU := 0
	if env.Router != nil {
		if r, err := env.Router.Route(dest.IP); err == nil {
			ifMTU = r.MTU
		}
	}
	res, err := probe.MTU(ctx, env.Echoer, dest.IP, ifMTU, *timeout)
	if err != nil {
		return c.runErr(ctx, err)
	}
	if *jsonOut {
		probe.MTUJSON(stdout, dest, res)
	} else {
		probe.MTUText(stdout, dest, res)
	}
	return exitOK
}

func runPorts(ctx context.Context, args []string) int {
	fs := newFlagSet("ports", "nperf ports <host> [opciones]")
	list := fs.String("p", "", "puertos TCP: lista y rangos, p. ej. 22,80,8000-8010 (máx. 32; por defecto: 21,22,23,53,80,443,445,3389,8080,8291)")
	timeout := fs.Duration("timeout", probe.DefaultPortsTimeout, "espera por puerto, 200ms-5s (por defecto: 1.5s)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	c := probeCmd{"ports", "nperf ports <host> [opciones]", jsonOut}
	opts := probe.DefaultPortsOptions()
	host, code, ok := c.begin(args, flagSet{fs}, func() error {
		if *list != "" {
			p, err := probe.ParsePorts(*list)
			if err != nil {
				return err
			}
			opts.Ports = p
		}
		opts.Timeout = *timeout
		return opts.Validate()
	})
	if !ok {
		return code
	}
	env, closeEnv := probeSystem()
	defer closeEnv()
	dest, code := c.target(ctx, host, env)
	if code != 0 {
		return code
	}
	res, err := probe.Ports(ctx, env.Dialer, dest.IP, opts.Ports, opts.Timeout)
	if err != nil {
		return c.runErr(ctx, err)
	}
	if *jsonOut {
		probe.PortsJSON(stdout, dest, res)
	} else {
		probe.PortsText(stdout, dest, res)
	}
	return exitOK
}

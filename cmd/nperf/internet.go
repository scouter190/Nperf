package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"nperf/internal/config"
	"nperf/internal/history"
	"nperf/internal/inet"
	"nperf/internal/ndt7"
	"nperf/internal/probe"
	"nperf/internal/report"
	"nperf/internal/version"
)

// internetEnv agrupa las dependencias de `nperf internet`; las pruebas la sustituyen.
type internetEnv struct {
	Probe    func() (probe.Env, func())
	Ping     probe.PingOptions                     // opciones de ping de la latencia
	Speed    func(quotaDir string) ndt7.RunOptions // opciones base de ndt7.Run (sin Consent ni Quota)
	Stdin    io.Reader
	Terminal func() bool // true si la entrada estándar es una terminal
}

var internetSystem = func() internetEnv {
	return internetEnv{
		Probe: probeSystem,
		Ping:  probe.DefaultPing(),
		Speed: func(string) ndt7.RunOptions { return ndt7.RunOptions{} },
		Stdin: os.Stdin,
		Terminal: func() bool {
			st, err := os.Stdin.Stat()
			return err == nil && st.Mode()&os.ModeCharDevice != 0
		},
	}
}

// internetCacheDir es el directorio base del historial (%LocalAppData% en Windows).
var internetCacheDir = os.UserCacheDir

func runInternet(ctx context.Context, args []string) int {
	fs := newFlagSet("internet", "nperf internet [opciones]")
	speed := fs.Bool("speed", false, "añade la prueba de velocidad con M-Lab NDT7 (envía tráfico a un servidor público; pide consentimiento)")
	accept := fs.Bool("accept-mlab", false, "acepta el aviso de M-Lab sin preguntar (solo con -speed)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1) por la salida estándar; los errores salen como {\"error\": \"...\"}")
	save := fs.Bool("save", false, "guarda el resultado en el historial")
	if code, done := parse(fs, args); done {
		return code
	}
	fail := func(code int, err error) int {
		if *jsonOut {
			report.JSONError(stdout, err)
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return code
	}
	if fs.NArg() > 0 {
		return fail(exitUsage, fmt.Errorf("argumento inesperado %q. Use -h para ver la ayuda", fs.Arg(0)))
	}
	if *accept && !*speed {
		return fail(exitUsage, errors.New("-accept-mlab solo aplica con -speed"))
	}
	cfg := config.Defaults()
	if dir, err := configDir(); err == nil {
		c, err := config.Load(dir)
		if err != nil {
			return fail(exitUsage, err)
		}
		cfg = c
	}
	targets, err := inet.ParseTargets(cfg.Targets())
	if err != nil {
		return fail(exitUsage, fmt.Errorf("internet_targets de config.json: %w", err))
	}
	env := internetSystem()

	// Consentimiento antes de cualquier tráfico.
	if *speed {
		fmt.Fprintln(stderr, ndt7.Notice())
		if !*accept {
			if !env.Terminal() {
				return fail(exitUsage, errors.New("no hay terminal para pedir el consentimiento de M-Lab; use -accept-mlab para aceptarlo. No se envió tráfico"))
			}
			fmt.Fprint(stderr, "¿Acepta y quiere hacer la prueba de velocidad? [s/N]: ")
			line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
			switch strings.ToLower(strings.TrimSpace(line)) {
			case "s", "si", "sí":
			default:
				return fail(exitError, errors.New("prueba de velocidad no aceptada. No se envió tráfico"))
			}
		}
	}

	pe, closeEnv := env.Probe()
	defer closeEnv()
	var gw netip.Addr
	if pe.Router != nil {
		gw, _ = pe.Router.Gateway()
	}
	out, err := inet.RunWith(ctx, pe, gw, targets, env.Ping)
	if err != nil {
		if interrupted(ctx, err) {
			return fail(exitError, errors.New("cancelado por el usuario"))
		}
		return fail(exitError, fmt.Errorf("no se pudo completar la prueba: %w", err))
	}
	var sres ndt7.Result
	if *speed {
		if cacheBase() == "" {
			return fail(exitError, errors.New("no se pudo determinar el directorio de datos para el límite diario"))
		}
		qdir := filepath.Join(cacheBase(), "NPerf")
		o := env.Speed(qdir)
		o.Consent = true
		o.Quota = &ndt7.Quota{Dir: qdir}
		sres, _ = ndt7.Run(ctx, o) // el fallo queda en el Result como "no disponible" con motivo
		if ctx.Err() != nil {
			return fail(exitError, errors.New("cancelado por el usuario"))
		}
	}

	var latJSON jsonBuf
	if err := inet.ReportJSON(&latJSON, out); err != nil {
		return fail(exitError, err)
	}
	var spJSON []byte
	if *speed {
		if spJSON, err = sres.JSON(); err != nil {
			return fail(exitError, err)
		}
	}
	if *jsonOut {
		b := []byte(latJSON.String())
		if *speed {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				return fail(exitError, err)
			}
			m["speed"] = spJSON
			if b, err = json.MarshalIndent(m, "", "  "); err != nil {
				return fail(exitError, err)
			}
		}
		fmt.Fprintln(stdout, strings.TrimRight(string(b), "\n"))
	} else {
		inet.ReportText(stdout, out)
		if *speed {
			fmt.Fprint(stdout, "\n"+sres.Text())
		}
	}

	if *save {
		return saveInternet(cfg, targets, latJSON.String(), spJSON, *speed, sres)
	}
	return exitOK
}

func cacheBase() string {
	d, err := internetCacheDir()
	if err != nil {
		return ""
	}
	return d
}

type jsonBuf struct{ strings.Builder }

func saveInternet(cfg config.Config, targets []netip.Addr, latJSON string, spJSON []byte, speed bool, sres ndt7.Result) int {
	base := cacheBase()
	st, err := history.Open(history.DefaultDir(base), nil, cfg.HistoryOn())
	if err != nil {
		fmt.Fprintln(stderr, "error: no se pudo guardar en el historial:", err)
		return exitError
	}
	if !st.Enabled {
		fmt.Fprintln(stderr, "aviso: el historial está desactivado; no se guardó el resultado")
		return exitOK
	}
	ts := make([]string, len(targets))
	for i, t := range targets {
		ts[i] = t.String()
	}
	params, _ := json.Marshal(map[string]any{"targets": ts})
	recs := []*history.Record{{
		Kind: history.KindInternet, Status: history.StatusDone, Version: version.Current,
		Params: params, Result: json.RawMessage(latJSON),
	}}
	if speed {
		r := &history.Record{
			Kind: history.KindSpeed, Status: history.StatusDone, Version: version.Current,
			Params: json.RawMessage(`{"provider":"M-Lab NDT7"}`), Result: spJSON,
		}
		if sres.Download.Direction == nil && sres.Upload.Direction == nil {
			r.Status = history.StatusError
			r.Error = sres.Download.Reason
		}
		recs = append(recs, r)
	}
	for _, r := range recs {
		id, err := st.Save(r)
		if err != nil {
			fmt.Fprintln(stderr, "error: no se pudo guardar en el historial:", err)
			return exitError
		}
		fmt.Fprintf(stderr, "guardado en el historial: %s\n", id)
	}
	return exitOK
}

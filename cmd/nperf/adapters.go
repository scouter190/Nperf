package main

import (
	"fmt"

	"nperf/internal/netinfo"
	"nperf/internal/report"
)

// adaptersSource es la fuente de datos de red; las pruebas la sustituyen.
var adaptersSource = netinfo.System

func runAdapters(args []string) int {
	fs := newFlagSet("adapters", "nperf adapters [opciones]")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1) por la salida estándar; los errores salen como {\"error\": \"...\"}")
	if code, done := parse(fs, args); done {
		return code
	}
	if fs.NArg() > 0 {
		err := fmt.Errorf("argumento inesperado %q. Use -h para ver la ayuda", fs.Arg(0))
		if *jsonOut {
			report.JSONError(stdout, err)
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return exitUsage
	}
	infos, err := netinfo.Inventory(adaptersSource())
	if err != nil {
		err = fmt.Errorf("no se pudo leer la información de red: %w", err)
		if *jsonOut {
			report.JSONError(stdout, err)
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return exitError
	}
	if *jsonOut {
		b, err := netinfo.JSON(infos)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitError
		}
		fmt.Fprintln(stdout, string(b))
		return exitOK
	}
	fmt.Fprint(stdout, netinfo.BuildView(infos, true).Text())
	return exitOK
}

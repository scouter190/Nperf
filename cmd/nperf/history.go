package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"nperf/internal/export"
	"nperf/internal/history"
	"nperf/internal/report"
)

// historyDir devuelve la carpeta del historial; las pruebas la sustituyen.
var historyDir = func() string {
	d, err := os.UserCacheDir() // %LocalAppData% en Windows
	if err != nil {
		return ""
	}
	return history.DefaultDir(d)
}

// historyNow es el reloj del informe HTML; las pruebas lo sustituyen.
var historyNow = time.Now

const historyUsage = "nperf history list|show|export [opciones]"

const historyHelp = `Uso:
  nperf history list [-kind tipo] [-host equipo] [-since fecha] [-json]
  nperf history show <id> [-json]
  nperf history export <id...>|-all -format csv|json|html [-csv std|excel-es] -o <ruta>

Tipos: test, ping, trace, mtu, ports, internet, speed.
Fechas: AAAA-MM-DD o RFC 3339 (hora local si no lleva zona).
`

// historyFail imprime el error (JSON o texto) y devuelve el código.
func historyFail(asJSON bool, code int, err error) int {
	if asJSON {
		report.JSONError(stdout, err)
	} else {
		fmt.Fprintln(stderr, "error:", err)
	}
	return code
}

func runHistory(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, historyHelp)
		return exitUsage
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, historyHelp)
		return exitOK
	case "list":
		return runHistoryList(args[1:])
	case "show":
		return runHistoryShow(args[1:])
	case "export":
		return runHistoryExport(args[1:])
	}
	fmt.Fprintf(stderr, "subcomando de history desconocido: %s\n\n%s", args[0], historyHelp)
	return exitUsage
}

func openHistoryDir() (string, error) {
	d := historyDir()
	if d == "" {
		return "", errors.New("no se pudo determinar la carpeta del historial")
	}
	return d, nil
}

func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("fecha no válida %q: use AAAA-MM-DD o RFC 3339", s)
}

func validKind(k string) bool {
	switch k {
	case history.KindTest, history.KindPing, history.KindTrace, history.KindMTU,
		history.KindPorts, history.KindInternet, history.KindSpeed:
		return true
	}
	return false
}

func endpointLabel(e *history.Endpoint) string {
	if e == nil {
		return "-"
	}
	switch {
	case e.Alias != "" && e.IP != "":
		return e.Alias + " (" + e.IP + ")"
	case e.Alias != "":
		return e.Alias
	case e.IP != "":
		return e.IP
	}
	return "-"
}

type listItem struct {
	ID     string            `json:"id"`
	Time   string            `json:"time"`
	Kind   string            `json:"kind"`
	Status string            `json:"status"`
	Source *history.Endpoint `json:"source,omitempty"`
	Dest   *history.Endpoint `json:"dest,omitempty"`
}

func runHistoryList(args []string) int {
	fs := newFlagSet("history list", "nperf history list [opciones]")
	kind := fs.String("kind", "", "solo este tipo: test, ping, trace, mtu, ports, internet, speed")
	host := fs.String("host", "", "solo registros con este alias o IP de origen o destino")
	since := fs.String("since", "", "solo desde esta fecha (AAAA-MM-DD o RFC 3339)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	if code, done := parse(fs, args); done {
		return code
	}
	if fs.NArg() > 0 {
		return historyFail(*jsonOut, exitUsage, errors.New("argumentos inesperados. Use -h para ver la ayuda"))
	}
	f := history.Filter{Host: *host, Kind: *kind}
	if *kind != "" && !validKind(*kind) {
		return historyFail(*jsonOut, exitUsage, fmt.Errorf("tipo desconocido %q", *kind))
	}
	if *since != "" {
		t, err := parseSince(*since)
		if err != nil {
			return historyFail(*jsonOut, exitUsage, err)
		}
		f.Since = t
	}
	dir, err := openHistoryDir()
	if err != nil {
		return historyFail(*jsonOut, exitError, err)
	}
	res, err := history.ListDir(dir, f)
	if err != nil {
		return historyFail(*jsonOut, exitError, fmt.Errorf("no se pudo leer el historial: %w", err))
	}
	if *jsonOut {
		items := make([]listItem, 0, len(res.Records))
		for _, r := range res.Records {
			items = append(items, listItem{r.ID, r.Time, r.Kind, r.Status, r.Source, r.Dest})
		}
		return writeHistoryJSON(map[string]any{
			"format_version": 1, "command": "history list", "records": items, "skipped": res.Skipped,
		})
	}
	if len(res.Records) == 0 {
		fmt.Fprintln(stdout, "No hay registros en el historial.")
	}
	for _, r := range res.Records {
		fmt.Fprintf(stdout, "%s  %s  %-8s %-9s %s -> %s\n", r.ID, r.Time, r.Kind, r.Status,
			endpointLabel(r.Source), endpointLabel(r.Dest))
	}
	if res.Skipped > 0 {
		fmt.Fprintf(stderr, "aviso: %d archivo(s) del historial no se pudieron leer y se omitieron\n", res.Skipped)
	}
	return exitOK
}

func writeHistoryJSON(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", b); err != nil {
		return exitError
	}
	return exitOK
}

// getRecord lee un registro: id inválido → uso (2); inexistente o ilegible → 1.
func getRecord(dir, id string) (*history.Record, int, error) {
	if !history.ValidID(id) {
		return nil, exitUsage, fmt.Errorf("id no válido: %q", id)
	}
	r, err := history.GetDir(dir, id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, exitError, fmt.Errorf("no existe el registro %s", id)
		}
		return nil, exitError, fmt.Errorf("no se pudo leer el registro %s: %w", id, err)
	}
	return r, 0, nil
}

func runHistoryShow(args []string) int {
	fs := newFlagSet("history show", "nperf history show <id> [-json]")
	jsonOut := fs.Bool("json", false, "imprime solo el registro como JSON (formato v1); los errores salen como {\"error\": \"...\"}")
	id, rest := splitHost(args)
	if code, done := parse(fs, rest); done {
		return code
	}
	if id == "" && fs.NArg() > 0 {
		id = fs.Arg(0)
	}
	if id == "" {
		return historyFail(*jsonOut, exitUsage, errors.New("falta el id. Uso: nperf history show <id>"))
	}
	if fs.NArg() > 1 || (fs.NArg() == 1 && fs.Arg(0) != id) {
		return historyFail(*jsonOut, exitUsage, errors.New("argumentos inesperados. Use -h para ver la ayuda"))
	}
	if !history.ValidID(id) {
		return historyFail(*jsonOut, exitUsage, fmt.Errorf("id no válido: %q", id))
	}
	dir, err := openHistoryDir()
	if err != nil {
		return historyFail(*jsonOut, exitError, err)
	}
	r, code, err := getRecord(dir, id)
	if err != nil {
		return historyFail(*jsonOut, code, err)
	}
	if *jsonOut {
		return writeHistoryJSON(map[string]any{"format_version": 1, "command": "history show", "record": r})
	}
	fmt.Fprintf(stdout, "ID:      %s\nFecha:   %s\nTipo:    %s\nEstado:  %s\n", r.ID, r.Time, r.Kind, r.Status)
	if r.Error != "" {
		fmt.Fprintf(stdout, "Error:   %s\n", r.Error)
	}
	if r.Version != "" {
		fmt.Fprintf(stdout, "Versión: %s\n", r.Version)
	}
	for _, e := range []struct {
		name string
		e    *history.Endpoint
	}{{"Origen", r.Source}, {"Destino", r.Dest}} {
		if e.e == nil {
			continue
		}
		line := fmt.Sprintf("%-8s %s", e.name+":", endpointLabel(e.e))
		if e.e.SSID != "" {
			line += ", WiFi " + e.e.SSID
		}
		if e.e.Link != "" {
			line += ", " + e.e.Link
		}
		if e.e.NominalMbps > 0 {
			line += fmt.Sprintf(", %.0f Mbps nominal", e.e.NominalMbps)
		}
		fmt.Fprintln(stdout, line)
	}
	for _, n := range r.Notes {
		fmt.Fprintln(stdout, "Nota:   ", n)
	}
	if len(r.Result) > 0 {
		var buf strings.Builder
		var v any
		if json.Unmarshal(r.Result, &v) == nil {
			if b, err := json.MarshalIndent(v, "  ", "  "); err == nil {
				buf.Write(b)
			}
		}
		fmt.Fprintf(stdout, "Resultado:\n  %s\n", buf.String())
	}
	return exitOK
}

func runHistoryExport(args []string) int {
	fs := newFlagSet("history export", "nperf history export <id...>|-all -format csv|json|html [-csv std|excel-es] -o <ruta>")
	all := fs.Bool("all", false, "exporta todos los registros")
	format := fs.String("format", "", "formato: csv, json o html")
	csvFmt := fs.String("csv", "std", "variante de CSV: std (coma y punto) o excel-es (punto y coma y coma decimal)")
	out := fs.String("o", "", "archivo de salida")
	// Los ids pueden ir antes, entre o después de los flags.
	var ids []string
	rest := args
	for {
		if code, done := parse(fs, rest); done {
			return code
		}
		if fs.NArg() == 0 {
			break
		}
		ids = append(ids, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	fail := func(code int, err error) int { return historyFail(false, code, err) }
	switch *format {
	case "csv", "json", "html":
	case "":
		return fail(exitUsage, errors.New("falta -format (csv, json o html)"))
	default:
		return fail(exitUsage, fmt.Errorf("formato desconocido %q: use csv, json o html", *format))
	}
	var cf export.CSVFormat
	switch *csvFmt {
	case "std":
		cf = export.CSVStd
	case "excel-es":
		cf = export.CSVExcelES
	default:
		return fail(exitUsage, fmt.Errorf("variante de CSV desconocida %q: use std o excel-es", *csvFmt))
	}
	if *out == "" {
		return fail(exitUsage, errors.New("falta -o <ruta>"))
	}
	if *all == (len(ids) > 0) {
		return fail(exitUsage, errors.New("indique uno o más ids, o -all (no ambos)"))
	}
	for _, id := range ids {
		if !history.ValidID(id) {
			return fail(exitUsage, fmt.Errorf("id no válido: %q", id))
		}
	}
	dir, err := openHistoryDir()
	if err != nil {
		return fail(exitError, err)
	}
	var recs []*history.Record
	if *all {
		res, err := history.ListDir(dir, history.Filter{})
		if err != nil {
			return fail(exitError, fmt.Errorf("no se pudo leer el historial: %w", err))
		}
		recs = res.Records
		if len(recs) == 0 {
			return fail(exitError, errors.New("no hay registros que exportar"))
		}
	} else {
		for _, id := range ids {
			r, code, err := getRecord(dir, id)
			if err != nil {
				return fail(code, err)
			}
			recs = append(recs, r)
		}
	}
	var data []byte
	switch *format {
	case "csv":
		data, err = export.SummaryCSV(recs, cf)
	case "html":
		data, err = export.ReportHTML(recs, export.ReportOptions{}, historyNow())
	default:
		data, err = json.MarshalIndent(map[string]any{
			"format_version": 1, "command": "history export", "records": recs,
		}, "", "  ")
		data = append(data, '\n')
	}
	if err != nil {
		return fail(exitError, fmt.Errorf("no se pudo generar la exportación: %w", err))
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		return fail(exitError, fmt.Errorf("no se pudo escribir %s: %w", *out, err))
	}
	fmt.Fprintf(stderr, "Exportados %d registro(s) en %s\n", len(recs), *out)
	return exitOK
}

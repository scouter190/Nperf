package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nperf/internal/export"
	"nperf/internal/history"
)

// Formatos de ExportHistory. "csv" usa excel-es (la ventana); "csv-std" usa ',' y punto.
const (
	ExportCSV    = "csv"
	ExportCSVStd = "csv-std"
	ExportJSON   = "json"
	ExportHTML   = "html"
)

// ExportOpts son los campos opcionales del informe y la elección de intervalos (CSV).
type ExportOpts struct {
	Client     string `json:"client"`
	Technician string `json:"technician"`
	Notes      string `json:"notes"`
	Intervals  bool   `json:"intervals"`
}

// IntervalsPath devuelve la ruta del segundo CSV: junto al principal, sufijo -intervalos.
func IntervalsPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-intervalos" + ext
}

// writeFileAtomic escribe en un temporal del mismo directorio y renombra sobre path.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nperf-export-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// ExportHistory exporta los registros indicados a path (ruta elegida por el usuario). Con CSV
// y opts.Intervals escribe además IntervalsPath(path). Devuelve las rutas escritas.
func (a *App) ExportHistory(ids []string, format string, opts ExportOpts, path string) ([]string, error) {
	if a.d.History == nil {
		return nil, errors.New("historial no disponible")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("ruta de destino vacía")
	}
	if len(ids) == 0 {
		return nil, errors.New("no hay registros que exportar")
	}
	recs := make([]*history.Record, 0, len(ids))
	for _, id := range ids {
		if !history.ValidID(id) {
			return nil, fmt.Errorf("identificador no válido: %q", cleanText(id, 40))
		}
		r, err := a.d.History.Get(id)
		if err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	var data, second []byte
	var err error
	switch format {
	case ExportCSV, ExportCSVStd:
		f := export.CSVExcelES
		if format == ExportCSVStd {
			f = export.CSVStd
		}
		if data, err = export.SummaryCSV(recs, f); err == nil && opts.Intervals {
			second, err = export.IntervalsCSV(recs, f)
		}
	case ExportJSON:
		data, err = json.MarshalIndent(map[string]any{
			"format_version": 1, "command": "history export", "records": recs}, "", "  ")
	case ExportHTML:
		data, err = export.ReportHTML(recs, export.ReportOptions{Client: opts.Client,
			Technician: opts.Technician, Notes: opts.Notes}, time.Now())
	default:
		return nil, fmt.Errorf("formato no válido: %q", cleanText(format, 40))
	}
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return nil, err
	}
	written := []string{path}
	if second != nil {
		p2 := IntervalsPath(path)
		if err := writeFileAtomic(p2, second); err != nil {
			return written, err
		}
		written = append(written, p2)
	}
	return written, nil
}

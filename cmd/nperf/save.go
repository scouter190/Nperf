package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"nperf/internal/config"
	"nperf/internal/history"
	"nperf/internal/version"
)

// saveHelp es la ayuda común de -save.
const saveHelp = "guarda el resultado en el historial (sin este flag no se guarda nada)"

// saveHistory guarda un registro de un comando con -save. Nunca cambia el código de
// salida: los fallos y el historial desactivado solo avisan por stderr.
// result es el JSON v1 del comando (objeto); host es el destino.
func saveHistory(kind, host string, params any, result []byte) {
	warn := func(format string, a ...any) { fmt.Fprintf(stderr, format+"\n", a...) }
	base := cacheBase()
	if base == "" {
		warn("aviso: no se pudo determinar la carpeta del historial; no se guardó el resultado")
		return
	}
	on := true
	if dir, err := configDir(); err == nil {
		if c, err := config.Load(dir); err == nil {
			on = c.HistoryOn()
		}
	}
	st, err := history.Open(history.DefaultDir(base), nil, on)
	if err != nil {
		warn("aviso: no se pudo guardar en el historial: %v", err)
		return
	}
	if !st.Enabled {
		warn("aviso: el historial está desactivado; no se guardó el resultado")
		return
	}
	p, _ := json.Marshal(params)
	r := &history.Record{
		Kind: kind, Status: history.StatusDone, Version: version.Current,
		Dest:   &history.Endpoint{Alias: host},
		Params: p, Result: bytes.TrimSpace(result),
	}
	id, err := st.Save(r)
	if err != nil {
		warn("aviso: no se pudo guardar en el historial: %v", err)
		return
	}
	fmt.Fprintf(stderr, "guardado en el historial: %s\n", id)
}

// emitJSON genera el JSON con fn; lo imprime en stdout si print y lo devuelve.
func emitJSON(w io.Writer, print bool, fn func(io.Writer) error) ([]byte, error) {
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		return nil, err
	}
	if print {
		if _, err := w.Write(buf.Bytes()); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

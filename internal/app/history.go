package app

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"nperf/internal/config"
	"nperf/internal/history"
	"nperf/internal/proto"
	"nperf/internal/report"
	"nperf/internal/version"
)

// HistoryWarningText se muestra (State.HistoryWarning) si no se pudo guardar un resultado.
const HistoryWarningText = "no se pudo guardar en el historial"

// cleanText deja texto válido para un registro: sin controles y con tope en bytes.
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	if len(s) > max {
		s = s[:max]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func endpoint(alias, ip string, l *proto.Link) *history.Endpoint {
	e := &history.Endpoint{Alias: cleanText(alias, 200), IP: cleanText(ip, 200)}
	if l != nil {
		e.SSID = cleanText(l.SSID, 200)
		e.Link = cleanText(l.Kind, 200)
		if l.TxBps > 0 {
			e.NominalMbps = float64(l.TxBps) / 1e6
		}
	}
	return e
}

func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// saveRecord guarda r si el historial está activado. Un fallo se devuelve para avisar;
// desactivado o sin almacén no es fallo.
func (a *App) saveRecord(r *history.Record) error {
	if a.d.History == nil {
		return nil
	}
	a.histMu.Lock()
	defer a.histMu.Unlock()
	if !a.d.History.Enabled {
		return nil
	}
	r.Version = version.Current
	_, err := a.d.History.Save(r)
	return err
}

// histWarning devuelve el aviso a publicar según el resultado del guardado.
func histWarning(err error) string {
	if err != nil && !errors.Is(err, history.ErrDisabled) {
		return HistoryWarningText
	}
	return ""
}

// testRecord arma el registro de un TEST! con el mismo estado que se publica (llamar con a.mu
// tomado: la gráfica la mutan los callbacks bajo ese candado).
func (a *App) testRecord(t TestState, rp report.JSONParams, src, dst *history.Endpoint) *history.Record {
	r := &history.Record{Kind: history.KindTest, Source: src, Dest: dst, Params: rawJSON(rp)}
	switch t.Status {
	case TestDone:
		r.Status = history.StatusDone
	case TestCancelled:
		r.Status = history.StatusCancelled
	default:
		r.Status = history.StatusError
		r.Error = cleanText(t.Error, 1000)
	}
	if t.Report != nil {
		r.Result = rawJSON(t.Report)
		r.Notes = t.Report.Notes
	}
	if t.Diagnosis != nil {
		r.Diagnosis = rawJSON(t.Diagnosis)
	}
	if t.Chart != nil {
		r.Chart = rawJSON(t.Chart)
	}
	return r
}

// History lista los registros guardados (también con el historial desactivado).
func (a *App) History(f history.Filter) (history.ListResult, error) {
	if a.d.History == nil {
		return history.ListResult{}, nil
	}
	return a.d.History.List(f)
}

// HistoryRecord lee un registro por id.
func (a *App) HistoryRecord(id string) (*history.Record, error) {
	if a.d.History == nil {
		return nil, errors.New("historial no disponible")
	}
	return a.d.History.Get(id)
}

// DeleteHistory borra los registros indicados.
func (a *App) DeleteHistory(ids []string) (int, error) {
	if a.d.History == nil {
		return 0, errors.New("historial no disponible")
	}
	return a.d.History.Delete(ids)
}

// DeleteAllHistory borra todos los registros.
func (a *App) DeleteAllHistory() (int, error) {
	if a.d.History == nil {
		return 0, errors.New("historial no disponible")
	}
	return a.d.History.DeleteAll()
}

// SetHistoryEnabled activa o desactiva el guardado y lo persiste en la configuración.
func (a *App) SetHistoryEnabled(on bool) error {
	if a.d.ConfigDir != "" {
		if err := a.update(func(c *config.Config) { c.HistoryEnabled = &on }); err != nil {
			return err
		}
	}
	if a.d.History != nil {
		a.histMu.Lock()
		a.d.History.Enabled = on
		a.histMu.Unlock()
	}
	return nil
}

// initHistory aplica la configuración al almacén y la retención al arrancar.
func (a *App) initHistory(c config.Config, haveCfg bool) {
	if a.d.History == nil {
		return
	}
	a.histMu.Lock()
	defer a.histMu.Unlock()
	if haveCfg {
		a.d.History.Enabled = c.HistoryOn()
	}
	a.d.History.Prune()
}

// historyOn indica si se guardará el próximo resultado.
func (a *App) historyOn(c config.Config) bool {
	if a.d.History == nil {
		return false
	}
	a.histMu.Lock()
	defer a.histMu.Unlock()
	return a.d.History.Enabled
}

// probeRecord arma el registro de un diagnóstico terminado.
func (a *App) probeRecord(p ProbeState, target string) *history.Record {
	kind := map[string]string{ProbePing: history.KindPing, ProbeTrace: history.KindTrace,
		ProbeMTU: history.KindMTU, ProbePorts: history.KindPorts}[p.Kind]
	alias, ip := "", ""
	cfg, _ := a.cfg()
	for _, d := range a.noAgentDevices(cfg) {
		if d.ID == p.TargetID && len(d.IPs) > 0 {
			alias, ip = d.Alias, d.IPs[0]
		}
	}
	if alias == "" {
		for _, pe := range a.d.Table.Snapshot() {
			if pe.InstanceID == p.TargetID {
				alias, ip = pe.Alias, pe.IP.String()
			}
		}
	}
	var selfIP string
	if a.d.SelfIPs != nil {
		if ips := a.d.SelfIPs(); len(ips) > 0 {
			selfIP = ips[0]
		}
	}
	r := &history.Record{Kind: kind, Source: endpoint(a.Alias(), selfIP, nil), Dest: endpoint(alias, ip, nil),
		Params: rawJSON(map[string]string{"target": cleanText(target, 200)})}
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
	}
	return r
}

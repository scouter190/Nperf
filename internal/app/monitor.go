package app

import (
	"nperf/internal/netinfo"
)

// MonitorPoint es la tasa media de un instante (bits por segundo).
type MonitorPoint struct {
	AtMs   int64   `json:"at_ms"` // milisegundos Unix
	InBps  float64 `json:"in_bps"`
	OutBps float64 `json:"out_bps"`
	Gap    bool    `json:"gap,omitempty"`
}

// MonitorAdapter es la serie de una tarjeta física.
type MonitorAdapter struct {
	GUID   string         `json:"guid"`
	Name   string         `json:"name"`
	Kind   string         `json:"kind"`
	Status string         `json:"status"` // up | disconnected
	Points []MonitorPoint `json:"points"`
}

// MonitorView es la vista del monitor para la interfaz.
type MonitorView struct {
	Available bool             `json:"available"`
	Message   string           `json:"message,omitempty"`
	Adapters  []MonitorAdapter `json:"adapters"`
}

// monitor devuelve el monitor, creándolo si hace falta; nil si no hay fuentes.
func (a *App) monitor() *netinfo.Monitor {
	if a.d.Counters == nil || a.d.Net == nil {
		return nil
	}
	a.monMu.Lock()
	defer a.monMu.Unlock()
	if a.mon == nil {
		a.mon = netinfo.NewMonitor(a.d.Counters, a.d.Net.Inventory, a.d.Now)
	}
	return a.mon
}

// Monitor con active=true renueva el permiso, toma una muestra y devuelve la vista;
// con false detiene el muestreo y devuelve una vista vacía. Sin fuente de contadores
// responde "no disponible" sin error.
func (a *App) Monitor(active bool) (MonitorView, error) {
	m := a.monitor()
	if m == nil {
		return MonitorView{Message: "el monitor de tarjetas no está disponible", Adapters: []MonitorAdapter{}}, nil
	}
	if !active {
		m.Stop()
		return MonitorView{Available: true, Adapters: []MonitorAdapter{}}, nil
	}
	m.Touch()
	m.Step(a.d.Now())
	return monitorView(m.Snapshot()), nil
}

// StopMonitor detiene el muestreo (p. ej. al ocultar la ventana).
func (a *App) StopMonitor() {
	a.monMu.Lock()
	m := a.mon
	a.monMu.Unlock()
	if m != nil {
		m.Stop()
	}
}

func monitorView(ss []netinfo.MonitorSeries) MonitorView {
	v := MonitorView{Available: true, Adapters: make([]MonitorAdapter, 0, len(ss))}
	for _, s := range ss {
		ad := MonitorAdapter{GUID: s.GUID, Name: s.Name, Kind: string(s.Kind), Status: s.Status,
			Points: make([]MonitorPoint, 0, len(s.Points))}
		for _, p := range s.Points {
			ad.Points = append(ad.Points, MonitorPoint{AtMs: p.At.UnixMilli(), InBps: p.InBps, OutBps: p.OutBps, Gap: p.Gap})
		}
		v.Adapters = append(v.Adapters, ad)
	}
	return v
}

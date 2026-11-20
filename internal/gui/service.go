package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nperf/internal/app"
	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/history"
	"nperf/internal/ndt7"
	"nperf/internal/netinfo"
	"nperf/internal/probe"
	"nperf/internal/version"
	"nperf/internal/ws"
)

// localAppData devuelve %LocalAppData% ("" si no existe); variable para pruebas.
var localAppData = func() string { return os.Getenv("LOCALAPPDATA") }

// historyAndSpeed crea el almacén del historial (activado según la configuración) y las
// opciones de la prueba M-Lab con la cuota en %LocalAppData%\NPerf. Sin LocalAppData
// devuelve nil en ambos (las funciones quedan "no disponibles"). Locate y WS usan la
// red real: sin proxy y con la verificación TLS del sistema.
func historyAndSpeed(configDir string) (*history.Store, *ndt7.RunOptions) {
	la := localAppData()
	if la == "" {
		return nil, nil
	}
	on := true
	if c, err := config.LoadFull(configDir); err == nil {
		on = c.HistoryOn()
	}
	st, err := history.Open(history.DefaultDir(la), nil, on)
	if err != nil {
		st = nil
	}
	return st, &ndt7.RunOptions{
		Quota: &ndt7.Quota{Dir: filepath.Join(la, "NPerf")},
		WS:    ws.Options{UserAgent: "NPerf/" + version.Current},
	}
}

// OpenURL abre una URL en el sistema; lo asigna la implementación de cada plataforma
// (open_windows.go, T14). nil = no disponible.
var OpenURL func(string) error

// netTTL es la vigencia de la caché del inventario de red.
const netTTL = 5 * time.Second

// StartFunc arranca el runtime (app.Start; inyectable en pruebas).
type StartFunc func(context.Context, app.RuntimeOptions) (*app.Runtime, error)

// Service une App y Runtime: arranca el runtime y lo reinicia (Stop+Start) cuando
// cambia la configuración. La tabla de equipos y el id de instancia se conservan.
type Service struct {
	App *app.App

	ctx        context.Context
	dir        string
	base       func() app.RuntimeOptions // opciones base (puertos, inyección); nil = vacías
	start      StartFunc
	table      *discovery.Table
	net        app.InventorySource
	mu         sync.Mutex // serializa Start/Stop/Restart
	stoped     bool
	rtMu       sync.Mutex // protege rt (State lo lee sin esperar a un reinicio)
	rt         *app.Runtime
	closeProbe func() // libera el Env de los diagnósticos
}

// NewService crea App y arranca el runtime. Si algo falla (config inválida, puerto
// ocupado) devuelve el error y no deja nada abierto. start nil = app.Start.
func NewService(ctx context.Context, configDir string, base func() app.RuntimeOptions, start StartFunc) (*Service, error) {
	if start == nil {
		start = app.Start
	}
	id, err := discovery.NewInstanceID()
	if err != nil {
		return nil, fmt.Errorf("no se pudo generar el id de instancia: %w", err)
	}
	s := &Service{ctx: ctx, dir: configDir, base: base, start: start, table: &discovery.Table{SelfID: id}}
	s.net = s.opts().Net
	if s.net == nil {
		s.net = netinfo.Cached(netinfo.System(), netTTL, nil)
	}
	ifaces := s.opts().Ifaces
	if ifaces == nil {
		ifaces = discovery.SystemIfaces(false)
	}
	penv, pclose := probe.System()
	s.closeProbe = pclose
	hist, speed := historyAndSpeed(configDir)
	a, err := app.New(app.Deps{
		History: hist, Speed: speed,
		Probe: penv, Router: penv.Router,
		ConfigDir: configDir, Table: s.table, SelfID: id, SelfState: s.selfState,
		Net: s.net, OpenURL: OpenURL, Counters: netinfo.NewSystemCounters(),
		SelfIPs: func() []string {
			ifs, err := ifaces()
			if err != nil {
				return nil
			}
			ips := make([]string, 0, len(ifs))
			for _, f := range ifs {
				ips = append(ips, f.IP.String())
			}
			return ips
		},
	})
	if err != nil {
		pclose()
		return nil, err
	}
	s.App = a
	if err := s.startRuntime(); err != nil {
		pclose()
		return nil, err
	}
	return s, nil
}

func (s *Service) opts() app.RuntimeOptions {
	if s.base == nil {
		return app.RuntimeOptions{}
	}
	return s.base()
}

func (s *Service) selfState() (string, int) {
	s.rtMu.Lock()
	rt := s.rt
	s.rtMu.Unlock()
	if rt == nil {
		return app.StatusAvailable, 0
	}
	return rt.SelfState()
}

// startRuntime arranca el runtime con la configuración actual (s.mu tomado o sin concurrencia).
func (s *Service) startRuntime() error {
	o := s.opts()
	o.ConfigDir, o.Alias, o.Table, o.Net = s.dir, s.App.Alias(), s.table, s.net
	rt, err := s.start(s.ctx, o)
	if err != nil {
		return err
	}
	s.rtMu.Lock()
	s.rt = rt
	s.rtMu.Unlock()
	return nil
}

func (s *Service) stopRuntime() {
	s.rtMu.Lock()
	rt := s.rt
	s.rt = nil
	s.rtMu.Unlock()
	if rt != nil {
		rt.Stop()
	}
}

// Restart aplica la configuración guardada: Stop+Start. Si falla, el error queda en
// App.State().ConfigError y el runtime queda parado hasta el siguiente reinicio.
func (s *Service) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stoped {
		return nil
	}
	s.stopRuntime()
	if err := s.startRuntime(); err != nil {
		s.App.SetRuntimeError("No se pudo aplicar la configuración: " + err.Error())
		return err
	}
	s.App.SetRuntimeError("")
	return nil
}

// RestartAsync lanza Restart sin bloquear al llamante (hilo de la UI); el canal se
// cierra al terminar.
func (s *Service) RestartAsync() <-chan struct{} {
	done := make(chan struct{})
	go func() { defer close(done); _ = s.Restart() }()
	return done
}

// Stop cancela la prueba en curso y detiene el runtime; no hay más reinicios. Idempotente.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stoped = true
	s.App.CancelTest()
	s.App.CancelProbe()
	s.App.StopMonitor()
	s.stopRuntime()
	if s.closeProbe != nil {
		s.closeProbe()
		s.closeProbe = nil
	}
}

// Backend adapta App a la interfaz del puente (State como any).
func (s *Service) Backend() Backend { return backend{s.App} }

type backend struct{ *app.App }

func (b backend) State() any { return b.App.State() }

func (b backend) Monitor(active bool) (any, error) { return b.App.Monitor(active) }

func (b backend) Adapters(detailed bool) (any, error) { return b.App.Adapters(detailed) }

func (b backend) History(host, kind string, since, until time.Time) (any, error) {
	r, err := b.App.History(history.Filter{Host: host, Kind: kind, Since: since, Until: until})
	if err != nil {
		return nil, err
	}
	recs := r.Records
	if recs == nil {
		recs = []*history.Record{}
	}
	return map[string]any{"records": recs, "skipped": r.Skipped}, nil
}

func (b backend) HistoryRecord(id string) (any, error) { return b.App.HistoryRecord(id) }

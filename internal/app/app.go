package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/report"
)

// Sentidos de la prueba.
const (
	DirFwd  = "fwd"
	DirRev  = "rev"
	DirBoth = "both"
)

// Estados mostrados en la lista.
const (
	StatusAvailable   = "available"
	StatusBusy        = "busy"
	StatusQueued      = "queued"
	StatusUnreachable = "unreachable"
)

// FirewallDelay: sin equipos tras este plazo desde el arranque, se sugiere revisar el firewall.
const FirewallDelay = 15 * time.Second

// FirewallHint es el texto de la sugerencia.
const FirewallHint = "No se encontró ningún equipo. Revise el firewall de Windows: debe permitir 5300 TCP/UDP y 5301 UDP."

// SelfState es el estado propio: StatusAvailable/StatusBusy y clientes en cola.
type SelfState func() (state string, queue int)

// Deps son las dependencias inyectables de App.
type Deps struct {
	ConfigDir string                 // directorio de configuración del usuario
	Table     *discovery.Table       // equipos descubiertos
	SelfID    string                 // id de instancia propia
	SelfIPs   func() []string        // IPs propias; nil = ninguna
	SelfState SelfState              // nil = disponible
	Hostname  func() (string, error) // nil = os.Hostname
	Now       func() time.Time       // nil = time.Now
}

// Device es un equipo de la lista.
type Device struct {
	ID     string   `json:"id"`
	Alias  string   `json:"alias"`
	IPs    []string `json:"ips"`
	Status string   `json:"status"`
	Label  string   `json:"status_label"`
	Queue  int      `json:"queue"`
	Port   int      `json:"port,omitempty"`
}

// DirectionOption es un sentido elegible con su etiqueta.
type DirectionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Estados de la prueba (TestState.Status).
const (
	TestIdle       = "idle"
	TestQueued     = "queued"
	TestRunning    = "running"
	TestDone       = "done"
	TestError      = "error"
	TestCancelling = "cancelling"
	TestCancelled  = "cancelled"
)

// TestState es el estado de la prueba. Report solo está en TestDone (mismo modelo que la CLI).
type TestState struct {
	Running  bool           `json:"running"` // en cola o midiendo
	Status   string         `json:"status"`
	Label    string         `json:"label,omitempty"`
	Phase    string         `json:"phase,omitempty"` // rtt, tcp_fwd, tcp_rev, udp_fwd, udp_rev
	Position int            `json:"queue_position,omitempty"`
	Error    string         `json:"error,omitempty"`
	Report   *report.Report `json:"report,omitempty"`
}

// State es el snapshot serializable para la UI.
type State struct {
	Self            Device            `json:"self"`
	Peers           []Device          `json:"peers"`
	SelectedID      string            `json:"selected_id"`
	Direction       string            `json:"direction"`
	Directions      []DirectionOption `json:"directions"`
	DurationSeconds int               `json:"duration_seconds"`
	TotalSeconds    int               `json:"total_seconds"`
	TotalLabel      string            `json:"total_label"`
	Alias           string            `json:"alias"`
	ExtraSubnets    []string          `json:"extra_subnets"`
	FirewallHint    string            `json:"firewall_hint"`
	Test            TestState         `json:"test"`
	CanTest         bool              `json:"can_test"`
	TestBlocked     string            `json:"test_blocked_reason"`
	ConfigError     string            `json:"config_error"`
}

// App es el backend de la interfaz. Seguro para uso concurrente.
type App struct {
	d     Deps
	start time.Time

	cfgMu     sync.Mutex // serializa leer-modificar-escribir de la config
	mu        sync.Mutex
	selected  string
	direction string
	duration  int
	test      TestState
	gen       int                // identifica la prueba vigente
	cancel    context.CancelFunc // nil si no hay prueba en curso
	testHook  func()             // solo pruebas: se llama en la goroutine de la prueba antes de fijar el estado final
	testDone  chan struct{}      // se cierra al terminar la goroutine de la prueba
}

// New crea la App; lee la duración guardada (si la configuración es inválida, devuelve error).
func New(d Deps) (*App, error) {
	if d.Table == nil {
		return nil, errors.New("falta la tabla de equipos")
	}
	if d.Hostname == nil {
		d.Hostname = os.Hostname
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	a := &App{d: d, start: d.Now(), direction: DirBoth, duration: config.DefaultDuration}
	if d.ConfigDir != "" {
		c, err := config.LoadFull(d.ConfigDir)
		if err != nil {
			return nil, err
		}
		a.duration = c.Duration
	}
	return a, nil
}

// cfg lee la configuración; si es inválida devuelve los valores por defecto y el error.
func (a *App) cfg() (config.Config, error) {
	if a.d.ConfigDir == "" {
		return config.Defaults(), nil
	}
	c, err := config.LoadFull(a.d.ConfigDir)
	if err != nil {
		return config.Defaults(), err
	}
	return c, nil
}

// Alias devuelve el alias propio: el guardado o el nombre del equipo.
func (a *App) Alias() string {
	c, _ := a.cfg()
	return a.aliasFrom(c)
}

func (a *App) aliasFrom(c config.Config) string {
	if al := strings.TrimSpace(c.Alias); al != "" {
		return al
	}
	h, err := a.d.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "Este equipo"
	}
	return h
}

func (a *App) update(fn func(*config.Config)) error {
	if a.d.ConfigDir == "" {
		return errors.New("no hay directorio de configuración")
	}
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return config.Update(a.d.ConfigDir, fn)
}

// SetAlias guarda el alias; vacío = volver al nombre del equipo.
func (a *App) SetAlias(alias string) error {
	alias = strings.TrimSpace(alias)
	if len(alias) > 64 {
		return errors.New("el alias es demasiado largo (máximo 64 bytes)")
	}
	return a.update(func(c *config.Config) { c.Alias = alias })
}

// SetKey guarda la clave compartida.
func (a *App) SetKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("la clave no puede estar vacía")
	}
	if len(key) > 256 {
		return errors.New("la clave es demasiado larga (máximo 256 bytes)")
	}
	return a.update(func(c *config.Config) { c.Key = key })
}

// HasKey indica si hay clave guardada (la clave nunca se expone).
func (a *App) HasKey() bool {
	c, _ := a.cfg()
	return c.Key != ""
}

// SetDuration valida (3-300) y guarda la duración por prueba.
func (a *App) SetDuration(sec int) error {
	if sec < config.MinDuration || sec > config.MaxDuration {
		return fmt.Errorf("la duración debe estar entre %d y %d segundos", config.MinDuration, config.MaxDuration)
	}
	if a.d.ConfigDir != "" {
		if err := a.update(func(c *config.Config) { c.Duration = sec }); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.duration = sec
	a.mu.Unlock()
	return nil
}

// SetDirection valida el sentido (fwd, rev, both).
func (a *App) SetDirection(dir string) error {
	switch dir {
	case DirFwd, DirRev, DirBoth:
	default:
		return fmt.Errorf("sentido inválido %q: use fwd, rev o both", dir)
	}
	a.mu.Lock()
	a.direction = dir
	a.mu.Unlock()
	return nil
}

// SelectPeer elige el destino: solo un equipo alcanzable distinto de "Este equipo".
func (a *App) SelectPeer(id string) error {
	if id == "" {
		return errors.New("elija un equipo de destino")
	}
	if id == a.d.SelfID {
		return errors.New("el destino no puede ser Este equipo: el origen siempre es este equipo")
	}
	for _, p := range a.d.Table.Snapshot() {
		if p.InstanceID == id {
			if !p.Reachable {
				return fmt.Errorf("%s está inalcanzable", p.Alias)
			}
			a.mu.Lock()
			a.selected = id
			a.mu.Unlock()
			return nil
		}
	}
	return errors.New("el equipo elegido ya no está en la lista")
}

// Target devuelve el destino seleccionado y alcanzable; error claro si no hay.
func (a *App) Target() (discovery.Peer, error) {
	a.mu.Lock()
	id := a.selected
	a.mu.Unlock()
	if id == "" {
		return discovery.Peer{}, errors.New("elija un equipo de destino")
	}
	for _, p := range a.d.Table.Snapshot() {
		if p.InstanceID == id {
			if !p.Reachable {
				return discovery.Peer{}, fmt.Errorf("%s está inalcanzable", p.Alias)
			}
			return p, nil
		}
	}
	return discovery.Peer{}, errors.New("el equipo elegido ya no está en la lista")
}

// AddSubnet valida y guarda una subred extra (IPv4 o CIDR hasta /24).
func (a *App) AddSubnet(s string) error {
	s = strings.TrimSpace(s)
	if _, err := discovery.ParseSubnet(s); err != nil {
		return err
	}
	return a.update(func(c *config.Config) {
		for _, x := range c.ExtraSubnets {
			if x == s {
				return
			}
		}
		c.ExtraSubnets = append(c.ExtraSubnets, s)
	})
}

// RemoveSubnet quita una subred extra guardada.
func (a *App) RemoveSubnet(s string) error {
	s = strings.TrimSpace(s)
	found := false
	err := a.update(func(c *config.Config) {
		out := make([]string, 0, len(c.ExtraSubnets))
		for _, x := range c.ExtraSubnets {
			if x == s {
				found = true
				continue
			}
			out = append(out, x)
		}
		if found {
			c.ExtraSubnets = out
		}
	})
	if err == nil && !found {
		return fmt.Errorf("la subred %q no está en la lista", s)
	}
	return err
}

func statusLabel(status string, queue int) string {
	switch status {
	case StatusBusy:
		return "ocupado"
	case StatusQueued:
		return fmt.Sprintf("en cola %d", queue)
	case StatusUnreachable:
		return "inalcanzable"
	}
	return "disponible"
}

func device(id, alias string, ips []string, status string, queue int, port int) Device {
	if ips == nil {
		ips = []string{}
	}
	return Device{ID: id, Alias: alias, IPs: ips, Status: status, Label: statusLabel(status, queue), Queue: queue, Port: port}
}

func peerStatus(state string, queue int) string {
	switch {
	case state == discovery.StateBusy && queue > 0:
		return StatusQueued
	case state == discovery.StateBusy:
		return StatusBusy
	}
	return StatusAvailable
}

func directionOptions(self, peer string) []DirectionOption {
	if peer == "" {
		peer = "destino"
	}
	return []DirectionOption{
		{DirBoth, fmt.Sprintf("%s ⇄ %s", self, peer)},
		{DirFwd, fmt.Sprintf("%s → %s", self, peer)},
		{DirRev, fmt.Sprintf("%s → %s", peer, self)},
	}
}

// SetTest permite a T12 publicar el estado de la prueba.
func (a *App) SetTest(t TestState) {
	a.mu.Lock()
	a.test = t
	a.mu.Unlock()
}

// State devuelve un snapshot completo para la UI.
func (a *App) State() State {
	cfg, cfgErr := a.cfg()
	alias := a.aliasFrom(cfg)
	a.mu.Lock()
	sel, dir, dur, test := a.selected, a.direction, a.duration, a.test
	a.mu.Unlock()
	if test.Status == "" {
		test.Status = TestIdle
	}

	var ips []string
	if a.d.SelfIPs != nil {
		ips = a.d.SelfIPs()
	}
	selfState, selfQueue := StatusAvailable, 0
	if a.d.SelfState != nil {
		selfState, selfQueue = a.d.SelfState()
	}
	self := device(a.d.SelfID, alias, ips, peerStatus(selfState, selfQueue), selfQueue, 0)

	snap := a.d.Table.Snapshot()
	peers := make([]Device, 0, len(snap))
	peerAlias := ""
	for _, p := range snap {
		st := StatusUnreachable
		if p.Reachable {
			st = peerStatus(p.State, p.Queue)
		}
		peers = append(peers, device(p.InstanceID, p.Alias, []string{p.IP.String()}, st, p.Queue, p.Port))
		if p.InstanceID == sel {
			peerAlias = p.Alias
		}
	}
	if peerAlias == "" {
		sel = ""
	}

	total := dur
	if dir == DirBoth {
		total = 2 * dur
	}
	label := fmt.Sprintf("%d s", dur)
	if dir == DirBoth {
		label = fmt.Sprintf("%d s por sentido, %d s en total", dur, total)
	}
	hint := ""
	if len(snap) == 0 && a.d.Now().Sub(a.start) >= FirewallDelay {
		hint = FirewallHint
	}
	subs := cfg.ExtraSubnets
	if subs == nil {
		subs = []string{}
	}
	cfgText := ""
	if cfgErr != nil {
		cfgText = "Configuración inválida (se usan valores por defecto): " + cfgErr.Error()
	}
	canTest, blocked := true, ""
	switch {
	case test.Status == TestCancelling:
		canTest, blocked = false, "cancelando la prueba anterior"
	case test.Running:
		canTest, blocked = false, "hay una prueba en curso"
	case sel == "":
		canTest, blocked = false, "elija un equipo de destino"
	default:
		for _, p := range snap {
			if p.InstanceID == sel && !p.Reachable {
				canTest, blocked = false, p.Alias+" está inalcanzable"
			}
		}
	}
	return State{CanTest: canTest, TestBlocked: blocked, ConfigError: cfgText, Self: self, Peers: peers, SelectedID: sel, Direction: dir, Directions: directionOptions(alias, peerAlias),
		DurationSeconds: dur, TotalSeconds: total, TotalLabel: label, Alias: alias, ExtraSubnets: subs,
		FirewallHint: hint, Test: test}
}

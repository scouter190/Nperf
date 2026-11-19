// Package config lee la configuración local de NPerf.
//
// Formato: archivo config.json en <directorio de configuración del usuario>/NPerf
// (en Windows, %AppData%\NPerf\config.json), con un objeto JSON:
//
//	{"key": "clave", "alias": "PC", "duration": 10, "discovery_port": 5301, "extra_subnets": ["10.0.0.0/24"]}
//
// Todos los campos son opcionales; los ausentes toman su valor por defecto al cargar.
// Los campos desconocidos se ignoran al leer y se conservan al guardar; un JSON inválido es un error. La clave va en texto plano
// (el archivo se crea con permisos 0600 en Unix). La validación de las subredes
// (IP/CIDR) no se hace aquí.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
)

// File es el nombre del archivo de configuración.
const File = "config.json"

// Config es el contenido de config.json.
type Config struct {
	Key           string   `json:"key"`
	Alias         string   `json:"alias"`
	Duration      int      `json:"duration"`       // segundos, 3-300
	DiscoveryPort int      `json:"discovery_port"` // 1-65535
	ExtraSubnets  []string `json:"extra_subnets"`
	// ManualHosts son los equipos sin agente añadidos a mano (máx. MaxManualHosts).
	ManualHosts []ManualHost `json:"manual_hosts"`
	// HistoryEnabled: nil (ausente) = activado. Usar HistoryOn.
	HistoryEnabled *bool `json:"history_enabled"`
	// InternetTargets: destinos públicos IPv4 (máx. MaxInternetTargets); vacío = DefaultInternetTargets. Usar Targets.
	InternetTargets []string `json:"internet_targets"`
}

// DefaultInternetTargets son los destinos públicos por defecto.
var DefaultInternetTargets = []string{"1.1.1.1", "8.8.8.8"}

// HistoryOn indica si el historial está activado (ausente = true).
func (c Config) HistoryOn() bool { return c.HistoryEnabled == nil || *c.HistoryEnabled }

// Targets devuelve los destinos de internet efectivos (copia).
func (c Config) Targets() []string {
	if len(c.InternetTargets) == 0 {
		return append([]string(nil), DefaultInternetTargets...)
	}
	return append([]string(nil), c.InternetTargets...)
}

// validPublicIPv4 acepta solo una IPv4 unicast pública en notación literal.
func validPublicIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() || a.String() != s {
		return false
	}
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsMulticast() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
}

// ManualHost es un equipo sin agente añadido a mano: nombre o IP y alias opcional.
type ManualHost struct {
	Target string `json:"target"`
	Alias  string `json:"alias"`
}

// Valores por defecto y límites.
const (
	DefaultDuration      = 10
	DefaultDiscoveryPort = 5301
	MinDuration          = 3
	MaxDuration          = 300

	MaxManualHosts     = 64
	MaxInternetTargets = 4
	MaxHostTargetLen   = 253
	MaxHostAliasLen    = 64
)

// Defaults devuelve una Config con los valores por defecto.
func Defaults() Config {
	return Config{Duration: DefaultDuration, DiscoveryPort: DefaultDiscoveryPort}
}

// Validate comprueba los rangos de duración y puerto.
func (c Config) Validate() error {
	if c.Duration < MinDuration || c.Duration > MaxDuration {
		return fmt.Errorf("duration debe estar entre %d y %d segundos (es %d)", MinDuration, MaxDuration, c.Duration)
	}
	if c.DiscoveryPort < 1 || c.DiscoveryPort > 65535 {
		return fmt.Errorf("discovery_port debe estar entre 1 y 65535 (es %d)", c.DiscoveryPort)
	}
	if len(c.ManualHosts) > MaxManualHosts {
		return fmt.Errorf("manual_hosts admite como máximo %d equipos (hay %d)", MaxManualHosts, len(c.ManualHosts))
	}
	for i, h := range c.ManualHosts {
		if h.Target == "" || len(h.Target) > MaxHostTargetLen || hasControl(h.Target) {
			return fmt.Errorf("manual_hosts[%d].target inválido (1-%d caracteres, sin control)", i, MaxHostTargetLen)
		}
		if len(h.Alias) > MaxHostAliasLen || hasControl(h.Alias) {
			return fmt.Errorf("manual_hosts[%d].alias inválido (máx. %d bytes, sin control)", i, MaxHostAliasLen)
		}
	}
	if len(c.InternetTargets) > MaxInternetTargets {
		return fmt.Errorf("internet_targets admite como máximo %d destinos (hay %d)", MaxInternetTargets, len(c.InternetTargets))
	}
	for i, t := range c.InternetTargets {
		if !validPublicIPv4(t) {
			return fmt.Errorf("internet_targets[%d] debe ser una IPv4 pública (unicast): %q", i, t)
		}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return true
		}
	}
	return false
}

// Path devuelve la ruta de config.json dentro de userConfigDir.
func Path(userConfigDir string) string { return filepath.Join(userConfigDir, "NPerf", File) }

// readRaw lee config.json como mapa de campos. exists=false si no hay archivo.
// Los campos desconocidos se aceptan (compatibilidad entre versiones de NPerf).
func readRaw(userConfigDir string) (m map[string]json.RawMessage, exists bool, err error) {
	p := Path(userConfigDir)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("no se pudo leer %s: %w", p, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&m); err != nil || m == nil {
		if err == nil {
			err = errors.New("se esperaba un objeto JSON")
		}
		return nil, true, fmt.Errorf("archivo de configuración inválido %s: %v (formato: {\"key\": \"clave\", \"alias\": \"nombre\", \"duration\": 10, \"discovery_port\": 5301, \"extra_subnets\": []})", p, err)
	}
	if dec.More() {
		return nil, true, fmt.Errorf("archivo de configuración inválido %s: contenido extra tras el objeto JSON", p)
	}
	return m, true, nil
}

// field decodifica el campo name de m en dst; si no existe no hace nada.
func field(m map[string]json.RawMessage, name string, dst any) error {
	raw, ok := m[name]
	if !ok {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// Load es la lectura para la CLI: exige JSON válido y una clave válida, pero no
// valida rangos ni tipos de los demás campos (los que no se puedan leer quedan
// con su valor por defecto). Si no existe el archivo devuelve los valores por defecto.
func Load(userConfigDir string) (Config, error) {
	c := Defaults()
	m, _, err := readRaw(userConfigDir)
	if err != nil {
		return Config{}, err
	}
	if err := field(m, "key", &c.Key); err != nil {
		return Config{}, fmt.Errorf("archivo de configuración inválido %s: campo \"key\": %v", Path(userConfigDir), err)
	}
	_ = field(m, "alias", &c.Alias)
	if d := c.Duration; field(m, "duration", &c.Duration) != nil {
		c.Duration = d
	}
	if d := c.DiscoveryPort; field(m, "discovery_port", &c.DiscoveryPort) != nil {
		c.DiscoveryPort = d
	}
	if field(m, "extra_subnets", &c.ExtraSubnets) != nil {
		c.ExtraSubnets = nil
	}
	if field(m, "history_enabled", &c.HistoryEnabled) != nil {
		c.HistoryEnabled = nil
	}
	if field(m, "internet_targets", &c.InternetTargets) != nil {
		c.InternetTargets = nil
	}
	return c, nil
}

// LoadFull es la lectura completa para la interfaz gráfica: valida tipos y rangos
// de todos los campos conocidos y nombra el archivo y el campo en el error.
func LoadFull(userConfigDir string) (Config, error) {
	c := Defaults()
	p := Path(userConfigDir)
	m, _, err := readRaw(userConfigDir)
	if err != nil {
		return Config{}, err
	}
	for name, dst := range map[string]any{"key": &c.Key, "alias": &c.Alias, "duration": &c.Duration,
		"discovery_port": &c.DiscoveryPort, "extra_subnets": &c.ExtraSubnets, "manual_hosts": &c.ManualHosts,
		"history_enabled": &c.HistoryEnabled, "internet_targets": &c.InternetTargets} {
		if err := field(m, name, dst); err != nil {
			return Config{}, fmt.Errorf("archivo de configuración inválido %s: campo %q: %v", p, name, err)
		}
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("archivo de configuración inválido %s: %v", p, err)
	}
	return c, nil
}

// renameFn permite inyectar fallos en los tests.
var renameFn = os.Rename

// Save escribe c en config.json de forma atómica: archivo temporal en el mismo
// directorio y renombrado (os.Rename reemplaza el destino también en Windows).
// Si algo falla, el archivo anterior queda intacto y se borra el temporal.
func Save(userConfigDir string, c Config) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("configuración inválida: %v", err)
	}
	p := Path(userConfigDir)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", dir, err)
	}
	// Conserva los campos desconocidos del archivo actual (si es legible).
	m, _, _ := readRaw(userConfigDir)
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	for name, v := range map[string]any{"key": c.Key, "alias": c.Alias, "duration": c.Duration,
		"discovery_port": c.DiscoveryPort, "extra_subnets": c.ExtraSubnets, "manual_hosts": c.ManualHosts,
		"history_enabled": c.HistoryEnabled, "internet_targets": c.InternetTargets} {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		m[name] = raw
	}
	if len(c.ManualHosts) == 0 { // no añade el campo a archivos que no lo tenían
		delete(m, "manual_hosts")
	}
	if c.HistoryEnabled == nil {
		delete(m, "history_enabled")
	}
	if len(c.InternetTargets) == 0 {
		delete(m, "internet_targets")
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, File+".*.tmp") // CreateTemp crea con 0600
	if err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", p, err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("no se pudo escribir %s: %w", p, err)
	}
	if _, err := tmp.Write(b); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("no se pudo escribir %s: %w", p, err)
	}
	if err := renameFn(name, p); err != nil {
		os.Remove(name)
		return fmt.Errorf("no se pudo escribir %s: %w", p, err)
	}
	// fsync del directorio para que el renombrado sea durable; en Windows no se
	// puede abrir un directorio así, y el error se ignora.
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// Update carga la configuración, aplica fn y la guarda; lo que fn no toca
// (incluida la clave y los campos desconocidos) se conserva. Si el archivo es
// inválido, falla sin escribir. No hay bloqueo entre procesos: si la CLI y la GUI
// escriben a la vez, gana la última escritura (riesgo bajo).
func Update(userConfigDir string, fn func(*Config)) error {
	c, err := LoadFull(userConfigDir)
	if err != nil {
		return err
	}
	fn(&c)
	return Save(userConfigDir, c)
}

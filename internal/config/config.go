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
}

// Valores por defecto y límites.
const (
	DefaultDuration      = 10
	DefaultDiscoveryPort = 5301
	MinDuration          = 3
	MaxDuration          = 300
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
	return nil
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
		"discovery_port": &c.DiscoveryPort, "extra_subnets": &c.ExtraSubnets} {
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
		"discovery_port": c.DiscoveryPort, "extra_subnets": c.ExtraSubnets} {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		m[name] = raw
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

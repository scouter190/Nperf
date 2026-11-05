// Package config lee la configuración local de NPerf.
//
// Formato: archivo config.json en <directorio de configuración del usuario>/NPerf
// (en Windows, %AppData%\NPerf\config.json), con un objeto JSON:
//
//	{"key": "clave-compartida"}
//
// Solo se admite el campo "key". Un campo desconocido o un JSON inválido es un error.
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
	Key string `json:"key"`
}

// Path devuelve la ruta de config.json dentro de userConfigDir.
func Path(userConfigDir string) string { return filepath.Join(userConfigDir, "NPerf", File) }

// Load lee config.json bajo userConfigDir. Si no existe devuelve una Config vacía sin error;
// si existe pero es inválido, devuelve un error que nombra el archivo.
func Load(userConfigDir string) (Config, error) {
	var c Config
	p := Path(userConfigDir)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("no se pudo leer %s: %w", p, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("archivo de configuración inválido %s: %v (formato: {\"key\": \"clave\"})", p, err)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("archivo de configuración inválido %s: contenido extra tras el objeto JSON", p)
	}
	return c, nil
}

// Package gui contiene la lógica de la ventana sin depender de Windows: el puente
// JS-Go (lista blanca), el servicio que reinicia el runtime y el servidor de la UI.
package gui

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Backend son los métodos de internal/app.App que expone el puente.
type Backend interface {
	State() any
	SelectPeer(id string) error
	SetDirection(dir string) error
	SetDuration(sec int) error
	SetAlias(alias string) error
	SetKey(key string) error
	AddSubnet(s string) error
	RemoveSubnet(s string) error
	StartTest() error
	CancelTest()
	Adapters(detailed bool) (any, error)
	OpenLocationSettings() error
	AddHost(target, alias string) error
	RemoveHost(id string) error
	StartProbe(kind, id string) error
	CancelProbe()
}

// Bridge despacha las llamadas de la UI (window.nperfCall) a Backend con una lista
// blanca de nombres y validación estricta de número y tipo de argumentos.
type Bridge struct {
	// Secret es el secreto de sesión: Call lo exige como primer argumento (ver NewSecret).
	Secret string
	B      Backend
	// OnConfigChange se llama tras guardar con éxito alias, clave o subredes
	// (debe volver de inmediato: el puente corre en el hilo de la UI).
	OnConfigChange func()
}

// NewSecret genera el secreto de sesión (32 bytes aleatorios, hex), distinto en cada ejecución.
func NewSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var errUnauthorized = errors.New("no autorizado")

var errNoArgs = errors.New("este método no recibe argumentos")

func noArgs(name string, args []json.RawMessage) error {
	if len(args) != 0 {
		return fmt.Errorf("%s: %w", name, errNoArgs)
	}
	return nil
}

func strArg(name string, args []json.RawMessage) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s: se esperaba 1 argumento (texto)", name)
	}
	var s string
	if bytes.Equal(bytes.TrimSpace(args[0]), []byte("null")) || json.Unmarshal(args[0], &s) != nil {
		return "", fmt.Errorf("%s: el argumento debe ser texto", name)
	}
	return s, nil
}

// Límites de longitud (bytes) de los textos de los diagnósticos.
const (
	maxTargetLen = 255
	maxAliasLen  = 128
	maxIDLen     = 128
)

var probeKinds = map[string]bool{"ping": true, "trace": true, "mtu": true, "ports": true}

// strArgs exige exactamente len(max) argumentos de texto, cada uno con su longitud máxima.
func strArgs(name string, args []json.RawMessage, max ...int) ([]string, error) {
	if len(args) != len(max) {
		return nil, fmt.Errorf("%s: se esperaban %d argumentos (texto)", name, len(max))
	}
	out := make([]string, len(args))
	for i, a := range args {
		if bytes.Equal(bytes.TrimSpace(a), []byte("null")) || json.Unmarshal(a, &out[i]) != nil {
			return nil, fmt.Errorf("%s: los argumentos deben ser texto", name)
		}
		if len(out[i]) > max[i] {
			return nil, fmt.Errorf("%s: argumento %d demasiado largo", name, i+1)
		}
	}
	return out, nil
}

func boolArg(name string, args []json.RawMessage) (bool, error) {
	if len(args) != 1 {
		return false, fmt.Errorf("%s: se esperaba 1 argumento (booleano)", name)
	}
	var v bool
	if bytes.Equal(bytes.TrimSpace(args[0]), []byte("null")) || json.Unmarshal(args[0], &v) != nil {
		return false, fmt.Errorf("%s: el argumento debe ser verdadero o falso", name)
	}
	return v, nil
}

func intArg(name string, args []json.RawMessage) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s: se esperaba 1 argumento (número entero)", name)
	}
	var n int
	if bytes.Equal(bytes.TrimSpace(args[0]), []byte("null")) || json.Unmarshal(args[0], &n) != nil {
		return 0, fmt.Errorf("%s: el argumento debe ser un número entero", name)
	}
	return n, nil
}

// Call comprueba el secreto de sesión (tiempo constante) y ejecuta el método permitido
// `name`. Secreto ausente o distinto = "no autorizado" sin ejecutar nada; nombres fuera
// de la lista = error.
func (b *Bridge) Call(secret, name string, args ...json.RawMessage) (any, error) {
	if b.Secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(b.Secret)) != 1 {
		return nil, errUnauthorized
	}
	changed := false
	var err error
	switch name {
	case "state":
		if err = noArgs(name, args); err != nil {
			return nil, err
		}
		return b.B.State(), nil
	case "adapters":
		var d bool
		if d, err = boolArg(name, args); err != nil {
			return nil, err
		}
		return b.B.Adapters(d)
	case "openLocationSettings":
		if err = noArgs(name, args); err == nil {
			err = b.B.OpenLocationSettings()
		}
	case "cancelProbe":
		if err = noArgs(name, args); err == nil {
			b.B.CancelProbe()
		}
	case "addHost":
		var v []string
		if v, err = strArgs(name, args, maxTargetLen, maxAliasLen); err == nil {
			err = b.B.AddHost(v[0], v[1])
		}
	case "removeHost":
		var v []string
		if v, err = strArgs(name, args, maxIDLen); err == nil {
			err = b.B.RemoveHost(v[0])
		}
	case "startProbe":
		var v []string
		if v, err = strArgs(name, args, 16, maxIDLen); err == nil {
			if !probeKinds[v[0]] {
				return nil, fmt.Errorf("%s: tipo de diagnóstico no permitido: %q", name, v[0])
			}
			err = b.B.StartProbe(v[0], v[1])
		}
	case "startTest":
		if err = noArgs(name, args); err == nil {
			err = b.B.StartTest()
		}
	case "cancelTest":
		if err = noArgs(name, args); err == nil {
			b.B.CancelTest()
		}
	case "selectPeer", "setDirection", "setAlias", "setKey", "addSubnet", "removeSubnet":
		var s string
		if s, err = strArg(name, args); err != nil {
			return nil, err
		}
		switch name {
		case "selectPeer":
			err = b.B.SelectPeer(s)
		case "setDirection":
			err = b.B.SetDirection(s)
		case "setAlias":
			err, changed = b.B.SetAlias(s), true
		case "setKey":
			err, changed = b.B.SetKey(s), true
		case "addSubnet":
			err, changed = b.B.AddSubnet(s), true
		case "removeSubnet":
			err, changed = b.B.RemoveSubnet(s), true
		}
	case "setDuration":
		var n int
		if n, err = intArg(name, args); err == nil {
			err = b.B.SetDuration(n)
		}
	default:
		return nil, fmt.Errorf("método desconocido: %q", name)
	}
	if err != nil {
		return nil, err
	}
	if changed && b.OnConfigChange != nil {
		b.OnConfigChange()
	}
	return nil, nil
}

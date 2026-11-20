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
	"sync/atomic"
	"time"

	"nperf/internal/app"
	"nperf/internal/history"
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
	Monitor(active bool) (any, error)
	History(host, kind string, since, until time.Time) (any, error)
	HistoryRecord(id string) (any, error)
	DeleteHistory(ids []string) (int, error)
	DeleteAllHistory() (int, error)
	ExportHistory(ids []string, format string, opts app.ExportOpts, path string) ([]string, error)
	SetHistoryEnabled(on bool) error
	SetInternetTargets(targets []string) error
	StartInternet() error
	StartSpeed(consent bool) error
	OpenPrivacy(which string) error
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
	// SaveDialog muestra el diálogo nativo de guardar (formato, nombre sugerido) y devuelve
	// la ruta elegida o ErrSaveCancelled. La ruta nunca viene de JS. Corre en el hilo de la UI.
	SaveDialog func(format, suggested string) (string, error)
	// Now es el reloj del nombre sugerido (nil = time.Now).
	Now func() time.Time

	saving atomic.Bool // hay un diálogo de guardar abierto (el bucle modal es reentrante)
}

// SavedCancelled es el resultado de exportHistory cuando el usuario cancela el diálogo.
const SavedCancelled = "cancelado"

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

// Límites del historial y la exportación.
const (
	maxHistoryIDs  = 1000
	maxHostFilter  = 255
	maxPersonField = 200
	maxNotesField  = 2000
	maxTargets     = 4 // config.MaxInternetTargets
)

var exportFormats = map[string]string{"csv": ".csv", "csv-std": ".csv", "json": ".json", "html": ".html"}

var privacyDocs = map[string]bool{app.PrivacyMLab: true, app.PrivacyNPerf: true}

// objArg decodifica un único argumento objeto sin campos desconocidos.
func objArg(name string, raw json.RawMessage, v any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%s: el argumento debe ser un objeto", name)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil || dec.More() {
		return fmt.Errorf("%s: argumento no válido", name)
	}
	return nil
}

// textList decodifica una lista de textos (máximo n elementos de hasta max bytes).
func textList(name string, raw json.RawMessage, n, max int) ([]string, error) {
	var l []string
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &l) != nil {
		return nil, fmt.Errorf("%s: el argumento debe ser una lista de textos", name)
	}
	if len(l) > n {
		return nil, fmt.Errorf("%s: demasiados elementos", name)
	}
	for _, x := range l {
		if len(x) > max {
			return nil, fmt.Errorf("%s: elemento demasiado largo", name)
		}
	}
	return l, nil
}

func idList(name string, raw json.RawMessage) ([]string, error) {
	ids, err := textList(name, raw, maxHistoryIDs, maxIDLen)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !history.ValidID(id) {
			return nil, fmt.Errorf("%s: identificador no válido", name)
		}
	}
	return ids, nil
}

func parseTime(name, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: fecha no válida (use RFC 3339)", name)
	}
	return t, nil
}

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
	case "monitor":
		var a bool
		if a, err = boolArg(name, args); err != nil {
			return nil, err
		}
		return b.B.Monitor(a)
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
	case "history":
		var f struct {
			Host  string `json:"host"`
			Kind  string `json:"kind"`
			Since string `json:"since"`
			Until string `json:"until"`
		}
		if len(args) != 1 {
			return nil, fmt.Errorf("%s: se esperaba 1 argumento (filtro)", name)
		}
		if err = objArg(name, args[0], &f); err != nil {
			return nil, err
		}
		if len(f.Host) > maxHostFilter || len(f.Kind) > 32 {
			return nil, fmt.Errorf("%s: filtro demasiado largo", name)
		}
		var since, until time.Time
		if since, err = parseTime(name, f.Since); err != nil {
			return nil, err
		}
		if until, err = parseTime(name, f.Until); err != nil {
			return nil, err
		}
		return b.B.History(f.Host, f.Kind, since, until)
	case "historyRecord":
		var v []string
		if v, err = strArgs(name, args, maxIDLen); err != nil {
			return nil, err
		}
		if !history.ValidID(v[0]) {
			return nil, fmt.Errorf("%s: identificador no válido", name)
		}
		return b.B.HistoryRecord(v[0])
	case "deleteHistory":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s: se esperaba 1 argumento (lista de ids)", name)
		}
		var ids []string
		if ids, err = idList(name, args[0]); err != nil {
			return nil, err
		}
		return b.B.DeleteHistory(ids)
	case "deleteAllHistory":
		if err = noArgs(name, args); err != nil {
			return nil, err
		}
		return b.B.DeleteAllHistory()
	case "exportHistory":
		return b.exportHistory(name, args)
	case "setHistoryEnabled":
		var on bool
		if on, err = boolArg(name, args); err == nil {
			err = b.B.SetHistoryEnabled(on)
		}
	case "setInternetTargets":
		if len(args) != 1 {
			return nil, fmt.Errorf("%s: se esperaba 1 argumento (lista de destinos)", name)
		}
		var t []string
		if t, err = textList(name, args[0], maxTargets, maxTargetLen); err == nil {
			err = b.B.SetInternetTargets(t)
		}
	case "startInternet":
		if err = noArgs(name, args); err == nil {
			err = b.B.StartInternet()
		}
	case "startSpeed":
		var ok bool
		if ok, err = boolArg(name, args); err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s: se requiere el consentimiento explícito (true)", name)
		}
		err = b.B.StartSpeed(true)
	case "openPrivacy":
		var v []string
		if v, err = strArgs(name, args, 16); err == nil {
			if !privacyDocs[v[0]] {
				return nil, fmt.Errorf("%s: política no permitida: %q", name, v[0])
			}
			err = b.B.OpenPrivacy(v[0])
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

// exportHistory valida todo antes de abrir el diálogo; la ruta la elige el usuario en el
// diálogo nativo, nunca llega desde JS (ni como argumento ni dentro de opts).
func (b *Bridge) exportHistory(name string, args []json.RawMessage) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("%s: se esperaban 3 argumentos (ids, formato, opciones)", name)
	}
	ids, err := idList(name, args[0])
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s: no hay registros que exportar", name)
	}
	var format string
	if bytes.Equal(bytes.TrimSpace(args[1]), []byte("null")) || json.Unmarshal(args[1], &format) != nil {
		return nil, fmt.Errorf("%s: el formato debe ser texto", name)
	}
	ext, ok := exportFormats[format]
	if !ok {
		return nil, fmt.Errorf("%s: formato no permitido: %q", name, format)
	}
	var opts app.ExportOpts
	if err := objArg(name, args[2], &opts); err != nil {
		return nil, err
	}
	if len(opts.Client) > maxPersonField || len(opts.Technician) > maxPersonField || len(opts.Notes) > maxNotesField {
		return nil, fmt.Errorf("%s: opciones demasiado largas", name)
	}
	if b.SaveDialog == nil {
		return nil, errors.New("el diálogo de guardar no está disponible")
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	if !b.saving.CompareAndSwap(false, true) {
		return nil, errors.New("ya hay un diálogo de guardar abierto")
	}
	path, err := b.runSaveDialog(format, "nperf-historial-"+now().Format("20060102")+ext)
	if errors.Is(err, ErrSaveCancelled) {
		return SavedCancelled, nil
	}
	if err != nil {
		return nil, err
	}
	return b.B.ExportHistory(ids, format, opts, path)
}

// runSaveDialog ejecuta el diálogo y libera el guard al volver (también ante un panic).
func (b *Bridge) runSaveDialog(format, suggested string) (string, error) {
	defer b.saving.Store(false)
	return b.SaveDialog(format, suggested)
}

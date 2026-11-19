// Package history guarda y lee los resultados de NPerf en disco.
package history

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
	"unicode/utf8"
)

// FormatVersion es la versión del formato del registro.
const FormatVersion = 1

// MaxRecordBytes es el tope de tamaño de un registro (al leer y al escribir).
const MaxRecordBytes = 2 << 20

const (
	maxText  = 256
	maxNotes = 64
	maxNote  = 1024
	maxError = 1024
)

// Kinds y estados conocidos.
const (
	KindTest     = "test"
	KindPing     = "ping"
	KindTrace    = "trace"
	KindMTU      = "mtu"
	KindPorts    = "ports"
	KindInternet = "internet"
	KindSpeed    = "speed"

	StatusDone      = "done"
	StatusCancelled = "cancelled"
	StatusError     = "error"
)

var idRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$`)

// Endpoint describe un equipo. No tiene campos de clave, MAC ni BSSID.
type Endpoint struct {
	Alias       string  `json:"alias,omitempty"`
	IP          string  `json:"ip,omitempty"`
	SSID        string  `json:"ssid,omitempty"`
	Link        string  `json:"link,omitempty"`
	NominalMbps float64 `json:"nominal_mbps,omitempty"`
}

// Record es un resultado guardado. Params, Result, Diagnosis y Chart son
// JSON v1 tal cual (objetos).
type Record struct {
	FormatVersion int             `json:"format_version"`
	ID            string          `json:"id"`
	Time          string          `json:"time"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	Error         string          `json:"error,omitempty"`
	Version       string          `json:"version,omitempty"`
	Source        *Endpoint       `json:"source,omitempty"`
	Dest          *Endpoint       `json:"dest,omitempty"`
	Params        json.RawMessage `json:"params,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	Diagnosis     json.RawMessage `json:"diagnosis,omitempty"`
	Chart         json.RawMessage `json:"chart,omitempty"`
	Notes         []string        `json:"notes,omitempty"`
}

// NewID genera AAAAMMDD-HHMMSS-<8 hex> en UTC con bytes de r.
func NewID(now time.Time, r io.Reader) (string, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("generar id: %w", err)
	}
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:]), nil
}

// ValidID indica si id tiene la forma exacta de un id (sin rutas ni separadores).
func ValidID(id string) bool { return len(id) == 24 && idRe.MatchString(id) }

func validText(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validNote(s string) bool {
	if len(s) > maxNote || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return false
		}
	}
	return true
}

func (e *Endpoint) validate(name string) error {
	if e == nil {
		return nil
	}
	for _, s := range []string{e.Alias, e.IP, e.SSID, e.Link} {
		if !validText(s, maxText) {
			return fmt.Errorf("%s: texto no válido", name)
		}
	}
	if e.NominalMbps < 0 || e.NominalMbps > 1e7 || e.NominalMbps != e.NominalMbps {
		return fmt.Errorf("%s: velocidad nominal fuera de rango", name)
	}
	return nil
}

func validObject(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] != '{' || !json.Valid(t) {
		return fmt.Errorf("%s: no es un objeto JSON", name)
	}
	return nil
}

// Validate comprueba tipos conocidos, tamaños y formas.
func (r *Record) Validate() error {
	if r.FormatVersion != FormatVersion {
		return fmt.Errorf("format_version %d no soportada", r.FormatVersion)
	}
	if !ValidID(r.ID) {
		return errors.New("id no válido")
	}
	if len(r.Time) > 64 {
		return errors.New("time no válido")
	}
	if _, err := time.Parse(time.RFC3339, r.Time); err != nil {
		return errors.New("time no válido")
	}
	switch r.Kind {
	case KindTest, KindPing, KindTrace, KindMTU, KindPorts, KindInternet, KindSpeed:
	default:
		return fmt.Errorf("kind desconocido %q", r.Kind)
	}
	switch r.Status {
	case StatusDone, StatusCancelled, StatusError:
	default:
		return fmt.Errorf("status desconocido %q", r.Status)
	}
	if !validNote(r.Error) || len(r.Error) > maxError {
		return errors.New("error no válido")
	}
	if !validText(r.Version, 64) {
		return errors.New("version no válida")
	}
	if err := r.Source.validate("source"); err != nil {
		return err
	}
	if err := r.Dest.validate("dest"); err != nil {
		return err
	}
	for name, raw := range map[string]json.RawMessage{
		"params": r.Params, "result": r.Result, "diagnosis": r.Diagnosis, "chart": r.Chart,
	} {
		if err := validObject(name, raw); err != nil {
			return err
		}
	}
	if len(r.Notes) > maxNotes {
		return errors.New("demasiadas notas")
	}
	for _, n := range r.Notes {
		if !validNote(n) {
			return errors.New("nota no válida")
		}
	}
	return nil
}

// Encode valida y serializa el registro (máx. 2 MB).
func (r *Record) Encode() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxRecordBytes {
		return nil, errors.New("registro mayor de 2 MB")
	}
	return b, nil
}

// Decode lee y valida un registro no confiable (máx. 2 MB).
func Decode(data []byte) (*Record, error) {
	if len(data) > MaxRecordBytes {
		return nil, errors.New("registro mayor de 2 MB")
	}
	var r Record
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("registro no válido: %w", err)
	}
	if dec.More() {
		return nil, errors.New("registro no válido: datos tras el JSON")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

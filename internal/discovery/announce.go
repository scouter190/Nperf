// Package discovery define el anuncio público de NPerf en la red local.
//
// Formato: una línea de texto ASCII "NPDISC1 clave=valor clave=valor ...".
// Se eligió texto con clave=valor porque es legible al depurar, fácil de
// extender (Parse ignora claves desconocidas, así que una versión nueva puede
// añadir campos sin romper a las anteriores) y no necesita dependencias. La
// firma NPDISC1 es a la vez la versión del formato (no del programa). El
// alias va codificado en porcentaje (url.PathEscape), por lo que acepta UTF-8
// y no puede contener espacios ni saltos de línea que rompan el análisis.
//
// El anuncio es público: no incluye clave, versión exacta del programa, ids
// de sesión ni detalles de la prueba. El id de instancia es aleatorio, se
// regenera al arrancar la app y solo sirve para deduplicar anuncios.
package discovery

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// Signature identifica el formato (y su versión).
	Signature = "NPDISC1"
	// MaxSize es el tamaño máximo del mensaje completo en bytes.
	MaxSize = 512
	// MaxAliasBytes es el máximo del alias (UTF-8 sin codificar); Marshal lo trunca.
	MaxAliasBytes = 256

	StateAvailable = "available"
	StateBusy      = "busy"

	instanceIDBytes = 8
)

// Announce es el contenido de un anuncio.
type Announce struct {
	Alias      string     // nombre visible; UTF-8
	IP         netip.Addr // dirección donde se mide
	Port       int        // puerto de medición, 1-65535
	State      string     // StateAvailable o StateBusy
	Queue      int        // clientes en espera, >= 0
	InstanceID string     // 16 caracteres hex; ver NewInstanceID
}

// NewInstanceID genera un id de instancia aleatorio (8 bytes en hex).
func NewInstanceID() (string, error) {
	b := make([]byte, instanceIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validInstanceID(s string) bool {
	if len(s) != instanceIDBytes*2 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (a Announce) validate() error {
	ip := a.IP.Unmap()
	if !ip.IsValid() || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
		return errors.New("IP inválida (solo IPv4 unicast)")
	}
	if a.Port < 1 || a.Port > 65535 {
		return errors.New("puerto fuera de rango")
	}
	if a.State != StateAvailable && a.State != StateBusy {
		return errors.New("estado desconocido")
	}
	if a.Queue < 0 {
		return errors.New("cola negativa")
	}
	if !validInstanceID(a.InstanceID) {
		return errors.New("id de instancia inválido")
	}
	return nil
}

func (a Announce) encode(alias string) string {
	return fmt.Sprintf("%s id=%s ip=%s port=%d state=%s queue=%d alias=%s",
		Signature, a.InstanceID, a.IP.Unmap().String(), a.Port, a.State, a.Queue,
		url.PathEscape(alias))
}

// Marshal serializa el anuncio. Si el mensaje superase MaxSize bytes, el
// alias se TRUNCA (por caracteres completos, antes a MaxAliasBytes) hasta que quepa; el resto de
// campos inválidos devuelven error.
func (a Announce) Marshal() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	if !utf8.ValidString(a.Alias) {
		return nil, errors.New("alias no es UTF-8 válido")
	}
	if strings.IndexFunc(a.Alias, unicode.IsControl) >= 0 {
		return nil, errors.New("alias con caracteres de control")
	}
	alias := a.Alias
	for len(alias) > MaxAliasBytes {
		_, n := utf8.DecodeLastRuneInString(alias)
		alias = alias[:len(alias)-n]
	}
	for {
		msg := a.encode(alias)
		if len(msg) <= MaxSize {
			return []byte(msg), nil
		}
		if alias == "" {
			return nil, errors.New("anuncio demasiado grande")
		}
		_, n := utf8.DecodeLastRuneInString(alias)
		alias = alias[:len(alias)-n]
	}
}

// Parse analiza un anuncio recibido. Devuelve error (nunca entra en pánico)
// ante otra firma, truncado, campos inválidos o más de MaxSize bytes.
func Parse(data []byte) (Announce, error) {
	var a Announce
	if len(data) > MaxSize {
		return a, errors.New("anuncio demasiado grande")
	}
	fields := strings.Split(string(data), " ")
	if fields[0] != Signature {
		return a, errors.New("firma desconocida")
	}
	seen := map[string]string{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return a, errors.New("campo mal formado")
		}
		if _, dup := seen[k]; dup {
			return a, errors.New("campo repetido: " + k)
		}
		seen[k] = v
	}
	for _, k := range []string{"id", "ip", "port", "state", "queue", "alias"} {
		if _, ok := seen[k]; !ok {
			return a, errors.New("falta el campo " + k)
		}
	}
	ip, err := netip.ParseAddr(seen["ip"])
	if err != nil {
		return Announce{}, errors.New("IP inválida")
	}
	port, err := strconv.Atoi(seen["port"])
	if err != nil {
		return Announce{}, errors.New("puerto inválido")
	}
	queue, err := strconv.Atoi(seen["queue"])
	if err != nil {
		return Announce{}, errors.New("cola inválida")
	}
	alias, err := url.PathUnescape(seen["alias"])
	if err != nil || !utf8.ValidString(alias) {
		return Announce{}, errors.New("alias inválido")
	}
	if strings.IndexFunc(alias, unicode.IsControl) >= 0 {
		return Announce{}, errors.New("alias con caracteres de control")
	}
	a = Announce{Alias: alias, IP: ip.Unmap(), Port: port, State: seen["state"], Queue: queue, InstanceID: seen["id"]}
	if err := a.validate(); err != nil {
		return Announce{}, err
	}
	return a, nil
}

// TCPAddr devuelve la dirección de medición host:puerto.
func (a Announce) TCPAddr() string {
	return net.JoinHostPort(a.IP.String(), strconv.Itoa(a.Port))
}

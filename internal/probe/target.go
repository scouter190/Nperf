// Package probe: diagnóstico de equipos sin NPerf (ping, traceroute, MTU, puertos).
package probe

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// MaxNameLen es el largo máximo de un nombre de equipo.
const MaxNameLen = 253

// Resolver resuelve un nombre a direcciones IP.
type Resolver interface {
	LookupIP(ctx context.Context, name string) ([]netip.Addr, error)
}

// Target es un destino sintácticamente válido: IPv4 o nombre.
type Target struct {
	Raw  string
	IP   netip.Addr // válida si el destino era una IPv4 literal
	Name string     // no vacío si es un nombre
}

// ParseTarget valida solo la sintaxis, sin resolver ni enviar nada.
func ParseTarget(s string) (Target, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Target{}, errors.New("destino vacío")
	}
	if len(s) > MaxNameLen {
		return Target{}, fmt.Errorf("destino demasiado largo (máx. %d caracteres)", MaxNameLen)
	}
	if strings.Contains(s, "://") {
		return Target{}, errors.New("el destino no debe llevar esquema (use solo IP o nombre)")
	}
	if strings.ContainsAny(s, "/\\") {
		return Target{}, errors.New("no se admiten CIDR, rutas ni URL")
	}
	if strings.Contains(s, ":") {
		return Target{}, errors.New("no se admiten puertos ni IPv6")
	}
	if strings.ContainsAny(s, " \t\r\n,;*?") {
		return Target{}, errors.New("el destino contiene caracteres no válidos")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return Target{}, errors.New("el destino contiene caracteres de control")
		}
	}
	if isDigitsAndDots(s) {
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() {
			return Target{}, errors.New("dirección IPv4 no válida")
		}
		return Target{Raw: s, IP: a}, nil
	}
	if strings.Contains(s, "-") && strings.Trim(s, "0123456789.-") == "" {
		return Target{}, errors.New("no se admiten rangos de direcciones")
	}
	if !validName(s) {
		return Target{}, errors.New("nombre de equipo no válido")
	}
	return Target{Raw: s, Name: s}, nil
}

func isDigitsAndDots(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

// validName: etiquetas de letras, dígitos y guion (no al borde), no vacías.
func validName(s string) bool {
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for _, r := range l {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
			if !ok {
				return false
			}
		}
	}
	return true
}

// Resolve devuelve la IPv4 del destino; un nombre usa la primera IPv4 del resolver.
func Resolve(ctx context.Context, t Target, r Resolver) (netip.Addr, error) {
	if t.IP.IsValid() {
		return t.IP, nil
	}
	if t.Name == "" {
		return netip.Addr{}, errors.New("destino sin validar")
	}
	if r == nil {
		return netip.Addr{}, errors.New("sin resolver de nombres")
	}
	ips, err := r.LookupIP(ctx, t.Name)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("no se pudo resolver %q: %w", t.Name, err)
	}
	for _, a := range ips {
		a = a.Unmap()
		if a.Is4() {
			return a, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("%q no tiene dirección IPv4", t.Name)
}

// CheckAddr rechaza direcciones que no son un equipo: 0.0.0.0, multicast,
// broadcast y red/broadcast de los prefijos locales dados.
func CheckAddr(a netip.Addr, local []netip.Prefix) error {
	if !a.Is4() {
		return errors.New("solo se admite IPv4")
	}
	switch {
	case a.IsUnspecified():
		return errors.New("0.0.0.0 no es un destino válido")
	case a.IsMulticast():
		return errors.New("las direcciones multicast no son un destino válido")
	case a == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return errors.New("255.255.255.255 (broadcast) no es un destino válido")
	}
	for _, p := range local {
		p = p.Masked()
		if !p.IsValid() || !p.Addr().Is4() || p.Bits() >= 31 || !p.Contains(a) {
			continue
		}
		if a == p.Addr() {
			return fmt.Errorf("%s es la dirección de red de %s", a, p)
		}
		if a == lastAddr(p) {
			return fmt.Errorf("%s es la dirección de broadcast de %s", a, p)
		}
	}
	return nil
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As4()
	host := 32 - p.Bits()
	for i := 0; i < host; i++ {
		b[3-i/8] |= 1 << (i % 8)
	}
	return netip.AddrFrom4(b)
}

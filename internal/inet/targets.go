// Package inet reúne la lógica de la prueba de latencia hacia internet.
package inet

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// MaxTargets es el máximo de destinos públicos editables.
const MaxTargets = 4

// DefaultTargets son los destinos usados cuando no hay ninguno configurado.
func DefaultTargets() []netip.Addr {
	return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")}
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), // reservado y broadcast
}

// ParseTargets valida destinos: IPv4 unicast pública, máx. MaxTargets.
// Vacío devuelve DefaultTargets. Los repetidos se ignoran.
func ParseTargets(in []string) ([]netip.Addr, error) {
	if len(in) == 0 {
		return DefaultTargets(), nil
	}
	if len(in) > MaxTargets {
		return nil, fmt.Errorf("demasiados destinos (máx. %d)", MaxTargets)
	}
	var out []netip.Addr
	seen := map[netip.Addr]bool{}
	for _, s := range in {
		a, err := parseTarget(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("destino %q: %w", s, err)
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out, nil
}

func parseTarget(s string) (netip.Addr, error) {
	if s == "" {
		return netip.Addr{}, errors.New("vacío")
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, errors.New("no es una dirección IP")
	}
	if !a.Is4() {
		return netip.Addr{}, errors.New("solo se admite IPv4")
	}
	if a.IsMulticast() || a.IsLoopback() || a.IsUnspecified() || a.IsLinkLocalUnicast() || a.IsPrivate() {
		return netip.Addr{}, errors.New("debe ser una dirección IPv4 pública")
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return netip.Addr{}, errors.New("debe ser una dirección IPv4 pública")
		}
	}
	return a, nil
}

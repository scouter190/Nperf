package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

const (
	// QueryMessage es el prefijo de la consulta unicast: pide a un NPerf que
	// responda con su anuncio.
	QueryMessage = "NPDISC1 ?"
	// QuerySize es el tamaño exacto mínimo de la consulta: QueryMessage
	// rellenado con espacios. Anti-amplificación: respuesta <= 512 bytes
	// frente a consulta >= 128 bytes, factor máximo ~4x.
	QuerySize = 128

	// minSubnetBits: la subred más amplia permitida es /24 (254 hosts).
	minSubnetBits = 24
)

// QueryPacket devuelve la consulta completa: QueryMessage más espacios hasta
// QuerySize bytes.
func QueryPacket() []byte {
	b := []byte(strings.Repeat(" ", QuerySize))
	copy(b, QueryMessage)
	return b
}

// isQuery indica si data es una consulta válida: al menos QuerySize bytes,
// prefijo QueryMessage y el resto solo espacios de relleno.
func isQuery(data []byte) bool {
	if len(data) < QuerySize || string(data[:len(QueryMessage)]) != QueryMessage {
		return false
	}
	for _, c := range data[len(QueryMessage):] {
		if c != ' ' {
			return false
		}
	}
	return true
}

// ParseSubnet interpreta una IP IPv4 o un CIDR IPv4 de máximo /24 y devuelve
// las direcciones a consultar. En /24../30 se excluyen la dirección de red y
// la de broadcast; en /31 y /32 se devuelven tal cual.
func ParseSubnet(s string) ([]netip.Addr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("subred vacía")
	}
	if strings.Contains(s, ":") {
		return nil, fmt.Errorf("%q: IPv6 no está soportado, use IPv4", s)
	}
	if !strings.Contains(s, "/") {
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() {
			return nil, fmt.Errorf("%q no es una IP IPv4 válida", s)
		}
		return []netip.Addr{a}, nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return nil, fmt.Errorf("%q no es un CIDR IPv4 válido (ejemplo: 192.168.1.0/24)", s)
	}
	if p.Bits() < minSubnetBits {
		return nil, fmt.Errorf("%q es demasiado amplia: el máximo permitido es /24", s)
	}
	p = p.Masked()
	var out []netip.Addr
	for a := p.Addr(); a.IsValid() && p.Contains(a); a = a.Next() {
		out = append(out, a)
	}
	if p.Bits() <= 30 && len(out) > 2 {
		out = out[1 : len(out)-1]
	}
	return out, nil
}

// SendQueries envía la consulta NPDISC1 ? (rellena hasta 128 bytes) a cada dirección en el puerto dado.
// Las respuestas llegan al mismo socket (lo lee el Listener). Respeta ctx
// (cancelación y plazo) y no deja goroutines. Los fallos de envío puntuales
// no detienen el resto; se devuelve el conjunto de errores.
func SendQueries(ctx context.Context, conn net.PacketConn, addrs []netip.Addr, port int) error {
	if port < 1 || port > 65535 {
		return errors.New("puerto de descubrimiento fuera de rango")
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(dl)
	} else {
		_ = conn.SetWriteDeadline(time.Time{})
	}
	var errs []error
	for _, a := range addrs {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		ua := net.UDPAddrFromAddrPort(netip.AddrPortFrom(a, uint16(port)))
		if _, err := conn.WriteTo(QueryPacket(), ua); err != nil {
			errs = append(errs, fmt.Errorf("consulta a %s: %w", a, err))
		}
	}
	return errors.Join(errs...)
}

package probe

import (
	"errors"
	"net/netip"
)

// Límites de MTU de interfaz creíbles; fuera de ellos el valor es "desconocido" (0).
const (
	minPlausibleMTU = 576
	maxPlausibleMTU = 9216
)

// ReferenceIP es la IP pública usada solo para preguntar al sistema cuál es la
// ruta por defecto (no se envía tráfico).
var ReferenceIP = netip.MustParseAddr("1.1.1.1")

var (
	// ErrNoGateway: no hay puerta de enlace (ruta directa o sin ruta por defecto).
	ErrNoGateway = errors.New("sin puerta de enlace")
	// ErrRouteUnsupported: la consulta de rutas no existe en este sistema.
	ErrRouteUnsupported = errors.New("consulta de rutas no soportada en este sistema")
)

// Route describe la mejor ruta hacia un destino IPv4.
type Route struct {
	IfIndex uint32
	Source  netip.Addr // IP local de origen
	NextHop netip.Addr // inválido (netip.Addr{}) si el destino está en enlace directo; 0.0.0.0 se normaliza a vacío
	MTU     int        // MTU de la interfaz; 0 = desconocido
}

// Router consulta la tabla de rutas del sistema.
type Router interface {
	Route(dst netip.Addr) (Route, error)
	Gateway() (netip.Addr, error)
	LocalPrefixes() ([]netip.Prefix, error)
}

// normalizeMTU devuelve 0 (desconocido) si mtu queda fuera de 576–9216
// (p. ej. loopback de Windows informa 4294967295).
func normalizeMTU(mtu uint32) int {
	if mtu < minPlausibleMTU || mtu > maxPlausibleMTU {
		return 0
	}
	return int(mtu)
}

// gatewayFromRoute extrae la puerta de enlace de la ruta hacia ReferenceIP.
func gatewayFromRoute(r Route) (netip.Addr, error) {
	if !r.NextHop.IsValid() || r.NextHop.IsUnspecified() || !r.NextHop.Is4() {
		return netip.Addr{}, ErrNoGateway
	}
	return r.NextHop, nil
}

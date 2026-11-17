//go:build !windows

package probe

import "net/netip"

type systemRouter struct{}

// NewSystemRouter devuelve un Router que informa "no soportado" fuera de Windows.
func NewSystemRouter() Router { return systemRouter{} }

func (systemRouter) Route(netip.Addr) (Route, error) { return Route{}, ErrRouteUnsupported }
func (systemRouter) Gateway() (netip.Addr, error)    { return netip.Addr{}, ErrRouteUnsupported }
func (systemRouter) LocalPrefixes() ([]netip.Prefix, error) {
	return nil, ErrRouteUnsupported
}

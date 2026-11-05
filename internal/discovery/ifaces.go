package discovery

import (
	"net"
	"net/netip"
)

// Iface es una dirección IPv4 activa con su dirección de broadcast.
type Iface struct {
	Name      string
	IP        netip.Addr   // IP propia de la interfaz
	Broadcast netip.Addr   // broadcast de su subred
	Prefix    netip.Prefix // subred de la interfaz (opcional; la usa Listener.Self para elegir IP)
}

// IfaceSource enumera las interfaces IPv4 activas. Es inyectable en tests.
type IfaceSource func() ([]Iface, error)

// BroadcastAddr calcula el broadcast de la subred de p (solo IPv4). Devuelve
// false si no hay broadcast: /31 y /32 (punto a punto o host), IPv6 o inválido.
func BroadcastAddr(p netip.Prefix) (netip.Addr, bool) {
	if !p.IsValid() || !p.Addr().Is4() || p.Bits() > 30 {
		return netip.Addr{}, false
	}
	a := p.Addr().As4()
	host := uint32(0xFFFFFFFF) >> uint(p.Bits())
	for i := 0; i < 4; i++ {
		a[i] |= byte(host >> uint(24-8*i))
	}
	return netip.AddrFrom4(a), true
}

// SystemIfaces devuelve la fuente real: interfaces up, con broadcast, IPv4 y
// con dirección y máscara. Excluye loopback salvo que includeLoopback sea
// true (solo para pruebas). No filtra interfaces virtuales (Hyper-V, VPN):
// se anuncian todas las IPv4 activas; el filtrado es un riesgo abierto de la
// spec. Las interfaces sin broadcast (punto a punto, /31, /32) se ignoran.
func SystemIfaces(includeLoopback bool) IfaceSource {
	return func() ([]Iface, error) {
		ifs, err := net.Interfaces()
		if err != nil {
			return nil, err
		}
		var out []Iface
		for _, ni := range ifs {
			if ni.Flags&net.FlagUp == 0 || ni.Flags&net.FlagBroadcast == 0 {
				continue
			}
			if ni.Flags&net.FlagLoopback != 0 && !includeLoopback {
				continue
			}
			addrs, err := ni.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				ipn, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				ip, ok := netip.AddrFromSlice(ipn.IP)
				if !ok {
					continue
				}
				ip = ip.Unmap()
				ones, _ := ipn.Mask.Size()
				bc, ok := BroadcastAddr(netip.PrefixFrom(ip, ones))
				if !ok {
					continue
				}
				out = append(out, Iface{Name: ni.Name, IP: ip, Broadcast: bc, Prefix: netip.PrefixFrom(ip, ones)})
			}
		}
		return out, nil
	}
}

//go:build windows

package probe

import (
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetBestRoute2 = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetBestRoute2")

// sockaddrInet es SOCKADDR_INET (unión de 28 bytes, alineada a 4).
type sockaddrInet struct {
	Family uint16
	Data   [26]byte
}

// mibIPForwardRow2 es MIB_IPFORWARD_ROW2 (104 B en amd64).
type mibIPForwardRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	DestinationPrefix    sockaddrInet
	DestinationPrefixLen uint8
	_                    [3]byte
	NextHop              sockaddrInet
	SitePrefixLength     uint8
	ValidLifetime        uint32
	PreferredLifetime    uint32
	Metric               uint32
	Protocol             uint32
	Loopback             uint8
	AutoconfigureAddress uint8
	Publish              uint8
	Immortal             uint8
	Age                  uint32
	Origin               uint32
}

func newSockaddrInet4(a netip.Addr) sockaddrInet {
	var s sockaddrInet
	s.Family = windows.AF_INET
	b := a.As4()
	copy(s.Data[2:6], b[:]) // sin_port (2 B) + sin_addr
	return s
}

func (s *sockaddrInet) addr() netip.Addr {
	if s.Family != windows.AF_INET {
		return netip.Addr{}
	}
	a := netip.AddrFrom4([4]byte(s.Data[2:6]))
	if a.IsUnspecified() {
		return netip.Addr{} // 0.0.0.0 = enlace directo
	}
	return a
}

type systemRouter struct{}

// NewSystemRouter devuelve el Router real de Windows (sin administrador).
func NewSystemRouter() Router { return systemRouter{} }

func (systemRouter) Route(dst netip.Addr) (Route, error) {
	if !dst.Is4() {
		return Route{}, fmt.Errorf("solo IPv4: %v", dst)
	}
	d := newSockaddrInet4(dst)
	var row mibIPForwardRow2
	var src sockaddrInet
	r, _, _ := procGetBestRoute2.Call(0, 0, 0, uintptr(unsafe.Pointer(&d)), 0,
		uintptr(unsafe.Pointer(&row)), uintptr(unsafe.Pointer(&src)))
	if r != 0 {
		return Route{}, fmt.Errorf("GetBestRoute2: %w", windows.Errno(r))
	}
	rt := Route{IfIndex: row.InterfaceIndex, Source: src.addr(), NextHop: row.NextHop.addr()}
	if ad, err := adapters(); err == nil {
		for _, a := range ad {
			if a.IfIndex == rt.IfIndex {
				rt.MTU = normalizeMTU(a.Mtu)
				break
			}
		}
	}
	return rt, nil
}

func (s systemRouter) Gateway() (netip.Addr, error) {
	r, err := s.Route(ReferenceIP)
	if err != nil {
		return netip.Addr{}, err
	}
	return gatewayFromRoute(r)
}

func (systemRouter) LocalPrefixes() ([]netip.Prefix, error) {
	ad, err := adapters()
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, a := range ad {
		for _, p := range a.Prefixes {
			if !p.Addr().IsLoopback() {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

type adapterInfo struct {
	IfIndex  uint32
	Mtu      uint32
	Prefixes []netip.Prefix
}

func adapters() ([]adapterInfo, error) {
	const flags = windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER
	size := uint32(15000)
	var buf []byte
	for i := 0; i < 5; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW || i == 4 {
			return nil, fmt.Errorf("GetAdaptersAddresses: %w", err)
		}
	}
	var out []adapterInfo
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		info := adapterInfo{IfIndex: a.IfIndex, Mtu: a.Mtu}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			sa := u.Address.Sockaddr
			if sa == nil || u.Address.SockaddrLength < 8 {
				continue
			}
			if (*windows.RawSockaddrAny)(unsafe.Pointer(sa)).Addr.Family != windows.AF_INET {
				continue
			}
			ip := netip.AddrFrom4((*windows.RawSockaddrInet4)(unsafe.Pointer(sa)).Addr)
			if p, err := ip.Prefix(int(u.OnLinkPrefixLength)); err == nil {
				info.Prefixes = append(info.Prefixes, p)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

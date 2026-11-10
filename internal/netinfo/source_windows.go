//go:build windows

package netinfo

import (
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsSource struct {
	wifi func(guid string) RawWiFi
}

// NewSystemSource devuelve la fuente real de Windows (GetAdaptersAddresses, sin
// administrador). wifi consulta WlanAPI; si es nil el WiFi consta como servicio
// WLAN no disponible (1062).
func NewSystemSource(wifi func(guid string) RawWiFi) Source {
	return &windowsSource{wifi: wifi}
}

func (s *windowsSource) WiFi(guid string) (RawWiFi, error) {
	if s.wifi == nil {
		return RawWiFi{Err: 1062}, nil
	}
	return s.wifi(guid), nil
}

func (s *windowsSource) Adapters() ([]RawAdapter, error) {
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
	var out []RawAdapter
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); a != nil; a = a.Next {
		r := RawAdapter{
			Name:        windows.UTF16PtrToString(a.FriendlyName),
			GUID:        bytePtrToString(a.AdapterName),
			Description: windows.UTF16PtrToString(a.Description),
			IfType:      a.IfType,
			OperStatus:  a.OperStatus,
			TxBps:       linkSpeedBps(a.TransmitLinkSpeed),
			RxBps:       linkSpeedBps(a.ReceiveLinkSpeed),
		}
		if n := int(a.PhysicalAddressLength); n > 0 && n <= len(a.PhysicalAddress) {
			r.MAC = fmt.Sprintf("%02X", a.PhysicalAddress[0])
			for _, b := range a.PhysicalAddress[1:n] {
				r.MAC += fmt.Sprintf("-%02X", b)
			}
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			sa := u.Address.Sockaddr
			if sa == nil || u.Address.SockaddrLength < 8 {
				continue
			}
			if (*windows.RawSockaddrAny)(unsafe.Pointer(sa)).Addr.Family != windows.AF_INET {
				continue
			}
			ip := (*windows.RawSockaddrInet4)(unsafe.Pointer(sa)).Addr
			r.IPv4 = append(r.IPv4, netip.AddrFrom4(ip))
		}
		// ConnectorPresent no lo ofrece x/sys: Physical queda nil (heurística por descripción).
		out = append(out, r)
	}
	return out, nil
}

func bytePtrToString(p *byte) string {
	if p == nil {
		return ""
	}
	return windows.BytePtrToString(p)
}

// System devuelve la fuente real de Windows con la consulta WlanAPI incluida.
func System() Source { return NewSystemSource(queryWiFi) }

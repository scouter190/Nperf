package netinfo

import (
	"net/netip"
	"strings"

	"nperf/internal/proto"
)

// ForLocalIP elige el adaptador que tiene la IPv4 dada (dirección local de la
// conexión de control). ok=false si ninguno coincide (interfaz desconocida).
func ForLocalIP(infos []Info, ip netip.Addr) (Info, bool) {
	ip = ip.Unmap()
	for _, in := range infos {
		for _, a := range in.IPv4 {
			if a.Unmap() == ip {
				return in, true
			}
		}
	}
	return Info{}, false
}

// shortStandard reduce "802.11ax (WiFi 6)" a "WiFi 6" (cabe en proto.Link).
func shortStandard(s string) string {
	if s == NotAvailable {
		return ""
	}
	if i := strings.Index(s, "("); i >= 0 {
		return strings.TrimSuffix(strings.TrimSpace(s[i+1:]), ")")
	}
	return s
}

// Summary convierte el adaptador en el resumen de enlace del protocolo.
// Nunca incluye BSSID ni MAC. Datos no disponibles quedan vacíos o en 0.
func (i Info) Summary() *proto.Link {
	l := &proto.Link{Kind: string(i.Kind)}
	if !i.Connected {
		return l
	}
	l.TxBps, l.RxBps = int64(i.TxBps), int64(i.RxBps)
	if w := i.WiFi; w != nil && w.Available {
		if w.TxBps > 0 {
			l.TxBps = int64(w.TxBps)
		}
		if w.RxBps > 0 {
			l.RxBps = int64(w.RxBps)
		}
		l.SSID, l.Band, l.Standard = w.SSID, w.Band, shortStandard(w.Standard)
		if w.SignalPct != nil {
			l.SignalPct = *w.SignalPct
		}
	}
	return l
}

// SummaryForLocalIP es ForLocalIP + Summary; sin coincidencia devuelve nil (desconocida).
func SummaryForLocalIP(infos []Info, ip netip.Addr) *proto.Link {
	in, ok := ForLocalIP(infos, ip)
	if !ok {
		return nil
	}
	return in.Summary()
}

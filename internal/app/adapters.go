package app

import (
	"errors"
	"net/netip"

	"nperf/internal/netinfo"
)

// LocationSettingsURL es la única URL que se abre desde la interfaz.
const LocationSettingsURL = "ms-settings:privacy-location"

// LinkUnknown es el tipo de enlace de un equipo que no lo anuncia (versión antigua).
const LinkUnknown = "unknown"

// InventorySource es la fuente del inventario local (netinfo.Cached la cumple).
type InventorySource interface {
	Inventory() ([]netinfo.Info, error)
}

// Adapters devuelve las tarjetas de red locales (vista compacta o detallada) con el
// aviso de ubicación. Solo contiene datos de red, nunca la configuración.
func (a *App) Adapters(detailed bool) (netinfo.View, error) {
	if a.d.Net == nil {
		return netinfo.View{}, errors.New("la detección de red no está disponible")
	}
	infos, err := a.d.Net.Inventory()
	if err != nil {
		return netinfo.View{}, err
	}
	return netinfo.BuildView(infos, detailed), nil
}

// OpenLocationSettings abre la página de ubicación de Windows con la URL fija.
func (a *App) OpenLocationSettings() error {
	if a.d.OpenURL == nil {
		return errors.New("no se puede abrir la configuración en este sistema")
	}
	return a.d.OpenURL(LocationSettingsURL)
}

func speedLabel(tx, rx int64) string {
	switch {
	case tx <= 0 && rx <= 0:
		return netinfo.NotAvailable
	case tx == rx:
		return netinfo.FormatBps(uint64(tx))
	case tx <= 0:
		return "↓ " + netinfo.FormatBps(uint64(rx))
	case rx <= 0:
		return "↑ " + netinfo.FormatBps(uint64(tx))
	}
	return "↑ " + netinfo.FormatBps(uint64(tx)) + " / ↓ " + netinfo.FormatBps(uint64(rx))
}

// withLink rellena link, link_speed y ssid; kind vacío = desconocido.
func (d Device) withLink(kind string, tx, rx int64, ssid string) Device {
	if kind == "" {
		kind = LinkUnknown
	}
	d.Link, d.LinkSpeed = kind, speedLabel(tx, rx)
	if netinfo.ValidSSID(ssid) {
		d.SSID = ssid
	}
	return d
}

// selfLink toma el enlace del adaptador que tiene alguna de las IPs propias.
func (a *App) selfLink(d Device, ips []string) Device {
	if a.d.Net != nil {
		if infos, err := a.d.Net.Inventory(); err == nil {
			for _, s := range ips {
				ip, err := netip.ParseAddr(s)
				if err != nil {
					continue
				}
				if l := netinfo.SummaryForLocalIP(infos, ip); l != nil {
					return d.withLink(l.Kind, l.TxBps, l.RxBps, l.SSID)
				}
			}
		}
	}
	return d.withLink("", 0, 0, "")
}

package netinfo

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FormatVersion es la versión de la forma JSON de la vista.
const FormatVersion = 1

// LocationNotice es el aviso mostrado cuando Windows niega los datos WiFi.
const LocationNotice = "Windows no permite leer los datos WiFi: activa el permiso de ubicación para las aplicaciones en Configuración › Privacidad › Ubicación."

// KindLabel devuelve el nombre en español del tipo.
func KindLabel(k Kind) string {
	switch k {
	case KindEthernet:
		return "Ethernet"
	case KindWiFi:
		return "WiFi"
	}
	return "Otro"
}

// WiFiView son los datos WiFi ya formateados como texto. Un campo vacío
// significa "no disponible"; Reason explica por qué.
type WiFiView struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	SSID      string `json:"ssid,omitempty"`
	Band      string `json:"band,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Signal    string `json:"signal,omitempty"` // "72 %"
	// Solo en vista detallada.
	BSSID    string `json:"bssid,omitempty"`
	RSSI     string `json:"rssi,omitempty"` // "-55 dBm"
	Standard string `json:"standard,omitempty"`
	TxRate   string `json:"tx_rate,omitempty"`
	RxRate   string `json:"rx_rate,omitempty"`
}

// AdapterView es una tarjeta lista para mostrar.
type AdapterView struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // eth|wifi|other
	KindLabel string    `json:"kind_label"`
	Connected bool      `json:"connected"`
	Speed     string    `json:"speed"` // "1 Gbps", "no disponible", "desconectada"
	WiFi      *WiFiView `json:"wifi,omitempty"`
	// Solo en vista detallada.
	Description string   `json:"description,omitempty"`
	MAC         string   `json:"mac,omitempty"`
	IPv4        []string `json:"ipv4,omitempty"`
	TxRate      string   `json:"tx_rate,omitempty"`
	RxRate      string   `json:"rx_rate,omitempty"`
}

// View es la lista de tarjetas con el aviso de ubicación.
type View struct {
	Adapters       []AdapterView `json:"adapters"`
	LocationDenied bool          `json:"location_denied"`
	LocationNotice string        `json:"location_notice,omitempty"`
}

func speedText(a Adapter) string {
	if !a.Connected {
		return "desconectada"
	}
	if a.TxBps == 0 && a.RxBps == 0 {
		return NotAvailable
	}
	return rateText(a.TxBps, a.RxBps)
}

func rateText(tx, rx uint64) string {
	switch {
	case tx == rx:
		return FormatBps(tx)
	case tx == 0:
		return "↓ " + FormatBps(rx)
	case rx == 0:
		return "↑ " + FormatBps(tx)
	}
	return "↑ " + FormatBps(tx) + " / ↓ " + FormatBps(rx)
}

func orNA(s string) string {
	if s == "" {
		return NotAvailable
	}
	return s
}

func wifiView(w *WiFiInfo, detailed bool) *WiFiView {
	if w == nil {
		return nil
	}
	v := &WiFiView{Available: w.Available, Reason: w.Reason}
	if !w.Available {
		return v
	}
	v.SSID, v.Band = orNA(w.SSID), orNA(w.Band)
	v.Channel, v.Signal = NotAvailable, NotAvailable
	if w.Channel != nil {
		v.Channel = strconv.Itoa(*w.Channel)
	}
	if w.SignalPct != nil {
		v.Signal = strconv.Itoa(*w.SignalPct) + " %"
	}
	if detailed {
		v.BSSID = orNA(w.BSSID)
		v.RSSI = NotAvailable
		if w.RSSI != nil {
			v.RSSI = strconv.Itoa(*w.RSSI) + " dBm"
		}
		v.Standard = w.Standard
		v.TxRate, v.RxRate = bpsOrNA(w.TxBps), bpsOrNA(w.RxBps)
	}
	return v
}

func bpsOrNA(b uint64) string {
	if b == 0 {
		return NotAvailable
	}
	return FormatBps(b)
}

// BuildView construye la vista compacta (detailed=false) o detallada.
func BuildView(infos []Info, detailed bool) View {
	v := View{Adapters: make([]AdapterView, 0, len(infos))}
	for _, in := range infos {
		av := AdapterView{
			Name: in.Name, Kind: string(in.Kind), KindLabel: KindLabel(in.Kind),
			Connected: in.Connected, Speed: speedText(in.Adapter),
			WiFi: wifiView(in.WiFi, detailed),
		}
		if in.WiFi != nil && in.WiFi.LocationDenied {
			v.LocationDenied = true
		}
		if detailed {
			av.Description, av.MAC = in.Description, in.MAC
			for _, ip := range in.IPv4 {
				av.IPv4 = append(av.IPv4, ip.String())
			}
			if in.Connected {
				av.TxRate, av.RxRate = bpsOrNA(in.TxBps), bpsOrNA(in.RxBps)
			}
		}
		v.Adapters = append(v.Adapters, av)
	}
	if v.LocationDenied {
		v.LocationNotice = LocationNotice
	}
	return v
}

// Text devuelve la vista como líneas legibles en español.
func (v View) Text() string {
	var b strings.Builder
	for i, a := range v.Adapters {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(a.Name + " [" + a.KindLabel + "] " + a.Speed + "\n")
		if a.Description != "" {
			b.WriteString("  Descripción: " + a.Description + "\n")
		}
		if a.MAC != "" {
			b.WriteString("  MAC: " + a.MAC + "\n")
		}
		if len(a.IPv4) > 0 {
			b.WriteString("  IPv4: " + strings.Join(a.IPv4, ", ") + "\n")
		}
		if w := a.WiFi; w != nil {
			if !w.Available {
				b.WriteString("  WiFi: " + orNA(w.Reason) + "\n")
				continue
			}
			b.WriteString("  WiFi: " + w.SSID + ", " + w.Band + ", canal " + w.Channel + ", señal " + w.Signal + "\n")
			if w.Standard != "" {
				b.WriteString("  Estándar: " + w.Standard + ", BSSID " + w.BSSID + ", RSSI " + w.RSSI +
					", tx " + w.TxRate + ", rx " + w.RxRate + "\n")
			}
		}
	}
	if v.LocationDenied {
		b.WriteString("\n" + v.LocationNotice + "\n")
	}
	return b.String()
}

// JSON v1: valores no disponibles son null y llevan "status".

type jsonWiFi struct {
	Status    string  `json:"status"` // ok | unavailable
	Reason    *string `json:"reason"`
	SSID      *string `json:"ssid"`
	BSSID     *string `json:"bssid"`
	Band      *string `json:"band"`
	Channel   *int    `json:"channel"`
	SignalPct *int    `json:"signal_pct"`
	RSSIDbm   *int    `json:"rssi_dbm"`
	Standard  *string `json:"standard"`
	TxBps     *uint64 `json:"tx_bps"`
	RxBps     *uint64 `json:"rx_bps"`
}

type jsonAdapter struct {
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	MAC         *string   `json:"mac"`
	IPv4        []string  `json:"ipv4"`
	Type        string    `json:"type"`
	Connected   bool      `json:"connected"`
	LinkTxBps   *uint64   `json:"link_tx_bps"`
	LinkRxBps   *uint64   `json:"link_rx_bps"`
	LinkStatus  string    `json:"link_status"` // ok | unavailable | disconnected
	WiFi        *jsonWiFi `json:"wifi"`
}

type jsonDoc struct {
	FormatVersion  int           `json:"format_version"`
	Adapters       []jsonAdapter `json:"adapters"`
	LocationDenied bool          `json:"location_denied"`
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func u64p(n uint64) *uint64 {
	if n == 0 {
		return nil
	}
	return &n
}

// JSON devuelve la forma JSON v1 (detallada) de las tarjetas.
func JSON(infos []Info) ([]byte, error) {
	doc := jsonDoc{FormatVersion: FormatVersion, Adapters: []jsonAdapter{}}
	for _, in := range infos {
		ja := jsonAdapter{
			Name: in.Name, Description: strp(in.Description), MAC: strp(in.MAC),
			IPv4: []string{}, Type: string(in.Kind), Connected: in.Connected,
		}
		for _, ip := range in.IPv4 {
			ja.IPv4 = append(ja.IPv4, ip.String())
		}
		switch {
		case !in.Connected:
			ja.LinkStatus = "disconnected"
		case in.TxBps == 0 && in.RxBps == 0:
			ja.LinkStatus = "unavailable"
		default:
			ja.LinkStatus = "ok"
			ja.LinkTxBps, ja.LinkRxBps = u64p(in.TxBps), u64p(in.RxBps)
		}
		if w := in.WiFi; w != nil {
			jw := &jsonWiFi{Status: "unavailable", Reason: strp(w.Reason)}
			if w.Available {
				jw.Status = "ok"
				jw.SSID, jw.BSSID, jw.Band = strp(w.SSID), strp(w.BSSID), strp(w.Band)
				jw.Channel, jw.SignalPct, jw.RSSIDbm = w.Channel, w.SignalPct, w.RSSI
				if w.Standard != NotAvailable {
					jw.Standard = strp(w.Standard)
				}
				jw.TxBps, jw.RxBps = u64p(w.TxBps), u64p(w.RxBps)
			}
			ja.WiFi = jw
			if w.LocationDenied {
				doc.LocationDenied = true
			}
		}
		doc.Adapters = append(doc.Adapters, ja)
	}
	return json.Marshal(doc)
}

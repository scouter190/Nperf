package netinfo

import (
	"errors"
	"sync"
	"time"
)

// RawWiFi son los datos WiFi tal cual los entrega WlanAPI.
type RawWiFi struct {
	SSID       string
	BSSID      string
	PHYType    int
	QualityPct int  // 0-100
	RSSI       *int // dBm; nil = desconocido
	FreqKHz    int  // 0 = desconocida
	TxKbps     uint64
	RxKbps     uint64
	// Err es el código Win32 de la consulta (0 = correcta).
	Err uint32
}

// Source es la fuente inyectable de datos del sistema.
type Source interface {
	Adapters() ([]RawAdapter, error)
	// WiFi consulta los datos WiFi del adaptador con ese GUID (RawAdapter.GUID, el
	// AdapterName "{...}" que T5 rellena desde GetAdaptersAddresses y T6 usa en wlanapi).
	WiFi(guid string) (RawWiFi, error)
}

// WiFiInfo son los datos WiFi interpretados. Si Available es false, Reason
// explica por qué y el resto de campos están vacíos (nunca 0 engañoso).
type WiFiInfo struct {
	Available      bool
	Reason         string
	LocationDenied bool
	SSID           string // "" = no disponible
	BSSID          string
	Band           string // "" = no disponible
	Channel        *int
	SignalPct      *int
	RSSI           *int
	Standard       string // NotAvailable si no se conoce
	TxBps          uint64
	RxBps          uint64
}

// Info es un adaptador con sus datos WiFi (solo si es WiFi y conectado).
type Info struct {
	Adapter
	WiFi *WiFiInfo
}

func wifiInfo(raw RawWiFi, err error) *WiFiInfo {
	code := raw.Err
	if code != 0 || err != nil {
		w := &WiFiInfo{Standard: NotAvailable}
		if code != 0 {
			w.Reason, w.LocationDenied = WLANReason(code)
		} else {
			w.Reason = NotAvailable + ": " + err.Error()
		}
		return w
	}
	w := &WiFiInfo{Available: true, Standard: NotAvailable, TxBps: raw.TxKbps * 1000, RxBps: raw.RxKbps * 1000}
	if ValidSSID(raw.SSID) {
		w.SSID = raw.SSID
	}
	w.BSSID = raw.BSSID
	if band, ch, ok := BandChannel(raw.FreqKHz); ok {
		w.Band = band
		w.Channel = &ch
	}
	w.Standard = Standard(raw.PHYType, w.Band)
	if raw.QualityPct >= 0 && raw.QualityPct <= 100 {
		q := raw.QualityPct
		w.SignalPct = &q
	}
	w.RSSI = raw.RSSI
	return w
}

// Inventory une adaptadores y datos WiFi. Un error de WlanAPI no es fatal:
// conserva tipo y velocidad del adaptador y marca el WiFi como no disponible.
func Inventory(src Source) ([]Info, error) {
	if src == nil {
		return nil, errors.New("sin fuente de datos de red")
	}
	raw, err := src.Adapters()
	if err != nil {
		return nil, err
	}
	adapters := ClassifyAll(raw)
	out := make([]Info, 0, len(adapters))
	for _, a := range adapters {
		in := Info{Adapter: a}
		if a.Kind == KindWiFi {
			if a.Connected {
				in.WiFi = wifiInfo(src.WiFi(a.GUID))
			} else {
				in.WiFi = &WiFiInfo{Reason: ReasonNotConnected, Standard: NotAvailable}
			}
		}
		out = append(out, in)
	}
	return out, nil
}

// Cache limita las consultas al sistema: reutiliza el inventario durante ttl.
type Cache struct {
	src Source
	ttl time.Duration
	now func() time.Time

	mu   sync.Mutex
	at   time.Time
	data []Info
	ok   bool
}

// Cached envuelve src con una caché de ttl y reloj inyectable (nil = time.Now).
func Cached(src Source, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{src: src, ttl: ttl, now: now}
}

// Inventory devuelve el inventario, consultando la fuente si caducó. Los errores no se cachean.
func (c *Cache) Inventory() ([]Info, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.now(); c.ok && t.Sub(c.at) < c.ttl {
		return cloneInfos(c.data), nil
	}
	data, err := Inventory(c.src)
	if err != nil {
		c.ok = false
		return nil, err
	}
	c.data, c.at, c.ok = data, c.now(), true
	return cloneInfos(data), nil
}

func cloneInfos(in []Info) []Info {
	out := make([]Info, len(in))
	copy(out, in)
	return out
}

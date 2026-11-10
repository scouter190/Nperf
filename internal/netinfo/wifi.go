package netinfo

import "fmt"

// Banda WiFi derivada de la frecuencia.
const (
	Band24 = "2.4 GHz"
	Band5  = "5 GHz"
	Band6  = "6 GHz"
)

// Texto cuando un dato no lo entrega Windows.
const NotAvailable = "no disponible"

// Motivos en español de un dato WiFi no disponible.
const (
	ReasonServiceUnavailable = "servicio WLAN no disponible"
	ReasonLocationDenied     = "permiso de ubicación denegado"
	ReasonNotConnected       = "sin conexión"
)

// Códigos Win32 relevantes de WlanAPI.
const (
	errFileNotFound   = 2    // ERROR_FILE_NOT_FOUND (DLL ausente)
	errAccessDenied   = 5    // ERROR_ACCESS_DENIED (ubicación)
	errNotFound       = 1168 // ERROR_NOT_FOUND
	errServiceStopped = 1062 // ERROR_SERVICE_NOT_ACTIVE
	errNotConnected   = 5023 // ERROR_INVALID_STATE (sin conexión)
)

// BandChannel convierte una frecuencia en kHz a banda y canal.
// ok=false si la frecuencia está fuera de los rangos conocidos.
func BandChannel(freqKHz int) (band string, channel int, ok bool) {
	mhz := freqKHz / 1000
	if freqKHz%1000 != 0 || freqKHz <= 0 {
		return "", 0, false
	}
	switch {
	case mhz == 2484:
		return Band24, 14, true
	case mhz >= 2412 && mhz < 2484:
		if (mhz-2407)%5 != 0 {
			return "", 0, false
		}
		return Band24, (mhz - 2407) / 5, true
	case mhz >= 5160 && mhz <= 5885:
		if mhz%5 != 0 {
			return "", 0, false
		}
		return Band5, (mhz - 5000) / 5, true
	case mhz >= 5925 && mhz <= 7125:
		if mhz == 5925 {
			return Band6, 2, true
		}
		if (mhz-5950)%5 != 0 {
			return "", 0, false
		}
		return Band6, (mhz - 5950) / 5, true
	}
	return "", 0, false
}

// Valores de DOT11_PHY_TYPE.
const (
	phyHT  = 7
	phyVHT = 8
	phyHE  = 10
	phyEHT = 11
)

// Standard devuelve el estándar 802.11 y su nombre WiFi a partir del PHY
// type y la banda (para distinguir WiFi 6 de 6E). Desconocido: NotAvailable.
func Standard(phyType int, band string) string {
	switch phyType {
	case phyHT:
		return "802.11n (WiFi 4)"
	case phyVHT:
		return "802.11ac (WiFi 5)"
	case phyHE:
		if band == Band6 {
			return "802.11ax (WiFi 6E)"
		}
		return "802.11ax (WiFi 6)"
	case phyEHT:
		return "802.11be (WiFi 7)"
	}
	return NotAvailable
}

// WLANReason traduce un código Win32 de WlanAPI a un motivo en español.
// locationDenied es true solo para el acceso denegado por ubicación.
// Un código desconocido da un motivo genérico sin aviso.
func WLANReason(code uint32) (reason string, locationDenied bool) {
	switch code {
	case errServiceStopped, errFileNotFound:
		return ReasonServiceUnavailable, false
	case errAccessDenied:
		return ReasonLocationDenied, true
	case errNotConnected, errNotFound:
		return ReasonNotConnected, false
	}
	return fmt.Sprintf("%s (error %d)", NotAvailable, code), false
}

package probe

// Status es el estado portable de una respuesta ICMP.
type Status int

const (
	StatusOther Status = iota
	StatusOK
	StatusTimeout
	StatusTTLExpired
	StatusHostUnreachable
	StatusNetUnreachable
	StatusPortUnreachable
	StatusFragNeeded
)

var statusNames = map[Status]string{
	StatusOther:           "other",
	StatusOK:              "ok",
	StatusTimeout:         "timeout",
	StatusTTLExpired:      "ttl_expired",
	StatusHostUnreachable: "host_unreachable",
	StatusNetUnreachable:  "net_unreachable",
	StatusPortUnreachable: "port_unreachable",
	StatusFragNeeded:      "frag_needed",
}

// String devuelve el nombre estable usado en JSON.
func (s Status) String() string {
	if n, ok := statusNames[s]; ok {
		return n
	}
	return "other"
}

// Códigos Win32 de IP_STATUS (IcmpSendEcho2 / GetLastError).
const (
	CodeSuccess         = 0
	CodeNetUnreachable  = 11002
	CodeHostUnreachable = 11003
	CodePortUnreachable = 11005
	CodePacketTooBig    = 11009
	CodeTimedOut        = 11010
	CodeTTLExpired      = 11013
)

// StatusFromCode traduce un código Win32 a Status y motivo en español.
// El éxito (0) devuelve StatusOK con motivo vacío; cualquier código
// desconocido devuelve StatusOther con motivo "no disponible".
func StatusFromCode(code uint32) (Status, string) {
	switch code {
	case CodeSuccess:
		return StatusOK, ""
	case CodeNetUnreachable:
		return StatusNetUnreachable, "red de destino inalcanzable"
	case CodeHostUnreachable:
		return StatusHostUnreachable, "equipo de destino inalcanzable"
	case CodePortUnreachable:
		return StatusPortUnreachable, "puerto de destino inalcanzable"
	case CodePacketTooBig:
		return StatusFragNeeded, "paquete demasiado grande (requiere fragmentar)"
	case CodeTimedOut:
		return StatusTimeout, "tiempo de espera agotado"
	case CodeTTLExpired:
		return StatusTTLExpired, "TTL agotado en tránsito"
	}
	return StatusOther, "no disponible"
}

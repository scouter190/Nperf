package probe

import (
	"context"
	"net/netip"
	"time"
)

// EchoRequest describe un eco ICMP a enviar.
type EchoRequest struct {
	Dst     netip.Addr
	TTL     int  // 0 = por defecto del sistema
	DF      bool // prohibir fragmentación
	Size    int  // bytes de carga
	Timeout time.Duration
}

// Reply es el resultado de un eco. RTTms solo es válido con Status == StatusOK
// o con respuestas de error que llegaron de un equipo.
type Reply struct {
	Status Status
	From   netip.Addr
	RTTms  float64
	Reason string // motivo en español cuando Status != StatusOK
}

// Echoer envía un eco ICMP y espera su respuesta. Un error devuelto significa
// que la API no pudo usarse; los estados de red van en Reply.Status.
type Echoer interface {
	Echo(ctx context.Context, req EchoRequest) (Reply, error)
}

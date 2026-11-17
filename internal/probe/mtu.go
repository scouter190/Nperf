package probe

import (
	"context"
	"net/netip"
	"time"
)

const (
	mtuMin         = 576
	mtuDefault     = 1500
	mtuHeader      = 28 // IPv4 (20) + ICMP (8)
	mtuMaxProbes   = 20
	mtuTries       = 2
	mtuNoteBlack   = "posible agujero negro de MTU"
	mtuNoteBudget  = "búsqueda detenida por el tope de sondas: el valor es una cota inferior"
	mtuReasonMute  = "el destino no responde a ICMP"
	mtuReasonSmall = "la ruta no admite ni 576 bytes sin fragmentar"
)

// MTUResult es el resultado de la búsqueda del MTU de la ruta.
type MTUResult struct {
	Available    bool   // false = "no disponible" (ver Reason)
	Reason       string // motivo cuando no está disponible
	MTU          int    // mayor paquete IP (bytes) que pasó con DF
	MaxPayload   int    // carga ICMP máxima = MTU - 28
	Probes       int    // sondas enviadas (tope 20)
	InterfaceMTU int    // límite superior usado (MTU de interfaz o 1500)
	Notes        []string
}

// MTU busca por bisección el mayor paquete IP que llega a dst con DF entre
// 576 y ifMTU (0 o fuera de 576-9216 = desconocido → 1500). Cada tamaño se
// reintenta una vez solo ante timeout; como mucho 20 sondas en total.
func MTU(ctx context.Context, e Echoer, dst netip.Addr, ifMTU int, timeout time.Duration) (MTUResult, error) {
	if ifMTU < minPlausibleMTU || ifMTU > maxPlausibleMTU {
		ifMTU = mtuDefault
	}
	res := MTUResult{InterfaceMTU: ifMTU}
	var sawFrag, sawTimeout bool

	// try devuelve si el tamaño (bytes IP) pasa. budget=false: tope agotado.
	try := func(size int) (ok, budget bool, err error) {
		for i := 0; i < mtuTries; i++ {
			if res.Probes >= mtuMaxProbes {
				return false, false, nil
			}
			if err := ctx.Err(); err != nil {
				return false, true, err
			}
			res.Probes++
			r, err := e.Echo(ctx, EchoRequest{Dst: dst, DF: true, Size: size - mtuHeader, Timeout: timeout})
			if err != nil {
				return false, true, err
			}
			switch r.Status {
			case StatusOK:
				return true, true, nil
			case StatusTimeout:
				sawTimeout = true
				continue
			case StatusFragNeeded:
				sawFrag = true
			}
			return false, true, nil
		}
		return false, true, nil
	}

	ok, _, err := try(mtuMin)
	if err != nil {
		return res, err
	}
	if !ok {
		res.Reason = mtuReasonMute
		if sawFrag {
			res.Reason = mtuReasonSmall
		}
		return res, nil
	}
	sawTimeout, sawFrag = false, false
	lo, hi := mtuMin, ifMTU // lo pasa; se busca el mayor que pasa en [lo, hi]
	stopped := false
	if hi > lo {
		ok, budget, err := try(hi)
		if err != nil {
			return res, err
		}
		if ok {
			lo = hi
		} else if !budget {
			stopped = true
		} else {
			hi--
			for lo < hi {
				mid := lo + (hi-lo+1)/2
				ok, budget, err := try(mid)
				if err != nil {
					return res, err
				}
				if !budget {
					stopped = true
					break
				}
				if ok {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
		}
	}
	res.Available = true
	res.MTU = lo
	res.MaxPayload = lo - mtuHeader
	if sawTimeout && !sawFrag && lo < ifMTU {
		res.Notes = append(res.Notes, mtuNoteBlack)
	}
	if stopped {
		res.Notes = append(res.Notes, mtuNoteBudget)
	}
	return res, nil
}

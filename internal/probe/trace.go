package probe

import (
	"context"
	"net/netip"
)

// ProbesPerHop es el número de sondas por salto de traceroute.
const ProbesPerHop = 3

// Hop es un salto del traceroute. From no es válida si ninguna sonda recibió
// respuesta ("*"). Stats cuenta como respondida la sonda que obtuvo eco o
// TTL agotado (respuesta de un router).
type Hop struct {
	TTL    int
	From   netip.Addr
	Probes []Reply
	Stats  Stats
}

// TraceResult es el resultado de un traceroute. Reached indica que el
// destino respondió al eco.
type TraceResult struct {
	Dst     netip.Addr
	Hops    []Hop
	Reached bool
}

// Trace recorre la ruta hacia dst con TTL 1..MaxHops, 3 sondas por salto en
// serie (nunca dos a la vez) y se detiene al llegar al destino. Si ctx se
// cancela devuelve lo medido hasta entonces junto a ctx.Err(). Un error de
// echo.Echo significa que la API ICMP no se pudo usar.
func Trace(ctx context.Context, echo Echoer, dst netip.Addr, opts TraceOptions) (TraceResult, error) {
	res := TraceResult{Dst: dst}
	if err := opts.Validate(); err != nil {
		return res, err
	}
	for ttl := 1; ttl <= opts.MaxHops; ttl++ {
		hop := Hop{TTL: ttl}
		var counted []Reply
		for i := 0; i < ProbesPerHop; i++ {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			r, err := echo.Echo(ctx, EchoRequest{Dst: dst, TTL: ttl, Size: PayloadSize, Timeout: opts.Timeout})
			if err != nil {
				return res, err
			}
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			hop.Probes = append(hop.Probes, r)
			c := r
			switch r.Status {
			case StatusOK:
				res.Reached = true
				fallthrough
			case StatusTTLExpired:
				c.Status = StatusOK
				if !hop.From.IsValid() {
					hop.From = r.From
				}
			}
			counted = append(counted, c)
		}
		hop.Stats = NewStats(counted)
		res.Hops = append(res.Hops, hop)
		if res.Reached {
			break
		}
	}
	return res, nil
}

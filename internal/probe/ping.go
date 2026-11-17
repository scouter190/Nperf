package probe

import (
	"context"
	"net/netip"
)

// Ping envía opts.Count ecos de PayloadSize bytes con opts.Interval entre
// envíos (medido con env.Clock). Los errores ICMP cuentan como pérdida con
// su motivo. Al cancelar ctx vuelve de inmediato con los ecos ya resueltos
// y el error de contexto.
func Ping(ctx context.Context, env Env, dst netip.Addr, opts PingOptions) (Stats, []Reply, error) {
	if err := opts.Validate(); err != nil {
		return Stats{}, nil, err
	}
	replies := make([]Reply, 0, opts.Count)
	for i := 0; i < opts.Count; i++ {
		start := env.Clock.Now()
		type res struct {
			r   Reply
			err error
		}
		ch := make(chan res, 1)
		go func() {
			r, err := env.Echoer.Echo(ctx, EchoRequest{Dst: dst, Size: PayloadSize, Timeout: opts.Timeout})
			ch <- res{r, err}
		}()
		select {
		case <-ctx.Done():
			return NewStats(replies), replies, ctx.Err()
		case x := <-ch:
			if x.err != nil {
				if ctx.Err() != nil {
					return NewStats(replies), replies, ctx.Err()
				}
				return NewStats(replies), replies, x.err
			}
			replies = append(replies, x.r)
		}
		if i == opts.Count-1 {
			break
		}
		wait := opts.Interval - env.Clock.Now().Sub(start)
		if wait < 0 {
			wait = 0
		}
		if err := env.Clock.Sleep(ctx, wait); err != nil {
			return NewStats(replies), replies, err
		}
	}
	return NewStats(replies), replies, ctx.Err()
}

package probe

import (
	"context"
	"net"
	"net/netip"
)

type systemResolver struct{}

func (systemResolver) LookupIP(ctx context.Context, name string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip4", name)
}

// failEchoer devuelve siempre el error de inicialización de ICMP, para que
// "ports" siga funcionando aunque la API ICMP no esté disponible.
type failEchoer struct{ err error }

func (f failEchoer) Echo(context.Context, EchoRequest) (Reply, error) { return Reply{}, f.err }

// System construye el entorno real. close libera la API ICMP; si esta no se
// pudo abrir, Echo devuelve ese error (los puertos siguen funcionando).
func System() (env Env, close func()) {
	env = Env{
		Router:   NewSystemRouter(),
		Resolver: systemResolver{},
		Clock:    RealClock{},
		Dialer:   &net.Dialer{},
	}
	e, err := NewWinEchoer()
	if err != nil {
		env.Echoer = failEchoer{err}
		return env, func() {}
	}
	env.Echoer = e
	return env, func() { e.Close() }
}

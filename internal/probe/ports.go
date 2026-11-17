package probe

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// MaxConcurrentPorts es el máximo de conexiones TCP simultáneas.
const MaxConcurrentPorts = 4

// Dialer abre conexiones TCP (inyectable en tests; *net.Dialer lo cumple).
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// PortState es el estado de un puerto.
type PortState string

const (
	PortOpen     PortState = "open"
	PortClosed   PortState = "closed"
	PortFiltered PortState = "filtered"
	PortError    PortState = "error"
	PortCanceled PortState = "canceled"
)

// PortResult es el resultado de un puerto.
type PortResult struct {
	Port   int       `json:"port"`
	State  PortState `json:"state"`
	Detail string    `json:"detail,omitempty"`
}

const wsaeConnRefused = 10061

func isRefused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var en syscall.Errno
	return errors.As(err, &en) && uintptr(en) == wsaeConnRefused
}

func classify(err error) (PortState, string) {
	if isRefused(err) {
		return PortClosed, ""
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return PortFiltered, "sin respuesta (timeout)"
	}
	return PortError, err.Error()
}

// Ports comprueba cada puerto TCP con una conexión normal: máx. 4 a la vez,
// se cierra al conectar sin enviar datos. El resultado sigue el orden de la
// lista; si se cancela, los puertos sin comprobar quedan como PortCanceled y
// se devuelve ctx.Err().
func Ports(ctx context.Context, d Dialer, ip netip.Addr, ports []int, timeout time.Duration) ([]PortResult, error) {
	res := make([]PortResult, len(ports))
	for i, p := range ports {
		res[i] = PortResult{Port: p, State: PortCanceled}
	}
	sem := make(chan struct{}, MaxConcurrentPorts)
	var wg sync.WaitGroup
loop:
	for i, p := range ports {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		if ctx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func(i, p int) {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			conn, err := d.DialContext(cctx, "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(p)))
			if err == nil {
				conn.Close()
				res[i].State = PortOpen
				return
			}
			if ctx.Err() != nil {
				return // sigue como cancelado
			}
			res[i].State, res[i].Detail = classify(err)
		}(i, p)
	}
	wg.Wait()
	return res, ctx.Err()
}

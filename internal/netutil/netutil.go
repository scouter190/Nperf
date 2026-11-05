// Package netutil abre sockets de escucha con uso exclusivo del puerto.
package netutil

import (
	"context"
	"net"
	"time"
)

// ListenConfig devuelve un net.ListenConfig que, en Windows, activa SO_EXCLUSIVEADDRUSE: sin
// él Windows deja que otro socket (enlazado a una IP concreta o con SO_REUSEADDR) comparta el
// puerto, de modo que no se detecta "puerto ocupado" y otra app podría recibir tráfico.
// keepAlive > 0 fija el keepalive TCP.
func ListenConfig(keepAlive time.Duration) net.ListenConfig {
	return net.ListenConfig{KeepAlive: keepAlive, Control: control}
}

// Listen abre un listener TCP exclusivo.
func Listen(ctx context.Context, addr string, keepAlive time.Duration) (net.Listener, error) {
	lc := ListenConfig(keepAlive)
	return lc.Listen(ctx, "tcp", addr)
}

// ListenUDP abre un socket UDP exclusivo.
func ListenUDP(network string, addr *net.UDPAddr) (*net.UDPConn, error) {
	lc := ListenConfig(0)
	pc, err := lc.ListenPacket(context.Background(), network, addr.String())
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}

// ListenPacket abre un socket de paquetes exclusivo.
func ListenPacket(network, addr string) (net.PacketConn, error) {
	lc := ListenConfig(0)
	return lc.ListenPacket(context.Background(), network, addr)
}

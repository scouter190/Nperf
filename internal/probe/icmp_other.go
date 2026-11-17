//go:build !windows

package probe

import (
	"context"
	"errors"
)

// WinEchoer no está soportado fuera de Windows.
type WinEchoer struct{}

// NewWinEchoer devuelve siempre un error fuera de Windows.
func NewWinEchoer() (*WinEchoer, error) {
	return nil, errors.New("ICMP no soportado en este sistema")
}

// Close no hace nada.
func (*WinEchoer) Close() error { return nil }

// Echo devuelve siempre un error fuera de Windows.
func (*WinEchoer) Echo(context.Context, EchoRequest) (Reply, error) {
	return Reply{}, errors.New("ICMP no soportado en este sistema")
}

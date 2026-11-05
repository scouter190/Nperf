//go:build !windows && !(linux && (amd64 || arm64))

package tcpinfo

import "net"

// Estimated: sin valor, no aplica.
const Estimated = false

// Retransmits no está disponible en este SO.
func Retransmits(*net.TCPConn) (uint64, error) { return 0, ErrUnavailable }

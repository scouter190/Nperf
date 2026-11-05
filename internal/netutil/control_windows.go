//go:build windows

package netutil

import (
	"syscall"
)

// soExclusiveAddrUse = ^SO_REUSEADDR (0x4 -> -5).
const soExclusiveAddrUse = ^0x4

func control(network, address string, c syscall.RawConn) error {
	var serr error
	if err := c.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); err != nil {
		return err
	}
	return serr
}

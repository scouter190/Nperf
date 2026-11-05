//go:build linux && (amd64 || arm64)

package tcpinfo

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

const (
	solTCP  = 6  // SOL_TCP
	tcpInfo = 11 // TCP_INFO

	// Desplazamiento de tcpi_total_retrans (u32) en struct tcp_info: 8 bytes de u8/bitfields
	// y 23 u32 anteriores.
	totalRetransOff = 100
)

// Estimated: en Linux el valor es exacto (segmentos).
const Estimated = false

// Retransmits devuelve tcpi_total_retrans: segmentos retransmitidos en la conexión.
func Retransmits(c *net.TCPConn) (uint64, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var (
		buf  [256]byte
		n    = uint32(len(buf))
		errn syscall.Errno
	)
	if err := rc.Control(func(fd uintptr) {
		_, _, errn = syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, solTCP, tcpInfo,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0)
	}); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if errn != 0 || n < totalRetransOff+4 {
		return 0, fmt.Errorf("%w: getsockopt TCP_INFO: %v", ErrUnavailable, errn)
	}
	return uint64(*(*uint32)(unsafe.Pointer(&buf[totalRetransOff]))), nil
}

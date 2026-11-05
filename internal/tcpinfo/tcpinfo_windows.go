//go:build windows

package tcpinfo

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"
)

// sioTCPInfo = _WSAIORW(IOC_VENDOR, 39). Requiere Windows 10 1703 / Server 2019; no pide
// administrador.
const sioTCPInfo = 0xD8000027

// tcpInfoV0 es TCP_INFO_v0 (mstcpip.h); el compilador de Go reproduce el relleno de C.
type tcpInfoV0 struct {
	State            uint32
	Mss              uint32
	ConnectionTimeMs uint64
	TimestampsOn     uint8
	RttUs            uint32
	MinRttUs         uint32
	BytesInFlight    uint32
	Cwnd             uint32
	SndWnd           uint32
	RcvWnd           uint32
	RcvBuf           uint32
	BytesOut         uint64
	BytesIn          uint64
	BytesReordered   uint32
	BytesRetrans     uint32
	FastRetrans      uint32
	DupAcksIn        uint32
	TimeoutEpisodes  uint32
	SynRetrans       uint8
}

// Estimated: en Windows el valor es una estimación (bytes/MSS).
const Estimated = true

// Retransmits devuelve los segmentos retransmitidos. Windows solo entrega BytesRetrans
// (bytes), así que se aproxima como BytesRetrans/Mss redondeado hacia arriba.
func Retransmits(c *net.TCPConn) (uint64, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var (
		info    tcpInfoV0
		ioErr   error
		version uint32 // entrada: versión 0
		got     uint32
	)
	if err := rc.Control(func(fd uintptr) {
		ioErr = syscall.WSAIoctl(syscall.Handle(fd), sioTCPInfo,
			(*byte)(unsafe.Pointer(&version)), 4,
			(*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &got, nil, 0)
	}); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if ioErr != nil || got < uint32(unsafe.Sizeof(info)) || info.Mss == 0 {
		return 0, fmt.Errorf("%w: SIO_TCP_INFO: %v", ErrUnavailable, ioErr)
	}
	return (uint64(info.BytesRetrans) + uint64(info.Mss) - 1) / uint64(info.Mss), nil
}

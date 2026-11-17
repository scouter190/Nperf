//go:build windows

package probe

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIphlpapi       = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate    = modIphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho2 = modIphlpapi.NewProc("IcmpSendEcho2")
	procIcmpClose     = modIphlpapi.NewProc("IcmpCloseHandle")
)

const ipFlagDF = 0x02 // IP_FLAG_DF

// ipOptionInformation es IP_OPTION_INFORMATION (16 B en amd64).
type ipOptionInformation struct {
	Ttl         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

// icmpEchoReply es ICMP_ECHO_REPLY (40 B en amd64).
type icmpEchoReply struct {
	Address       uint32 // IPAddr en orden de red
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

// WinEchoer implementa Echoer con IcmpSendEcho2 (sin administrador). Un
// handle por diagnóstico: crear con NewWinEchoer y cerrar con Close.
type WinEchoer struct {
	mu     sync.Mutex
	h      uintptr
	closed bool
	wg     sync.WaitGroup
}

// NewWinEchoer abre el handle ICMP.
func NewWinEchoer() (*WinEchoer, error) {
	h, _, err := procIcmpCreate.Call()
	if h == 0 || h == uintptr(windows.InvalidHandle) {
		return nil, fmt.Errorf("IcmpCreateFile: %w", err)
	}
	return &WinEchoer{h: h}, nil
}

// Close espera a los ecos en vuelo hasta su timeout (máx. 5 s), aunque Echo
// vuelva al instante al cancelarse el contexto, y entonces cierra el handle.
func (e *WinEchoer) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()
	e.wg.Wait()
	procIcmpClose.Call(e.h)
	return nil
}

type echoResult struct {
	rep Reply
	err error
}

// Echo envía un eco. IcmpSendEcho2 es síncrono: corre en una goroutine y, si
// ctx se cancela, Echo vuelve de inmediato con ctx.Err() (la llamada termina
// sola al vencer su timeout).
func (e *WinEchoer) Echo(ctx context.Context, req EchoRequest) (Reply, error) {
	if !req.Dst.Is4() {
		return Reply{}, fmt.Errorf("solo IPv4: %v", req.Dst)
	}
	if req.Size < 0 || req.Size > 65000 {
		return Reply{}, fmt.Errorf("tamaño de carga fuera de rango: %d", req.Size)
	}
	if err := ctx.Err(); err != nil {
		return Reply{}, err
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return Reply{}, errors.New("echoer cerrado")
	}
	e.wg.Add(1)
	e.mu.Unlock()

	ch := make(chan echoResult, 1)
	go func() {
		defer e.wg.Done()
		rep, err := sendEcho(e.h, req)
		ch <- echoResult{rep, err}
	}()
	select {
	case r := <-ch:
		return r.rep, r.err
	case <-ctx.Done():
		return Reply{}, ctx.Err()
	}
}

func sendEcho(h uintptr, req EchoRequest) (Reply, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	payload := make([]byte, req.Size)
	for i := range payload {
		payload[i] = byte('a' + i%23)
	}
	var opt ipOptionInformation
	if req.TTL > 0 {
		opt.Ttl = uint8(req.TTL)
	} else {
		opt.Ttl = 128
	}
	if req.DF {
		opt.Flags = ipFlagDF
	}
	replySize := int(unsafe.Sizeof(icmpEchoReply{})) + req.Size + 8 + 16 + 128
	buf := make([]byte, replySize)
	a4 := req.Dst.As4()
	// IPAddr en orden de red: los bytes en memoria son a4[0..3].
	addr := *(*uint32)(unsafe.Pointer(&a4[0]))

	var dataPtr unsafe.Pointer
	if len(payload) > 0 {
		dataPtr = unsafe.Pointer(&payload[0])
	}
	n, _, errno := procIcmpSendEcho2.Call(h, 0, 0, 0,
		uintptr(addr), uintptr(dataPtr), uintptr(len(payload)),
		uintptr(unsafe.Pointer(&opt)), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)), uintptr(timeout.Milliseconds()))
	if n == 0 {
		var code uint32
		if en, ok := errno.(windows.Errno); ok {
			code = uint32(en)
		}
		st, reason := StatusFromCode(code)
		if st == StatusOK { // 0 respuestas sin código: tratar como no disponible
			st, reason = StatusOther, "no disponible"
		}
		return Reply{Status: st, Reason: reason}, nil
	}
	r := (*icmpEchoReply)(unsafe.Pointer(&buf[0]))
	st, reason := StatusFromCode(r.Status)
	from := r.Address
	ip := netip.AddrFrom4(*(*[4]byte)(unsafe.Pointer(&from)))
	return Reply{Status: st, From: ip, RTTms: float64(r.RoundTripTime), Reason: reason}, nil
}

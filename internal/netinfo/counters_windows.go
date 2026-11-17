//go:build windows

package netinfo

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi          = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetIfTable2   = iphlpapi.NewProc("GetIfTable2")
	procFreeMibTable  = iphlpapi.NewProc("FreeMibTable")
	ifOperStatusUp    = uint32(1) // IfOperStatusUp
	maxIfTableEntries = 1 << 16
)

// mibIfRow2 es MIB_IF_ROW2 (netioapi.h), declarado a mano.
type mibIfRow2 struct {
	InterfaceLuid               uint64
	InterfaceIndex              uint32
	InterfaceGuid               windows.GUID
	Alias                       [257]uint16
	Description                 [257]uint16
	PhysicalAddressLength       uint32
	PhysicalAddress             [32]byte
	PermanentPhysicalAddress    [32]byte
	Mtu                         uint32
	Type                        uint32
	TunnelType                  uint32
	MediaType                   uint32
	PhysicalMediumType          uint32
	AccessType                  uint32
	DirectionType               uint32
	InterfaceAndOperStatusFlags uint8
	OperStatus                  uint32
	AdminStatus                 uint32
	MediaConnectState           uint32
	NetworkGuid                 windows.GUID
	ConnectionType              uint32
	TransmitLinkSpeed           uint64
	ReceiveLinkSpeed            uint64
	InOctets                    uint64
	InUcastPkts                 uint64
	InNUcastPkts                uint64
	InDiscards                  uint64
	InErrors                    uint64
	InUnknownProtos             uint64
	InUcastOctets               uint64
	InMulticastOctets           uint64
	InBroadcastOctets           uint64
	OutOctets                   uint64
	OutUcastPkts                uint64
	OutNUcastPkts               uint64
	OutDiscards                 uint64
	OutErrors                   uint64
	OutUcastOctets              uint64
	OutMulticastOctets          uint64
	OutBroadcastOctets          uint64
	OutQLen                     uint64
}

// mibIfTable2 es la cabecera de MIB_IF_TABLE2; las filas siguen alineadas a 8.
type mibIfTable2 struct {
	NumEntries uint32
	_          uint32
}

type winCounters struct {
	get  func() (unsafe.Pointer, error) // devuelve el puntero a MIB_IF_TABLE2
	free func(table unsafe.Pointer)     // FreeMibTable
}

// NewSystemCounters devuelve la fuente real de contadores (GetIfTable2, sin admin).
func NewSystemCounters() CounterSource {
	return winCounters{get: getIfTable2, free: freeMibTable}
}

func getIfTable2() (unsafe.Pointer, error) {
	var p unsafe.Pointer
	r, _, _ := procGetIfTable2.Call(uintptr(unsafe.Pointer(&p)))
	if r != 0 {
		return nil, fmt.Errorf("GetIfTable2: error %d", r)
	}
	return p, nil
}

func freeMibTable(p unsafe.Pointer) { procFreeMibTable.Call(uintptr(p)) }

func (w winCounters) Counters() ([]IfCounter, error) {
	p, err := w.get()
	if err != nil {
		return nil, err
	}
	defer w.free(p)
	return parseIfTable(p), nil
}

// parseIfTable copia los datos de la tabla; p debe seguir válido durante la llamada.
func parseIfTable(p unsafe.Pointer) []IfCounter {
	if p == nil {
		return nil
	}
	hdr := (*mibIfTable2)(p)
	n := int(hdr.NumEntries)
	if n > maxIfTableEntries {
		n = maxIfTableEntries
	}
	rows := unsafe.Slice((*mibIfRow2)(unsafe.Add(p, unsafe.Sizeof(*hdr))), n)
	out := make([]IfCounter, 0, n)
	for i := range rows {
		r := &rows[i]
		out = append(out, IfCounter{
			GUID: r.InterfaceGuid.String(),
			In:   r.InOctets,
			Out:  r.OutOctets,
			Up:   r.OperStatus == ifOperStatusUp,
		})
	}
	return out
}

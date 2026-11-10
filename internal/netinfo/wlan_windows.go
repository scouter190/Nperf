//go:build windows

package netinfo

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wlanapi                = windows.NewLazySystemDLL("wlanapi.dll")
	procWlanOpenHandle     = wlanapi.NewProc("WlanOpenHandle")
	procWlanCloseHandle    = wlanapi.NewProc("WlanCloseHandle")
	procWlanEnumInterfaces = wlanapi.NewProc("WlanEnumInterfaces")
	procWlanQueryInterface = wlanapi.NewProc("WlanQueryInterface")
	procWlanGetNetworkBss  = wlanapi.NewProc("WlanGetNetworkBssList")
	procWlanFreeMemory     = wlanapi.NewProc("WlanFreeMemory")
)

const (
	wlanClientVersion2       = 2
	wlanIntfOpcodeCurrentCon = 7 // wlan_intf_opcode_current_connection
	wlanStateConnected       = 1 // wlan_interface_state_connected
	dot11BssTypeAny          = 3
)

type dot11SSID struct {
	Len  uint32
	SSID [32]byte
}

type wlanAssociationAttributes struct {
	SSID          dot11SSID
	BssType       uint32
	BSSID         [6]byte
	PHYType       uint32
	PHYIndex      uint32
	SignalQuality uint32
	RxRate        uint32
	TxRate        uint32
}

type wlanSecurityAttributes struct {
	SecurityEnabled uint32
	OneXEnabled     uint32
	AuthAlgorithm   uint32
	CipherAlgorithm uint32
}

type wlanConnectionAttributes struct {
	State       uint32
	Mode        uint32
	ProfileName [256]uint16
	Assoc       wlanAssociationAttributes
	Security    wlanSecurityAttributes
}

type wlanRateSet struct {
	Length uint32
	Rates  [126]uint16
}

type wlanBSSEntry struct {
	SSID          dot11SSID
	PHYID         uint32
	BSSID         [6]byte
	BssType       uint32
	BssPHYType    uint32
	RSSI          int32
	LinkQuality   uint32
	InRegDomain   uint8
	BeaconPeriod  uint16
	Timestamp     uint64
	HostTimestamp uint64
	Capability    uint16
	CenterFreqKHz uint32
	Rates         wlanRateSet
	IEOffset      uint32
	IESize        uint32
}

type wlanBSSListHeader struct {
	TotalSize uint32
	NumItems  uint32
}

type wlanInterfaceInfo struct {
	GUID        windows.GUID
	Description [256]uint16
	State       uint32
}

type wlanInterfaceInfoListHeader struct {
	NumItems uint32
	Index    uint32
}

func wlanErr(code uintptr) uint32 { return uint32(code) }

func wlanFree(p unsafe.Pointer) {
	if p != nil {
		procWlanFreeMemory.Call(uintptr(p))
	}
}

// queryWiFi consulta los datos WiFi de la interfaz con ese GUID ("{...}").
// Nunca falla de forma fatal: los errores van en RawWiFi.Err (código Win32).
func queryWiFi(guid string) (raw RawWiFi) {
	defer func() {
		if r := recover(); r != nil {
			raw = RawWiFi{Err: errNotFound}
		}
	}()
	g, err := windows.GUIDFromString(guid)
	if err != nil {
		return RawWiFi{Err: errNotFound}
	}
	if err := wlanapi.Load(); err != nil {
		return RawWiFi{Err: errFileNotFound}
	}
	var negotiated uint32
	var handle windows.Handle
	if r, _, _ := procWlanOpenHandle.Call(wlanClientVersion2, 0,
		uintptr(unsafe.Pointer(&negotiated)), uintptr(unsafe.Pointer(&handle))); r != 0 {
		return RawWiFi{Err: wlanErr(r)}
	}
	defer procWlanCloseHandle.Call(uintptr(handle), 0)

	var size, valueType uint32
	var data unsafe.Pointer
	r, _, _ := procWlanQueryInterface.Call(uintptr(handle), uintptr(unsafe.Pointer(&g)),
		wlanIntfOpcodeCurrentCon, 0, uintptr(unsafe.Pointer(&size)),
		uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&valueType)))
	if r != 0 {
		return RawWiFi{Err: wlanErr(r)}
	}
	defer wlanFree(data)
	if data == nil || uintptr(size) < unsafe.Sizeof(wlanConnectionAttributes{}) {
		return RawWiFi{Err: errNotFound}
	}
	ca := *(*wlanConnectionAttributes)(data)
	if ca.State != wlanStateConnected {
		return RawWiFi{Err: errNotConnected}
	}
	a := ca.Assoc
	n := a.SSID.Len
	if n > 32 {
		n = 32
	}
	raw = RawWiFi{
		SSID:       string(a.SSID.SSID[:n]),
		BSSID:      fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", a.BSSID[0], a.BSSID[1], a.BSSID[2], a.BSSID[3], a.BSSID[4], a.BSSID[5]),
		PHYType:    int(a.PHYType),
		QualityPct: int(a.SignalQuality),
		TxKbps:     uint64(a.TxRate),
		RxKbps:     uint64(a.RxRate),
	}
	bssFreq(handle, &g, ca.Security.SecurityEnabled != 0, &a, &raw)
	return raw
}

// bssFreq rellena frecuencia y RSSI del BSS actual; un fallo deja esos datos vacíos.
// Usa el valor de seguridad de la conexión y, sin coincidencia, el opuesto.
func bssFreq(h windows.Handle, g *windows.GUID, sec bool, a *wlanAssociationAttributes, raw *RawWiFi) {
	if !bssLookup(h, g, sec, a, raw) {
		bssLookup(h, g, !sec, a, raw)
	}
}

func bssLookup(h windows.Handle, g *windows.GUID, sec bool, a *wlanAssociationAttributes, raw *RawWiFi) bool {
	ssid := a.SSID
	var secArg uintptr
	if sec {
		secArg = 1
	}
	var list unsafe.Pointer
	r, _, _ := procWlanGetNetworkBss.Call(uintptr(h), uintptr(unsafe.Pointer(g)),
		uintptr(unsafe.Pointer(&ssid)), dot11BssTypeAny, secArg, 0, uintptr(unsafe.Pointer(&list)))
	if r != 0 || list == nil {
		return false
	}
	defer wlanFree(list)
	freq, rssi, ok := findBSS(list, a.BSSID)
	if ok {
		raw.FreqKHz = freq
		raw.RSSI = &rssi
	}
	return ok
}

// findBSS busca el BSSID en una WLAN_BSS_LIST y devuelve su frecuencia (kHz) y RSSI.
func findBSS(list unsafe.Pointer, bssid [6]byte) (freqKHz, rssi int, ok bool) {
	hdr := (*wlanBSSListHeader)(list)
	if hdr.NumItems > 1024 {
		return 0, 0, false
	}
	base := unsafe.Add(list, unsafe.Sizeof(*hdr))
	for i := uintptr(0); i < uintptr(hdr.NumItems); i++ {
		e := (*wlanBSSEntry)(unsafe.Add(base, i*unsafe.Sizeof(wlanBSSEntry{})))
		if e.BSSID == bssid {
			return int(e.CenterFreqKHz), int(e.RSSI), true
		}
	}
	return 0, 0, false
}

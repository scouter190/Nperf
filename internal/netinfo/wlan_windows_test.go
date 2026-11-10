//go:build windows

package netinfo

import (
	"testing"
	"unsafe"
)

func TestWlanStructLayout(t *testing.T) {
	var ca wlanConnectionAttributes
	var be wlanBSSEntry
	var ii wlanInterfaceInfo
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"dot11SSID", unsafe.Sizeof(dot11SSID{}), 36},
		{"association", unsafe.Sizeof(wlanAssociationAttributes{}), 68},
		{"security", unsafe.Sizeof(wlanSecurityAttributes{}), 16},
		{"connection", unsafe.Sizeof(ca), 604},
		{"connection.Assoc", unsafe.Offsetof(ca.Assoc), 520},
		{"bss", unsafe.Sizeof(be), 360},
		{"bss.RSSI", unsafe.Offsetof(be.RSSI), 56},
		{"bss.Timestamp", unsafe.Offsetof(be.Timestamp), 72},
		{"bss.CenterFreq", unsafe.Offsetof(be.CenterFreqKHz), 92},
		{"bss.Rates", unsafe.Offsetof(be.Rates), 96},
		{"bss.IEOffset", unsafe.Offsetof(be.IEOffset), 352},
		{"bssHeader", unsafe.Sizeof(wlanBSSListHeader{}), 8},
		{"interface", unsafe.Sizeof(ii), 532},
		{"interfaceHeader", unsafe.Sizeof(wlanInterfaceInfoListHeader{}), 8},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, quiero %d", c.name, c.got, c.want)
		}
	}
}

func TestQueryWiFiNoPanic(t *testing.T) {
	raw := queryWiFi("{00000000-0000-0000-0000-000000000000}")
	if raw.Err == 0 {
		t.Fatalf("GUID inexistente no debería dar datos: %+v", raw)
	}
	if queryWiFi("no-es-guid").Err == 0 {
		t.Fatal("GUID inválido debe dar error")
	}
}

func TestFindBSS(t *testing.T) {
	buf := make([]byte, 8+2*unsafe.Sizeof(wlanBSSEntry{}))
	*(*wlanBSSListHeader)(unsafe.Pointer(&buf[0])) = wlanBSSListHeader{NumItems: 2}
	sz := unsafe.Sizeof(wlanBSSEntry{})
	e0 := (*wlanBSSEntry)(unsafe.Pointer(&buf[8]))
	e1 := (*wlanBSSEntry)(unsafe.Pointer(&buf[8+sz]))
	e0.BSSID = [6]byte{1, 2, 3, 4, 5, 6}
	e1.BSSID = [6]byte{6, 5, 4, 3, 2, 1}
	e1.CenterFreqKHz, e1.RSSI = 5180000, -55
	f, r, ok := findBSS(unsafe.Pointer(&buf[0]), [6]byte{6, 5, 4, 3, 2, 1})
	if !ok || f != 5180000 || r != -55 {
		t.Fatalf("findBSS = %d %d %v", f, r, ok)
	}
	if _, _, ok := findBSS(unsafe.Pointer(&buf[0]), [6]byte{9}); ok {
		t.Fatal("BSSID ajeno no debe coincidir")
	}
}

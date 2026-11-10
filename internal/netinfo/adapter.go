// Package netinfo detecta los adaptadores de red y su tipo de enlace.
package netinfo

import (
	"net/netip"
	"strings"
)

// Valores de IfType (IANA) usados por la clasificación.
const (
	IfTypeEthernet = 6
	IfTypeLoopback = 24
	IfTypeWiFi     = 71
)

// OperStatusUp es IfOperStatusUp de Windows.
const OperStatusUp = 1

// Kind es el tipo de enlace de un adaptador.
type Kind string

const (
	KindEthernet Kind = "eth"
	KindWiFi     Kind = "wifi"
	KindOther    Kind = "other"
)

// RawAdapter son los datos tal cual los entrega el sistema.
type RawAdapter struct {
	Name string
	// GUID es el AdapterName "{...}" de GetAdaptersAddresses (clave de wlanapi).
	GUID        string
	Description string
	MAC         string
	IPv4        []netip.Addr
	IfType      uint32
	OperStatus  uint32
	TxBps       uint64
	RxBps       uint64
	// Physical es ConnectorPresent si está disponible (nil = desconocido).
	Physical *bool
}

// Adapter es un adaptador clasificado.
type Adapter struct {
	Name        string
	GUID        string
	Description string
	MAC         string
	IPv4        []netip.Addr
	Kind        Kind
	Connected   bool
	TxBps       uint64
	RxBps       uint64
}

var virtualMarks = []string{"hyper-v", "virtual", "vpn", "tap", "wsl", "vmware", "virtualbox"}

func isVirtual(r RawAdapter) bool {
	if r.Physical != nil && !*r.Physical {
		return true
	}
	d := strings.ToLower(r.Description)
	for _, m := range virtualMarks {
		if strings.Contains(d, m) {
			return true
		}
	}
	return false
}

// Classify devuelve el tipo del adaptador; ok es false si debe omitirse (loopback).
func Classify(r RawAdapter) (Kind, bool) {
	switch r.IfType {
	case IfTypeLoopback:
		return "", false
	case IfTypeWiFi:
		return KindWiFi, true
	case IfTypeEthernet:
		if isVirtual(r) {
			return KindOther, true
		}
		return KindEthernet, true
	}
	return KindOther, true
}

// ClassifyAll aplica la clasificación a una lista, omitiendo el loopback.
func ClassifyAll(raw []RawAdapter) []Adapter {
	out := make([]Adapter, 0, len(raw))
	for _, r := range raw {
		k, ok := Classify(r)
		if !ok {
			continue
		}
		out = append(out, Adapter{
			Name: r.Name, GUID: r.GUID, Description: r.Description, MAC: r.MAC,
			IPv4: r.IPv4, Kind: k, Connected: r.OperStatus == OperStatusUp,
			TxBps: r.TxBps, RxBps: r.RxBps,
		})
	}
	return out
}

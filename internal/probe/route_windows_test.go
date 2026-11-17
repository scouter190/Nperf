//go:build windows

package probe

import (
	"errors"
	"net/netip"
	"runtime"
	"testing"
	"unsafe"
)

func TestRouteStructSizes(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("tamaños verificados para amd64")
	}
	if n := unsafe.Sizeof(sockaddrInet{}); n != 28 {
		t.Errorf("sockaddrInet=%d, quiero 28", n)
	}
	if n := unsafe.Sizeof(mibIPForwardRow2{}); n != 104 {
		t.Errorf("mibIPForwardRow2=%d, quiero 104", n)
	}
	var r mibIPForwardRow2
	if o := unsafe.Offsetof(r.NextHop); o != 44 {
		t.Errorf("offset NextHop=%d, quiero 44", o)
	}
	if o := unsafe.Offsetof(r.Origin); o != 100 {
		t.Errorf("offset Origin=%d, quiero 100", o)
	}
}

func TestRouteLoopback(t *testing.T) {
	r, err := NewSystemRouter().Route(netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Source.IsValid() {
		t.Errorf("origen inválido: %+v", r)
	}
}

func TestGatewayWindows(t *testing.T) {
	gw, err := NewSystemRouter().Gateway()
	if err != nil {
		if errors.Is(err, ErrNoGateway) {
			return
		}
		t.Fatal(err)
	}
	if !gw.Is4() || gw.IsUnspecified() {
		t.Errorf("puerta de enlace inválida: %v", gw)
	}
}

func TestLocalPrefixesNoLoopback(t *testing.T) {
	ps, err := NewSystemRouter().LocalPrefixes()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.Addr().IsLoopback() {
			t.Errorf("prefijo loopback: %v", p)
		}
	}
}

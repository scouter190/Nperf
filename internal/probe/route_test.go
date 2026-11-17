package probe

import (
	"errors"
	"net/netip"
	"testing"
)

func TestNormalizeMTU(t *testing.T) {
	for _, c := range []struct {
		in   uint32
		want int
	}{{0, 0}, {575, 0}, {576, 576}, {1500, 1500}, {9216, 9216}, {9217, 0}, {4294967295, 0}} {
		if got := normalizeMTU(c.in); got != c.want {
			t.Errorf("normalizeMTU(%d)=%d, quiero %d", c.in, got, c.want)
		}
	}
}

func TestGatewayFromRoute(t *testing.T) {
	gw := netip.MustParseAddr("192.168.1.1")
	if got, err := gatewayFromRoute(Route{NextHop: gw}); err != nil || got != gw {
		t.Errorf("got %v, %v", got, err)
	}
	for _, nh := range []netip.Addr{{}, netip.IPv4Unspecified(), netip.MustParseAddr("::1")} {
		if _, err := gatewayFromRoute(Route{NextHop: nh}); !errors.Is(err, ErrNoGateway) {
			t.Errorf("NextHop %v: err=%v", nh, err)
		}
	}
}

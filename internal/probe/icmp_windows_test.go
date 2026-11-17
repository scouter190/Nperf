//go:build windows

package probe

import (
	"context"
	"net/netip"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestICMPStructSizes(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("tamaños verificados en amd64")
	}
	if s := unsafe.Sizeof(ipOptionInformation{}); s != 16 {
		t.Errorf("IP_OPTION_INFORMATION = %d, quiero 16", s)
	}
	var r icmpEchoReply
	if s := unsafe.Sizeof(r); s != 40 {
		t.Errorf("ICMP_ECHO_REPLY = %d, quiero 40", s)
	}
	if o := unsafe.Offsetof(r.Data); o != 16 {
		t.Errorf("offset Data = %d, quiero 16", o)
	}
	if o := unsafe.Offsetof(r.Options); o != 24 {
		t.Errorf("offset Options = %d, quiero 24", o)
	}
}

func TestWinEchoLoopback(t *testing.T) {
	e, err := NewWinEchoer()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	lo := netip.MustParseAddr("127.0.0.1")
	for _, req := range []EchoRequest{
		{Dst: lo, Size: 32, Timeout: 2 * time.Second},
		{Dst: lo, Size: 1472, DF: true, Timeout: 2 * time.Second},
	} {
		rep, err := e.Echo(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Status != StatusOK {
			t.Errorf("size %d DF %v: status %v (%s)", req.Size, req.DF, rep.Status, rep.Reason)
		}
	}
}

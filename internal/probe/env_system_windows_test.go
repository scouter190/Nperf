//go:build windows

package probe

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestSystemLoopback(t *testing.T) {
	env, closeEnv := System()
	defer closeEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lo := netip.MustParseAddr("127.0.0.1")

	po := PingOptions{Count: 2, Interval: MinInterval, Timeout: DefaultTimeout}
	st, _, err := Ping(ctx, env, lo, po)
	if err != nil || st.Received == 0 {
		t.Fatalf("ping: %v %+v", err, st)
	}
	if _, err := Trace(ctx, env.Echoer, lo, TraceOptions{MaxHops: 3, Timeout: DefaultTimeout}); err != nil {
		t.Fatalf("trace: %v", err)
	}
	ifMTU := 0
	if r, err := env.Router.Route(lo); err == nil {
		ifMTU = r.MTU
	}
	if _, err := MTU(ctx, env.Echoer, lo, ifMTU, DefaultTimeout); err != nil {
		t.Fatalf("mtu: %v", err)
	}
}

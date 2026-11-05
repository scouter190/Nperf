package server_test

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"nperf/internal/discovery"
	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/testutil"
	"nperf/internal/version"
)

// El beacon con Status = srv.Status anuncia ocupado con la cola correcta y disponible al terminar.
func TestBeaconReflectsServerStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, uc := testutil.Listen(t)
	srv := server.New(server.WithUDPConn(uc))
	go func() { testutil.ServeErr(t, ctx, srv.Serve(ctx, ln)) }()

	var mu sync.Mutex
	var got []discovery.Announce
	ticks := make(chan time.Time)
	b := &discovery.Beacon{
		Alias: "pc", InstanceID: "0123456789abcdef", MeasurePort: 5300, Ticks: ticks,
		Ifaces: func() ([]discovery.Iface, error) {
			return []discovery.Iface{{Name: "a", IP: netip.MustParseAddr("192.168.1.10"), Broadcast: netip.MustParseAddr("192.168.1.255")}}, nil
		},
		Status: srv.Status,
		Send: func(_ netip.Addr, _ netip.AddrPort, data []byte) error {
			a, err := discovery.Parse(data)
			if err != nil {
				t.Error(err)
				return nil
			}
			mu.Lock()
			got = append(got, a)
			mu.Unlock()
			return nil
		},
	}
	go b.Run(ctx)
	waitN := func(n int) discovery.Announce {
		t.Helper()
		for i := 0; i < 3000; i++ {
			mu.Lock()
			if len(got) >= n {
				a := got[n-1]
				mu.Unlock()
				return a
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("esperaba %d anuncios", n)
		return discovery.Announce{}
	}
	if a := waitN(1); a.State != discovery.StateAvailable || a.Queue != 0 {
		t.Fatalf("inicial: %+v", a)
	}

	open := func() (net.Conn, *proto.Codec) {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(10 * time.Second))
		codec := proto.NewCodec(c)
		if _, err := proto.ClientHandshake(codec, "", version.Current); err != nil {
			t.Fatal(err)
		}
		if err := codec.Write(proto.Message{Type: proto.TypeTurn}); err != nil {
			t.Fatal(err)
		}
		return c, codec
	}
	a, ac := open()
	for {
		m, err := ac.Read()
		if err != nil {
			t.Fatal(err)
		}
		if m.Type == proto.TypeReady {
			break
		}
	}
	w, _ := open() // queda en espera
	defer w.Close()
	for i := 0; i < 2000; i++ {
		if _, q := srv.Status(); q == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	ticks <- time.Time{}
	if an := waitN(2); an.State != discovery.StateBusy || an.Queue != 1 {
		t.Fatalf("ocupado: %+v", an)
	}

	a.Close()
	w.Close()
	for i := 0; i < 3000; i++ {
		if s, q := srv.Status(); s == proto.StateAvailable && q == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	ticks <- time.Time{}
	if an := waitN(3); an.State != discovery.StateAvailable || an.Queue != 0 {
		t.Fatalf("final: %+v", an)
	}
}

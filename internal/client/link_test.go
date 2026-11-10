package client_test

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/testutil"
)

func startLink(t *testing.T, opts ...server.Option) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ln, uc := testutil.Listen(t)
	go func() { testutil.ServeErr(t, ctx, server.New(append(opts, server.WithUDPConn(uc))...).Serve(ctx, ln)) }()
	return ln.Addr().String()
}

func TestLinkExchange(t *testing.T) {
	var srvIP string
	addr := startLink(t, server.WithLinkProvider(func(l net.Addr) *proto.Link {
		srvIP = l.(*net.TCPAddr).IP.String()
		return &proto.Link{Kind: "eth", TxBps: 1e9, RxBps: 1e9}
	}))
	var cliIP netip.Addr
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond,
		LocalLink: func(ip netip.Addr) *proto.Link {
			cliIP = ip
			return &proto.Link{Kind: "wifi", TxBps: 5e8, RxBps: 6e8, SSID: "casa", Band: "5 GHz", Standard: "WiFi 6", SignalPct: 80}
		}})
	if err != nil {
		t.Fatal(err)
	}
	if cliIP != netip.MustParseAddr("127.0.0.1") || srvIP != "127.0.0.1" {
		t.Fatalf("IP local incorrecta: cliente %v servidor %v", cliIP, srvIP)
	}
	if res.RemoteLink == nil || res.RemoteLink.Kind != "eth" || res.RemoteLink.TxBps != 1e9 {
		t.Fatalf("RemoteLink: %+v", res.RemoteLink)
	}
	if res.LocalLink == nil || res.LocalLink.SSID != "casa" || res.LocalLink.SignalPct != 80 {
		t.Fatalf("LocalLink: %+v", res.LocalLink)
	}
}

func TestLinkPeerWithoutLink(t *testing.T) {
	addr := startLink(t) // servidor sin proveedor = versión previa
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond,
		LocalLink: func(netip.Addr) *proto.Link { return &proto.Link{Kind: "eth"} }})
	if err != nil || res.Fwd.Mbps <= 0 || res.RemoteLink != nil {
		t.Fatalf("%+v %v", res, err)
	}
	// Cliente sin LocalLink frente a servidor con proveedor.
	addr = startLink(t, server.WithLinkProvider(func(net.Addr) *proto.Link { return &proto.Link{Kind: "eth"} }))
	res, err = client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond})
	if err != nil || res.Fwd.Mbps <= 0 || res.LocalLink != nil || res.RemoteLink == nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestLinkNotInHandshakeNorSensitive(t *testing.T) {
	for _, m := range []proto.Message{
		{Type: proto.TypeHello, Version: "1", Nonce: "n"},
		{Type: proto.TypeAuth, MAC: "x", Session: "s"},
		{Type: proto.TypeTurn},
		{Type: proto.TypeReady},
	} {
		b, _ := json.Marshal(m)
		if strings.Contains(string(b), "link") {
			t.Fatalf("%s lleva link: %s", m.Type, b)
		}
	}
	b, _ := json.Marshal(proto.Message{Type: proto.TypeReady, Link: &proto.Link{Kind: "wifi", TxBps: 1, RxBps: 1, SSID: "s", Band: "b", Standard: "w", SignalPct: 1}})
	for _, w := range []string{"bssid", "mac"} {
		if strings.Contains(strings.ToLower(string(b)), w) {
			t.Fatalf("aparece %q: %s", w, b)
		}
	}
}

func TestLinkHostilePeer(t *testing.T) {
	hostile := &proto.Link{Kind: "x", TxBps: -1, SSID: "a\x01", SignalPct: 999, Band: strings.Repeat("b", 99)}
	addr := startLink(t, server.WithLinkProvider(func(net.Addr) *proto.Link { return hostile }))
	res, err := client.Run(context.Background(), addr, client.Options{Duration: 200 * time.Millisecond,
		LocalLink: func(netip.Addr) *proto.Link { h := *hostile; return &h }})
	if err != nil || res.RemoteLink == nil || *res.RemoteLink != (proto.Link{}) {
		t.Fatalf("%+v %v", res.RemoteLink, err)
	}
}

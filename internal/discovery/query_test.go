package discovery

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestParseSubnetRejects(t *testing.T) {
	cases := map[string]string{
		"10.0.0.0/23":   "demasiado amplia",
		"10.0.0.0/16":   "demasiado amplia",
		"0.0.0.0/0":     "demasiado amplia",
		"fe80::1":       "IPv6",
		"2001:db8::/64": "IPv6",
		"hola":          "válid",
		"1.2.3.4/33":    "válid",
		"1.2.3":         "válid",
		"":              "vacía",
	}
	for in, want := range cases {
		_, err := ParseSubnet(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, quería %q", in, err, want)
		}
	}
}

func TestParseSubnetExpand(t *testing.T) {
	cases := []struct {
		in         string
		n          int
		first, end string
	}{
		{"192.168.1.0/24", 254, "192.168.1.1", "192.168.1.254"},
		{"192.168.1.77/24", 254, "192.168.1.1", "192.168.1.254"},
		{"10.0.0.0/30", 2, "10.0.0.1", "10.0.0.2"},
		{"10.0.0.4/31", 2, "10.0.0.4", "10.0.0.5"},
		{"10.0.0.9/32", 1, "10.0.0.9", "10.0.0.9"},
		{"10.0.0.9", 1, "10.0.0.9", "10.0.0.9"},
		{"255.255.255.0/24", 254, "255.255.255.1", "255.255.255.254"},
	}
	for _, c := range cases {
		got, err := ParseSubnet(c.in)
		if err != nil || len(got) != c.n || got[0].String() != c.first || got[len(got)-1].String() != c.end {
			t.Errorf("%s: %v %v", c.in, got, err)
		}
	}
}

func testAnnounce(t *testing.T) Announce {
	id, err := NewInstanceID()
	if err != nil {
		t.Fatal(err)
	}
	return Announce{Alias: "pc", IP: netip.MustParseAddr("127.0.0.1"), Port: 5300, State: StateAvailable, InstanceID: id}
}

func listen(t *testing.T) *net.UDPConn {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skip(err)
	}
	return c
}

func TestQueryReceivesAnnounce(t *testing.T) {
	srv := listen(t)
	self := testAnnounce(t)
	l := &Listener{Conn: srv, Self: func(netip.Addr) (Announce, bool) { return self, true }}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sdone := make(chan error, 1)
	go func() { sdone <- l.Serve(ctx) }()

	cli := listen(t)
	got := make(chan Announce, 1)
	cl := &Listener{Conn: cli, OnAnnounce: func(a Announce, _ netip.AddrPort) { got <- a }}
	cdone := make(chan error, 1)
	go func() { cdone <- cl.Serve(ctx) }()

	port := srv.LocalAddr().(*net.UDPAddr).Port
	if err := SendQueries(ctx, cli, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, port); err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-got:
		if a.InstanceID != self.InstanceID || a.Alias != "pc" {
			t.Fatalf("anuncio inesperado: %+v", a)
		}
	case <-ctx.Done():
		t.Fatal("sin respuesta")
	}
	cancel()
	<-sdone
	<-cdone
}

func TestSendQueriesCancelled(t *testing.T) {
	c := listen(t)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := SendQueries(ctx, c, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, DefaultDiscoveryPort); err == nil {
		t.Fatal("esperaba error por ctx cancelado")
	}
}

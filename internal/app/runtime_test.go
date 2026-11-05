package app

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/testutil"
)

func loIfaces() ([]discovery.Iface, error) {
	lo := netip.MustParseAddr("127.0.0.1")
	return []discovery.Iface{{Name: "lo", IP: lo, Broadcast: lo}}, nil
}

func udpConn(t *testing.T) net.PacketConn {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func portOf(a net.Addr) int { return a.(*net.UDPAddr).Port }

func cfgDir(t *testing.T, c config.Config) string {
	t.Helper()
	d := t.TempDir()
	if err := config.Save(d, c); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRuntimeMeasures(t *testing.T) {
	ln, uc := testutil.Listen(t)
	c := config.Defaults()
	c.Key = "clave"
	r, err := Start(context.Background(), RuntimeOptions{
		ConfigDir: cfgDir(t, c), Alias: "A", MeasureListener: ln, MeasureUDP: uc,
		DiscoveryConn: udpConn(t), Ifaces: loIfaces, Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if st, _ := r.SelfState(); st == "" {
		t.Fatal("estado vacío")
	}
	res, err := client.Run(context.Background(), ln.Addr().String(), client.Options{
		Duration: time.Second, Proto: "tcp", Direction: "fwd", Key: "clave",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fwd.Measured {
		t.Fatalf("sin medición: %+v", res)
	}
}

func TestRuntimeMeasurePortBusy(t *testing.T) {
	ln, uc := testutil.Listen(t)
	defer ln.Close()
	defer uc.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	pc := udpConn(t)
	_, err := Start(context.Background(), RuntimeOptions{MeasurePort: port, DiscoveryConn: pc})
	if err == nil {
		t.Fatal("se esperaba error")
	}
	want := "el puerto " + itoa(port) + " ya está en uso: ¿hay otra instancia de NPerf o iperf?"
	if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("mensaje: %v", err)
	}
	// pc debe haberse cerrado: escribir falla.
	if _, werr := pc.WriteTo([]byte("x"), pc.LocalAddr()); werr == nil {
		t.Fatal("el socket de descubrimiento quedó abierto")
	}
}

func TestRuntimeDiscoveryPortBusy(t *testing.T) {
	busy := udpConn(t)
	defer busy.Close()
	ln, uc := testutil.Listen(t)
	_, err := Start(context.Background(), RuntimeOptions{
		MeasureListener: ln, MeasureUDP: uc, DiscoveryPort: portOf(busy.LocalAddr()),
	})
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if _, derr := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); derr == nil {
		t.Fatal("el puerto de medición quedó abierto")
	}
}

func TestRuntimeDiscoversPeers(t *testing.T) {
	pcA, pcB := udpConn(t), udpConn(t)
	mk := func(alias string, pc net.PacketConn, dst net.PacketConn) *Runtime {
		ln, uc := testutil.Listen(t)
		send := func(_ netip.Addr, _ netip.AddrPort, data []byte) error {
			_, err := pc.WriteTo(data, dst.LocalAddr())
			return err
		}
		r, err := Start(context.Background(), RuntimeOptions{
			Alias: alias, MeasureListener: ln, MeasureUDP: uc, DiscoveryConn: pc,
			Ifaces: loIfaces, Send: send, Interval: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a := mk("A", pcA, pcB)
	b := mk("B", pcB, pcA)
	deadline := time.Now().Add(5 * time.Second)
	for {
		pa, pb := a.Table.Snapshot(), b.Table.Snapshot()
		if len(pa) == 1 && pa[0].InstanceID == b.SelfID && len(pb) == 1 && pb[0].InstanceID == a.SelfID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no se descubrieron: %v %v", pa, pb)
		}
		time.Sleep(20 * time.Millisecond)
	}
	a.Stop()
	b.Stop()
}

func TestRuntimeStopReleases(t *testing.T) {
	ln, uc := testutil.Listen(t)
	pc := udpConn(t)
	addr := ln.Addr().String()
	r, err := Start(context.Background(), RuntimeOptions{
		MeasureListener: ln, MeasureUDP: uc, DiscoveryConn: pc, Ifaces: loIfaces,
		Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	r.Stop()
	r.Stop()
	if _, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		t.Fatal("puerto de medición abierto tras Stop")
	}
	if _, err := pc.WriteTo([]byte("x"), pc.LocalAddr()); err == nil {
		t.Fatal("socket de descubrimiento abierto tras Stop")
	}
	// El puerto de descubrimiento UDP se puede volver a abrir.
	pc2, err := net.ListenPacket("udp4", pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("puerto de descubrimiento sin liberar: %v", err)
	}
	pc2.Close()
}

func TestRuntimeExtraSubnetQueries(t *testing.T) {
	// Receptores en las direcciones expandidas de "127.0.0.2" y "127.0.0.4/31" (.4 y .5).
	var recv []net.PacketConn
	port := 0
	for _, ip := range []string{"127.0.0.2", "127.0.0.4", "127.0.0.5"} {
		addr := ip + ":0"
		if port != 0 {
			addr = net.JoinHostPort(ip, strconv.Itoa(port))
		}
		c, err := net.ListenPacket("udp4", addr)
		if err != nil {
			t.Skip(err)
		}
		defer c.Close()
		port = portOf(c.LocalAddr())
		recv = append(recv, c)
	}
	c := config.Defaults()
	c.ExtraSubnets = []string{"127.0.0.2", "127.0.0.4/31"}
	ln, uc := testutil.Listen(t)
	r, err := Start(context.Background(), RuntimeOptions{
		ConfigDir: cfgDir(t, c), MeasureListener: ln, MeasureUDP: uc, DiscoveryConn: udpConn(t),
		DiscoveryPort: port, Ifaces: loIfaces, Interval: 50 * time.Millisecond,
		Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, rc := range recv {
		for cycle := 0; cycle < 2; cycle++ { // en cada ciclo
			buf := make([]byte, 512)
			rc.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, _, err := rc.ReadFrom(buf)
			if err != nil {
				t.Fatalf("%v: ciclo %d sin consulta: %v", rc.LocalAddr(), cycle, err)
			}
			if string(buf[:n]) != string(discovery.QueryPacket()) {
				t.Fatalf("paquete inesperado: %q", buf[:n])
			}
		}
	}
}

func TestRuntimeInvalidSubnetWarns(t *testing.T) {
	c := config.Defaults()
	c.ExtraSubnets = []string{"no-es-ip"}
	ln, uc := testutil.Listen(t)
	var warns []string
	r, err := Start(context.Background(), RuntimeOptions{
		ConfigDir: cfgDir(t, c), MeasureListener: ln, MeasureUDP: uc, DiscoveryConn: udpConn(t),
		Ifaces: loIfaces, Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil },
		Warn: func(m string) { warns = append(warns, m) },
	})
	if err != nil {
		t.Fatal(err)
	}
	r.Stop()
	found := false
	for _, w := range warns {
		if strings.Contains(w, "subred ignorada") && strings.Contains(w, "no-es-ip") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sin aviso de subred inválida: %q", warns)
	}
}

func TestPickIP(t *testing.T) {
	ifs := []discovery.Iface{
		{IP: netip.MustParseAddr("10.0.0.1"), Prefix: netip.MustParsePrefix("10.0.0.1/24")},
		{IP: netip.MustParseAddr("192.168.1.5"), Prefix: netip.MustParsePrefix("192.168.1.5/24")},
	}
	if got := pickIP(ifs, netip.MustParseAddr("192.168.1.9")); got.String() != "192.168.1.5" {
		t.Fatalf("misma subred: %v", got)
	}
	if got := pickIP(ifs, netip.MustParseAddr("172.16.0.1")); got.String() != "10.0.0.1" {
		t.Fatalf("fallback: %v", got)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

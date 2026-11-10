package app

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/netinfo"
	"nperf/internal/testutil"
)

type diagNet struct{ infos []netinfo.Info }

func (f diagNet) Inventory() ([]netinfo.Info, error) { return f.infos, nil }

func loInfo(kind netinfo.Kind, bps uint64) diagNet {
	return diagNet{[]netinfo.Info{{Adapter: netinfo.Adapter{
		Name: "lo", IPv4: []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Kind: kind, Connected: true, TxBps: bps, RxBps: bps,
	}}}}
}

// diagRun levanta un Runtime servidor con srvNet y mide contra él desde una App con cliNet.
func diagRun(t *testing.T, srvNet, cliNet InventorySource) TestState {
	t.Helper()
	ln, uc := testutil.Listen(t)
	c := config.Defaults()
	c.Key = "k"
	r, err := Start(context.Background(), RuntimeOptions{
		ConfigDir: cfgDir(t, c), Alias: "Srv", MeasureListener: ln, MeasureUDP: uc, Net: srvNet,
		DiscoveryConn: udpConn(t), Ifaces: loIfaces, Send: func(netip.Addr, netip.AddrPort, []byte) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	tb := &discovery.Table{SelfID: selfID}
	dir := t.TempDir()
	cc := config.Defaults()
	cc.Key = "k"
	if err := config.Save(dir, cc); err != nil {
		t.Fatal(err)
	}
	a, err := New(Deps{ConfigDir: dir, Table: tb, SelfID: selfID, Net: cliNet,
		SelfIPs:  func() []string { return []string{"10.0.0.1"} },
		Hostname: func() (string, error) { return "Cli", nil }})
	if err != nil {
		t.Fatal(err)
	}
	ap := netip.MustParseAddrPort(ln.Addr().String())
	tb.Update(discovery.Announce{Alias: "Srv", IP: ap.Addr(), Port: int(ap.Port()), State: discovery.StateAvailable, InstanceID: idA}, netip.AddrPort{})
	if err := a.SelectPeer(idA); err != nil {
		t.Fatal(err)
	}
	a.duration = 1
	if err := a.StartTest(); err != nil {
		t.Fatal(err)
	}
	if !a.WaitTest(60 * time.Second) {
		t.Fatal("la prueba no terminó")
	}
	return a.State().Test
}

func TestDiagnosisBothEnds(t *testing.T) {
	// Enlaces de 1 kbps: el throughput de loopback los supera -> verde al 100 % por sentido.
	st := diagRun(t, loInfo(netinfo.KindEthernet, 1000), loInfo(netinfo.KindEthernet, 1000))
	if st.Status != TestDone || st.Diagnosis == nil {
		t.Fatalf("estado: %+v", st)
	}
	d := st.Diagnosis
	if d.Local == nil || d.Remote == nil {
		t.Fatalf("faltan enlaces: %+v", d)
	}
	for _, x := range []*DirDiagnosis{d.Fwd, d.Rev} {
		if x == nil || x.Color != "green" || x.Percent != 100 || x.Partial {
			t.Fatalf("sentido: %+v", x)
		}
	}
}

func TestDiagnosisNoProvider(t *testing.T) {
	st := diagRun(t, nil, nil)
	if st.Status != TestDone || st.Diagnosis == nil {
		t.Fatalf("estado: %+v", st)
	}
	for _, x := range []*DirDiagnosis{st.Diagnosis.Fwd, st.Diagnosis.Rev} {
		if x == nil || x.Color != "" {
			t.Fatalf("sin datos no debe haber semáforo: %+v", x)
		}
	}
}

func TestDiagnosisPartial(t *testing.T) {
	st := diagRun(t, nil, loInfo(netinfo.KindEthernet, 1000))
	d := st.Diagnosis
	if d == nil || d.Remote != nil || d.Local == nil {
		t.Fatalf("diagnóstico: %+v", d)
	}
	if d.Fwd == nil || !d.Fwd.Partial || d.Fwd.Color == "" {
		t.Fatalf("fwd debe ser parcial con color: %+v", d.Fwd)
	}
}

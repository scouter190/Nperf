package app

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"nperf/internal/discovery"
	"nperf/internal/netinfo"
)

type fakeNet struct {
	infos []netinfo.Info
	err   error
}

func (f fakeNet) Inventory() ([]netinfo.Info, error) { return f.infos, f.err }

func wifiInfos() []netinfo.Info {
	ch, sig := 36, 80
	return []netinfo.Info{{
		Adapter: netinfo.Adapter{Name: "Wi-Fi", Kind: netinfo.KindWiFi, Connected: true,
			IPv4: []netip.Addr{netip.MustParseAddr("10.0.0.1")}, TxBps: 866e6, RxBps: 866e6},
		WiFi: &netinfo.WiFiInfo{Available: true, SSID: "Casa", Band: "5 GHz", Channel: &ch, SignalPct: &sig,
			Standard: netinfo.NotAvailable, TxBps: 866e6, RxBps: 866e6},
	}}
}

func newNetApp(t *testing.T, n InventorySource, open func(string) error) (*App, *discovery.Table) {
	t.Helper()
	tb := &discovery.Table{SelfID: selfID, Now: time.Now}
	a, err := New(Deps{ConfigDir: t.TempDir(), Table: tb, SelfID: selfID, Net: n, OpenURL: open,
		SelfIPs:  func() []string { return []string{"10.0.0.1"} },
		Hostname: func() (string, error) { return "PC", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return a, tb
}

func TestStateLinkFields(t *testing.T) {
	a, tb := newNetApp(t, fakeNet{infos: wifiInfos()}, nil)
	tb.Update(discovery.Announce{Alias: "Otro", IP: netip.MustParseAddr("10.0.0.2"), Port: 5300,
		State: discovery.StateAvailable, InstanceID: "1111111111111111",
		Link: netinfo.KindEthernet, TxBps: 1e9, RxBps: 1e9, SSID: "<b>x</b>"}, netip.AddrPort{})
	tb.Update(discovery.Announce{Alias: "Viejo", IP: netip.MustParseAddr("10.0.0.3"), Port: 5300,
		State: discovery.StateAvailable, InstanceID: "2222222222222222"}, netip.AddrPort{})
	st := a.State()
	if s := st.Self; s.Link != "wifi" || s.SSID != "Casa" || s.LinkSpeed != "866 Mbps" {
		t.Fatalf("self: %+v", s)
	}
	for _, p := range st.Peers {
		switch p.Alias {
		case "Otro":
			if p.Link != "eth" || p.LinkSpeed != "1 Gbps" || p.SSID != "<b>x</b>" {
				t.Fatalf("otro: %+v", p)
			}
		case "Viejo":
			if p.Link != LinkUnknown || p.LinkSpeed != netinfo.NotAvailable || p.SSID != "" {
				t.Fatalf("viejo: %+v", p)
			}
		}
	}
}

func TestStateSelfLinkUnknownWithoutNet(t *testing.T) {
	a, _ := newNetApp(t, nil, nil)
	if s := a.State().Self; s.Link != LinkUnknown || s.SSID != "" {
		t.Fatalf("%+v", s)
	}
}

func TestAdaptersNoKeyAndNotice(t *testing.T) {
	infos := wifiInfos()
	infos[0].WiFi = &netinfo.WiFiInfo{Reason: "permiso", LocationDenied: true, Standard: netinfo.NotAvailable}
	a, _ := newNetApp(t, fakeNet{infos: infos}, nil)
	if err := a.SetKey("clave-super-secreta"); err != nil {
		t.Fatal(err)
	}
	v, err := a.Adapters(true)
	if err != nil || !v.LocationDenied || v.LocationNotice == "" {
		t.Fatalf("%+v %v", v, err)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "clave-super-secreta") {
		t.Fatal("el JSON contiene la clave")
	}
	st, _ := json.Marshal(a.State())
	if strings.Contains(string(st), "clave-super-secreta") {
		t.Fatal("State contiene la clave")
	}
}

func TestAdaptersErrors(t *testing.T) {
	a, _ := newNetApp(t, nil, nil)
	if _, err := a.Adapters(false); err == nil {
		t.Fatal("sin fuente debe fallar")
	}
	a, _ = newNetApp(t, fakeNet{err: errTest}, nil)
	if _, err := a.Adapters(false); err == nil {
		t.Fatal("el error de la fuente debe propagarse")
	}
}

var errTest = errString("fallo")

type errString string

func (e errString) Error() string { return string(e) }

func TestOpenLocationSettingsFixedURL(t *testing.T) {
	var got []string
	a, _ := newNetApp(t, nil, func(u string) error { got = append(got, u); return nil })
	if err := a.OpenLocationSettings(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "ms-settings:privacy-location" {
		t.Fatalf("%v", got)
	}
	a, _ = newNetApp(t, nil, nil)
	if a.OpenLocationSettings() == nil {
		t.Fatal("sin abridor debe fallar")
	}
}

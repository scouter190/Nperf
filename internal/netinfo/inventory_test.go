package netinfo

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

type fakeSrc struct {
	raw       []RawAdapter
	err       error
	wifi      RawWiFi
	wifiErr   error
	adapterN  int
	wifiCalls int
	guids     []string
}

func (f *fakeSrc) Adapters() ([]RawAdapter, error) { f.adapterN++; return f.raw, f.err }
func (f *fakeSrc) WiFi(guid string) (RawWiFi, error) {
	f.wifiCalls++
	f.guids = append(f.guids, guid)
	return f.wifi, f.wifiErr
}

func ip(s string) []netip.Addr { return []netip.Addr{netip.MustParseAddr(s)} }

func rawSet() []RawAdapter {
	return []RawAdapter{
		{Name: "lo", IfType: IfTypeLoopback, OperStatus: 1, IPv4: ip("127.0.0.1")},
		{Name: "eth0", Description: "Intel", IfType: IfTypeEthernet, OperStatus: 1, TxBps: 1e9, RxBps: 1e9, IPv4: ip("192.168.1.10")},
		{Name: "wl0", GUID: "{G-WL0}", Description: "AX200", IfType: IfTypeWiFi, OperStatus: 1, TxBps: 300e6, RxBps: 300e6, IPv4: ip("192.168.1.20")},
		{Name: "wl1", IfType: IfTypeWiFi, OperStatus: 2, IPv4: ip("10.0.0.5")},
	}
}

func find(t *testing.T, in []Info, name string) Info {
	t.Helper()
	for _, i := range in {
		if i.Name == name {
			return i
		}
	}
	t.Fatalf("falta %s", name)
	return Info{}
}

func TestInventoryWiFiOK(t *testing.T) {
	rssi := -55
	f := &fakeSrc{raw: rawSet(), wifi: RawWiFi{SSID: "Casa", BSSID: "aa:bb", PHYType: phyHE, QualityPct: 80, RSSI: &rssi, FreqKHz: 5180000, TxKbps: 866000, RxKbps: 700000}}
	in, err := Inventory(f)
	if err != nil || len(in) != 3 {
		t.Fatalf("err=%v n=%d", err, len(in))
	}
	w := find(t, in, "wl0").WiFi
	if !w.Available || w.SSID != "Casa" || w.Band != Band5 || *w.Channel != 36 || *w.SignalPct != 80 ||
		w.Standard != "802.11ax (WiFi 6)" || w.TxBps != 866e6 || w.RxBps != 700e6 {
		t.Fatalf("%+v", w)
	}
	if find(t, in, "eth0").WiFi != nil {
		t.Fatal("ethernet sin wifi")
	}
	if d := find(t, in, "wl1"); d.Connected || d.WiFi.Available || d.WiFi.Reason != ReasonNotConnected {
		t.Fatalf("%+v", d)
	}
	if f.wifiCalls != 1 || f.guids[0] != "{G-WL0}" {
		t.Fatalf("calls=%d", f.wifiCalls)
	}
}

func TestInventoryWLANErrors(t *testing.T) {
	cases := []struct {
		code   uint32
		reason string
		loc    bool
	}{
		{1062, ReasonServiceUnavailable, false},
		{5, ReasonLocationDenied, true},
		{5023, ReasonNotConnected, false},
	}
	for _, c := range cases {
		f := &fakeSrc{raw: rawSet(), wifi: RawWiFi{Err: c.code, SSID: "x"}}
		in, err := Inventory(f)
		if err != nil {
			t.Fatal(err)
		}
		a := find(t, in, "wl0")
		if a.WiFi.Available || a.WiFi.Reason != c.reason || a.WiFi.LocationDenied != c.loc || a.WiFi.SSID != "" {
			t.Fatalf("%d: %+v", c.code, a.WiFi)
		}
		if a.Kind != KindWiFi || a.TxBps != 300e6 || !a.Connected {
			t.Fatalf("se pierde tipo/velocidad: %+v", a)
		}
	}
	f := &fakeSrc{raw: rawSet(), wifiErr: errors.New("boom")}
	in, _ := Inventory(f)
	if w := find(t, in, "wl0").WiFi; w.Available || w.Reason == "" {
		t.Fatalf("%+v", w)
	}
}

func TestInventoryInvalidSSIDAndSourceError(t *testing.T) {
	f := &fakeSrc{raw: rawSet(), wifi: RawWiFi{SSID: "a\x00b"}}
	in, _ := Inventory(f)
	if find(t, in, "wl0").WiFi.SSID != "" {
		t.Fatal("ssid con control debe descartarse")
	}
	if _, err := Inventory(&fakeSrc{err: errors.New("x")}); err == nil {
		t.Fatal("error de adaptadores es fatal")
	}
	if _, err := Inventory(nil); err == nil {
		t.Fatal("fuente nil")
	}
}

func TestCached(t *testing.T) {
	f := &fakeSrc{raw: rawSet()}
	now := time.Unix(1000, 0)
	c := Cached(f, 2*time.Second, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if _, err := c.Inventory(); err != nil {
			t.Fatal(err)
		}
	}
	if f.adapterN != 1 {
		t.Fatalf("n=%d", f.adapterN)
	}
	now = now.Add(2 * time.Second)
	c.Inventory()
	if f.adapterN != 2 {
		t.Fatalf("n=%d", f.adapterN)
	}
	// Los errores no se cachean.
	f.err = errors.New("x")
	now = now.Add(5 * time.Second)
	if _, err := c.Inventory(); err == nil {
		t.Fatal("esperaba error")
	}
	f.err = nil
	if _, err := c.Inventory(); err != nil || f.adapterN != 4 {
		t.Fatalf("err=%v n=%d", err, f.adapterN)
	}
}

func TestForLocalIPAndSummary(t *testing.T) {
	f := &fakeSrc{raw: rawSet(), wifi: RawWiFi{SSID: "Casa", BSSID: "aa:bb:cc", PHYType: phyHE, QualityPct: 42, FreqKHz: 2437000, TxKbps: 144000, RxKbps: 144000}}
	in, _ := Inventory(f)
	got, ok := ForLocalIP(in, netip.MustParseAddr("192.168.1.20"))
	if !ok || got.Name != "wl0" {
		t.Fatalf("%v %v", got, ok)
	}
	if _, ok := ForLocalIP(in, netip.MustParseAddr("8.8.8.8")); ok {
		t.Fatal("sin coincidencia")
	}
	if SummaryForLocalIP(in, netip.MustParseAddr("8.8.8.8")) != nil {
		t.Fatal("desconocida = nil")
	}
	l := SummaryForLocalIP(in, netip.MustParseAddr("192.168.1.20"))
	if l.Kind != "wifi" || l.TxBps != 144e6 || l.SSID != "Casa" || l.Band != Band24 || l.Standard != "WiFi 6" || l.SignalPct != 42 {
		t.Fatalf("%+v", l)
	}
	if l.Sanitize().Standard != "WiFi 6" {
		t.Fatal("Standard no sobrevive a Sanitize")
	}
	e := SummaryForLocalIP(in, netip.MustParseAddr("192.168.1.10"))
	if e.Kind != "eth" || e.TxBps != 1e9 || e.SSID != "" {
		t.Fatalf("%+v", e)
	}
	if d := SummaryForLocalIP(in, netip.MustParseAddr("10.0.0.5")); d.TxBps != 0 || d.Kind != "wifi" {
		t.Fatalf("%+v", d)
	}
}

func TestSummaryStandards(t *testing.T) {
	cases := []struct {
		phy  int
		freq int
		want string
	}{{phyHE, 5955000, "WiFi 6E"}, {phyEHT, 5180000, "WiFi 7"}, {phyVHT, 5180000, "WiFi 5"}}
	for _, c := range cases {
		f := &fakeSrc{raw: rawSet(), wifi: RawWiFi{PHYType: c.phy, FreqKHz: c.freq, QualityPct: 50}}
		in, _ := Inventory(f)
		l := SummaryForLocalIP(in, netip.MustParseAddr("192.168.1.20"))
		if l.Standard != c.want || l.Sanitize().Standard != c.want {
			t.Fatalf("%s: %+v", c.want, l)
		}
	}
}

package netinfo

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

func sampleInfos() []Info {
	ch, sig, rssi := 36, 72, -55
	return []Info{
		{Adapter: Adapter{Name: "Ethernet", Description: "Realtek", MAC: "aa:bb", IPv4: []netip.Addr{netip.MustParseAddr("192.168.1.5")},
			Kind: KindEthernet, Connected: true, TxBps: 1e9, RxBps: 1e9}},
		{Adapter: Adapter{Name: "Wi-Fi", Description: "Intel", Kind: KindWiFi, Connected: true, TxBps: 866e6, RxBps: 866e6},
			WiFi: &WiFiInfo{Available: true, SSID: "Casa", BSSID: "11:22", Band: Band5, Channel: &ch, SignalPct: &sig, RSSI: &rssi,
				Standard: "802.11ac (WiFi 5)", TxBps: 780e6, RxBps: 780e6}},
	}
}

func TestCompactView(t *testing.T) {
	v := BuildView(sampleInfos(), false)
	b, _ := json.Marshal(v)
	want := `{"adapters":[{"name":"Ethernet","kind":"eth","kind_label":"Ethernet","connected":true,"speed":"1 Gbps"},` +
		`{"name":"Wi-Fi","kind":"wifi","kind_label":"WiFi","connected":true,"speed":"866 Mbps","wifi":{"available":true,"ssid":"Casa","band":"5 GHz","channel":"36","signal":"72 %"}}],"location_denied":false}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
}

func TestDetailedView(t *testing.T) {
	v := BuildView(sampleInfos(), true)
	e, w := v.Adapters[0], v.Adapters[1].WiFi
	if e.MAC != "aa:bb" || len(e.IPv4) != 1 || e.Description != "Realtek" || e.TxRate != "1 Gbps" {
		t.Errorf("eth: %+v", e)
	}
	if w.BSSID != "11:22" || w.RSSI != "-55 dBm" || w.Standard != "802.11ac (WiFi 5)" || w.TxRate != "780 Mbps" {
		t.Errorf("wifi: %+v", w)
	}
	txt := v.Text()
	for _, s := range []string{"Ethernet [Ethernet] 1 Gbps", "canal 36", "señal 72 %"} {
		if !strings.Contains(txt, s) {
			t.Errorf("texto sin %q:\n%s", s, txt)
		}
	}
}

func TestViewUnavailableAndLocation(t *testing.T) {
	infos := []Info{
		{Adapter: Adapter{Name: "Wi-Fi", Kind: KindWiFi, Connected: true, TxBps: 100e6, RxBps: 100e6},
			WiFi: &WiFiInfo{Reason: ReasonLocationDenied, LocationDenied: true, Standard: NotAvailable}},
		{Adapter: Adapter{Name: "Eth", Kind: KindEthernet}},
		{Adapter: Adapter{Name: "VPN", Kind: KindOther, Connected: true}},
	}
	v := BuildView(infos, false)
	if !v.LocationDenied || v.LocationNotice == "" {
		t.Error("falta aviso de ubicación")
	}
	if v.Adapters[0].Speed != "100 Mbps" {
		t.Errorf("la velocidad se pierde: %q", v.Adapters[0].Speed)
	}
	if v.Adapters[0].WiFi.Available || v.Adapters[0].WiFi.Reason != ReasonLocationDenied {
		t.Errorf("wifi: %+v", v.Adapters[0].WiFi)
	}
	if v.Adapters[1].Speed != "desconectada" || v.Adapters[2].Speed != NotAvailable {
		t.Errorf("speeds: %q %q", v.Adapters[1].Speed, v.Adapters[2].Speed)
	}
	if !strings.Contains(v.Text(), ReasonLocationDenied) {
		t.Error("texto sin motivo")
	}
}

func TestJSONGolden(t *testing.T) {
	infos := []Info{
		sampleInfos()[1],
		{Adapter: Adapter{Name: "Eth", Kind: KindEthernet}},
		{Adapter: Adapter{Name: "W2", Kind: KindWiFi, Connected: true},
			WiFi: &WiFiInfo{Reason: ReasonLocationDenied, LocationDenied: true, Standard: NotAvailable}},
	}
	b, err := JSON(infos)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"format_version":1,"adapters":[` +
		`{"name":"Wi-Fi","description":"Intel","mac":null,"ipv4":[],"type":"wifi","connected":true,"link_tx_bps":866000000,"link_rx_bps":866000000,"link_status":"ok",` +
		`"wifi":{"status":"ok","reason":null,"ssid":"Casa","bssid":"11:22","band":"5 GHz","channel":36,"signal_pct":72,"rssi_dbm":-55,"standard":"802.11ac (WiFi 5)","tx_bps":780000000,"rx_bps":780000000}},` +
		`{"name":"Eth","description":null,"mac":null,"ipv4":[],"type":"eth","connected":false,"link_tx_bps":null,"link_rx_bps":null,"link_status":"disconnected","wifi":null},` +
		`{"name":"W2","description":null,"mac":null,"ipv4":[],"type":"wifi","connected":true,"link_tx_bps":null,"link_rx_bps":null,"link_status":"unavailable",` +
		`"wifi":{"status":"unavailable","reason":"permiso de ubicación denegado","ssid":null,"bssid":null,"band":null,"channel":null,"signal_pct":null,"rssi_dbm":null,"standard":null,"tx_bps":null,"rx_bps":null}}],` +
		`"location_denied":true}`
	if string(b) != want {
		t.Fatalf("got\n%s", b)
	}
}

func TestViewHasNoKey(t *testing.T) {
	b, _ := json.Marshal(BuildView(sampleInfos(), true))
	j, _ := JSON(sampleInfos())
	for _, s := range []string{string(b), string(j)} {
		l := strings.ToLower(s)
		if strings.Contains(l, `"key`) || strings.Contains(l, "clave") || strings.Contains(l, "secret") {
			t.Errorf("la vista contiene la clave: %s", s)
		}
	}
}

package netinfo

import (
	"net/netip"
	"testing"
)

func bp(b bool) *bool { return &b }

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		r    RawAdapter
		kind Kind
		ok   bool
	}{
		{"wifi", RawAdapter{IfType: 71, Description: "Intel AX201"}, KindWiFi, true},
		{"eth", RawAdapter{IfType: 6, Description: "Realtek PCIe GbE"}, KindEthernet, true},
		{"eth physical", RawAdapter{IfType: 6, Physical: bp(true)}, KindEthernet, true},
		{"eth no physical", RawAdapter{IfType: 6, Physical: bp(false)}, KindOther, true},
		{"hyperv", RawAdapter{IfType: 6, Description: "Hyper-V Virtual Ethernet"}, KindOther, true},
		{"vpn", RawAdapter{IfType: 6, Description: "OpenVPN TAP-Windows"}, KindOther, true},
		{"wsl", RawAdapter{IfType: 6, Description: "vEthernet (WSL)"}, KindOther, true},
		{"vbox", RawAdapter{IfType: 6, Description: "VirtualBox Host-Only"}, KindOther, true},
		{"vmware", RawAdapter{IfType: 6, Description: "VMware Network Adapter"}, KindOther, true},
		{"loopback", RawAdapter{IfType: 24}, "", false},
		{"ppp", RawAdapter{IfType: 23}, KindOther, true},
	}
	for _, c := range cases {
		k, ok := Classify(c.r)
		if k != c.kind || ok != c.ok {
			t.Errorf("%s: got %q,%v", c.name, k, ok)
		}
	}
}

func TestClassifyAll(t *testing.T) {
	got := ClassifyAll([]RawAdapter{
		{Name: "lo", IfType: 24, OperStatus: 1},
		{Name: "Ethernet", IfType: 6, OperStatus: 1, TxBps: 1e9, RxBps: 1e9},
		{Name: "Wi-Fi", IfType: 71, OperStatus: 2},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if !got[0].Connected || got[0].Kind != KindEthernet || got[0].TxBps != 1e9 {
		t.Errorf("eth: %+v", got[0])
	}
	if got[1].Connected || got[1].Kind != KindWiFi {
		t.Errorf("wifi: %+v", got[1])
	}
}

func TestFormatBps(t *testing.T) {
	cases := map[uint64]string{
		0: "0 bps", 999: "999 bps", 56_000: "56 Kbps", 100_000_000: "100 Mbps",
		144_400_000: "144.4 Mbps", 1_000_000_000: "1 Gbps", 2_500_000_000: "2.5 Gbps",
		10_000_000_000: "10 Gbps",
	}
	for in, want := range cases {
		if got := FormatBps(in); got != want {
			t.Errorf("%d: got %q want %q", in, got, want)
		}
	}
}

func TestValidSSID(t *testing.T) {
	long := make([]byte, 33)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		s  string
		ok bool
	}{
		{"Casa", true}, {"Wi-Fi ñandú", true}, {"", false}, {string(long), false},
		{string(long[:32]), true}, {"a\x00b", false}, {"a\nb", false}, {"a\x7fb", false}, {"\xff", false},
	}
	for _, c := range cases {
		if ValidSSID(c.s) != c.ok {
			t.Errorf("%q: want %v", c.s, c.ok)
		}
	}
}

func TestVirtualTable(t *testing.T) {
	cases := []struct {
		name string
		r    RawAdapter
		kind Kind
		virt bool
	}{
		{"wifi direct", RawAdapter{IfType: 71, Name: "Conexión de área local* 1", Description: "Microsoft Wi-Fi Direct Virtual Adapter"}, KindOther, true},
		{"wifi direct 2", RawAdapter{IfType: 71, Name: "Conexión de área local* 10", Description: "Microsoft Wi-Fi Direct Virtual Adapter #2"}, KindOther, true},
		{"vethernet", RawAdapter{IfType: 6, Name: "vEthernet (Default Switch)", Description: "Hyper-V Virtual Ethernet Adapter"}, KindOther, true},
		{"vethernet nombre", RawAdapter{IfType: 6, Name: "vEthernet (Default Switch)", Description: "Adaptador"}, KindOther, true},
		{"tailscale", RawAdapter{IfType: 53, Name: "Tailscale", Description: "Tailscale Tunnel"}, KindOther, true},
		{"tailscale nombre", RawAdapter{IfType: 6, Name: "Tailscale", Description: "Wintun"}, KindOther, true},
		{"wireguard", RawAdapter{IfType: 6, Name: "wg0", Description: "WireGuard Tunnel"}, KindOther, true},
		{"tap", RawAdapter{IfType: 6, Description: "TAP-Windows Adapter V9"}, KindOther, true},
		{"vbox", RawAdapter{IfType: 6, Description: "VirtualBox Host-Only Ethernet Adapter"}, KindOther, true},
		{"vmware", RawAdapter{IfType: 6, Description: "VMware Virtual Ethernet Adapter for VMnet8"}, KindOther, true},
		{"loopback", RawAdapter{IfType: 6, Name: "Loopback Pseudo-Interface 1"}, KindOther, true},
		{"intel eth", RawAdapter{IfType: 6, Name: "Ethernet", Description: "Intel(R) Ethernet Connection (7) I219-V"}, KindEthernet, false},
		{"realtek", RawAdapter{IfType: 6, Name: "Ethernet 2", Description: "Realtek PCIe GbE Family Controller"}, KindEthernet, false},
		{"intel wifi", RawAdapter{IfType: 71, Name: "Wi-Fi", Description: "Intel(R) Wi-Fi 6 AX201 160MHz"}, KindWiFi, false},
		{"mediatek", RawAdapter{IfType: 71, Name: "Wi-Fi", Description: "MediaTek Wi-Fi 6E MT7922 160MHz Wireless LAN Card"}, KindWiFi, false},
		{"bluetooth pan", RawAdapter{IfType: 6, Name: "Conexión de red Bluetooth", Description: "Bluetooth Device (Personal Area Network)"}, KindEthernet, false},
		{"bluetooth pan otro", RawAdapter{IfType: 71 + 100, Name: "Bluetooth", Description: "Bluetooth Device (Personal Area Network)"}, KindOther, false},
	}
	for _, c := range cases {
		k, ok := Classify(c.r)
		if !ok || k != c.kind {
			t.Errorf("%s: kind %q ok=%v", c.name, k, ok)
		}
		if got := isVirtual(c.r); got != c.virt {
			t.Errorf("%s: virtual=%v", c.name, got)
		}
		if a := ClassifyAll([]RawAdapter{c.r}); a[0].Virtual != c.virt {
			t.Errorf("%s: Adapter.Virtual=%v", c.name, a[0].Virtual)
		}
	}
}

func TestSummaryIgnoresVirtual(t *testing.T) {
	ip := netip.MustParseAddr("100.64.0.5")
	in := []Info{{Adapter: Adapter{Virtual: true, Kind: KindOther, Connected: true, TxBps: 100e9, RxBps: 100e9, IPv4: []netip.Addr{ip}}}}
	if SummaryForLocalIP(in, ip) != nil {
		t.Error("IP en virtual debe dar enlace desconocido")
	}
}

package netinfo

import "testing"

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

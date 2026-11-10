package netinfo

import "testing"

func TestBandChannel(t *testing.T) {
	tests := []struct {
		khz  int
		band string
		ch   int
		ok   bool
	}{
		{2412000, Band24, 1, true},
		{2437000, Band24, 6, true},
		{2484000, Band24, 14, true},
		{2411000, "", 0, false},
		{5160000, Band5, 32, true},
		{5180000, Band5, 36, true},
		{5885000, Band5, 177, true},
		{5890000, "", 0, false},
		{5925000, Band6, 2, true},
		{5955000, Band6, 1, true},
		{7125000, Band6, 235, true},
		{7130000, "", 0, false},
		{0, "", 0, false},
		{-5, "", 0, false},
		{2412500, "", 0, false},
	}
	for _, tc := range tests {
		b, c, ok := BandChannel(tc.khz)
		if b != tc.band || c != tc.ch || ok != tc.ok {
			t.Errorf("BandChannel(%d) = %q,%d,%v; want %q,%d,%v", tc.khz, b, c, ok, tc.band, tc.ch, tc.ok)
		}
	}
}

func TestStandard(t *testing.T) {
	tests := []struct {
		phy  int
		band string
		want string
	}{
		{7, Band5, "802.11n (WiFi 4)"},
		{8, Band5, "802.11ac (WiFi 5)"},
		{10, Band24, "802.11ax (WiFi 6)"},
		{10, Band5, "802.11ax (WiFi 6)"},
		{10, Band6, "802.11ax (WiFi 6E)"},
		{11, Band6, "802.11be (WiFi 7)"},
		{0, Band5, NotAvailable},
		{99, "", NotAvailable},
	}
	for _, tc := range tests {
		if got := Standard(tc.phy, tc.band); got != tc.want {
			t.Errorf("Standard(%d,%q) = %q; want %q", tc.phy, tc.band, got, tc.want)
		}
	}
}

func TestWLANReason(t *testing.T) {
	tests := []struct {
		code   uint32
		reason string
		loc    bool
	}{
		{1062, ReasonServiceUnavailable, false},
		{2, ReasonServiceUnavailable, false},
		{5, ReasonLocationDenied, true},
		{5023, ReasonNotConnected, false},
		{1168, ReasonNotConnected, false},
		{9999, "no disponible (error 9999)", false},
	}
	for _, tc := range tests {
		r, l := WLANReason(tc.code)
		if r != tc.reason || l != tc.loc {
			t.Errorf("WLANReason(%d) = %q,%v; want %q,%v", tc.code, r, l, tc.reason, tc.loc)
		}
	}
}

package netinfo

import "testing"

func TestBandFromChannel(t *testing.T) {
	cases := []struct {
		name         string
		ch, phy, khz int
		band         string
		wantCh       int
		ok           bool
	}{
		{"canal 1", 1, phyHT, 0, Band24, 1, true},
		{"canal 14", 14, phyHT, 0, Band24, 14, true},
		{"canal 15", 15, phyHT, 0, "", 0, false},
		{"canal 31", 31, phyVHT, 0, "", 0, false},
		{"canal 32", 32, phyVHT, 0, Band5, 32, true},
		{"canal 177", 177, phyHE, 0, Band5, 177, true},
		{"canal 178 HE", 178, phyHE, 0, Band6, 178, true},
		{"canal 178 VHT", 178, phyVHT, 0, "", 0, false},
		{"canal 5 sin BSS", 5, phyHE, 0, Band24, 5, true},
		{"canal 5 con BSS 6 GHz", 5, phyHE, 5975000, Band6, 5, true},
		{"canal 5 con BSS 6 GHz, EHT", 5, phyEHT, 5975000, Band6, 5, true},
		{"canal 149 con BSS 5 GHz", 149, phyVHT, 5745000, Band5, 149, true},
		{"BSS manda sobre canal", 36, phyHE, 2437000, Band24, 6, true},
		{"freq 6 GHz irreconocible, VHT", 5, phyVHT, 6000001, Band24, 5, true},
		{"freq 6 GHz irreconocible, HE", 7, phyHE, 5977001, Band6, 7, true},
		{"sin datos", 0, 0, 0, "", 0, false},
		{"canal negativo", -1, phyHE, 0, "", 0, false},
	}
	for _, c := range cases {
		b, ch, ok := BandFromChannel(c.ch, c.phy, c.khz)
		if b != c.band || ch != c.wantCh || ok != c.ok {
			t.Errorf("%s: = %q,%d,%v; quiero %q,%d,%v", c.name, b, ch, ok, c.band, c.wantCh, c.ok)
		}
	}
}

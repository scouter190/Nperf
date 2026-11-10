//go:build windows

package netinfo

import "testing"

func TestSystemSourceAdapters(t *testing.T) {
	raw, err := NewSystemSource(nil).Adapters()
	if err != nil {
		t.Fatal(err)
	}
	ads := ClassifyAll(raw)
	if len(ads) < 1 {
		t.Fatalf("se esperaba al menos 1 adaptador, hay %d", len(ads))
	}
	for _, r := range raw {
		if r.IfType == IfTypeLoopback {
			continue
		}
		if r.GUID == "" || r.GUID[0] != '{' {
			t.Errorf("GUID inesperado %q", r.GUID)
		}
	}
	if _, err := Inventory(NewSystemSource(nil)); err != nil {
		t.Fatal(err)
	}
}

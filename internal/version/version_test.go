package version

import "testing"

func TestCompatible(t *testing.T) {
	cases := []struct {
		a, b string
		ok   bool
		err  bool
	}{
		{"1.0.0", "1.4.2", true, false},
		{"1.0.0", "0.9.0", false, false},
		{"1.2.0", "1.9.3", true, false},
		{"1.2.0", "2.0.0", false, false},
		{"x", "1.0.0", false, true},
		{"1.0.0", "", false, true},
	}
	for _, c := range cases {
		ok, err := Compatible(c.a, c.b)
		if ok != c.ok || (err != nil) != c.err {
			t.Errorf("Compatible(%q,%q) = %v, %v", c.a, c.b, ok, err)
		}
	}
}

func TestDisplayDefault(t *testing.T) {
	if Display() != "dev" {
		t.Errorf("Display() = %q", Display())
	}
}

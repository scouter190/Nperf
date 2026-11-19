package export

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

func wellFormed(t *testing.T, s string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		if _, err := d.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("XML mal formado: %v\n%s", err, s)
		}
	}
}

func phaseFromJSON(t *testing.T, j string) SVGPhase {
	t.Helper()
	var p SVGPhase
	if err := json.Unmarshal([]byte(j), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

const sample = `{"phase":"tcp_fwd","label":"TCP A → B","nominal_mbps":1000,"rtt_initial":{"min_ms":1,"avg_ms":2,"max_ms":3},
"samples":[[0,1,900,2.5,null,null],[1,1,940,null,null,null],[2,1,null,4,null,null],[3,1,950,3,null,null]],"available":true}`

func TestSVGGolden(t *testing.T) {
	p := phaseFromJSON(t, sample)
	for name, got := range map[string]string{"speed": SpeedSVG(p), "latency": LatencySVG(p)} {
		wellFormed(t, got)
		path := "testdata/" + name + ".svg"
		if os.Getenv("UPDATE_GOLDEN") != "" {
			_ = os.MkdirAll("testdata", 0o755)
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("%s difiere del golden", name)
		}
		for _, bad := range []string{"http:", "https:", "<script", "href", "<style", "url("} {
			if strings.Contains(strings.ReplaceAll(got, `xmlns="http://www.w3.org/2000/svg"`, ""), bad) {
				t.Errorf("%s contiene %q", name, bad)
			}
		}
		if !strings.Contains(got, `role="img"`) || !strings.Contains(got, "stroke-dasharray") {
			t.Errorf("%s sin role o nominal discontinua", name)
		}
	}
	if !strings.Contains(SpeedSVG(p), "nominal 1000 Mbps") || !strings.Contains(SpeedSVG(p), "máx 950") {
		t.Error("faltan valores en texto")
	}
}

func TestSVGHostileLabel(t *testing.T) {
	p := phaseFromJSON(t, sample)
	p.Label = `</text><script>alert("x")</script>" onload='x' & TCP`
	for _, s := range []string{SpeedSVG(p), LatencySVG(p)} {
		wellFormed(t, s)
		if strings.Contains(s, "<script") || strings.Contains(s, `" onload`) {
			t.Errorf("sin escapar: %s", s)
		}
	}
	p.Available = false
	s := SpeedSVG(p)
	wellFormed(t, s)
	if strings.Contains(s, "<script") {
		t.Error("sin escapar (no disponible)")
	}
}

func TestSVGEmptyAndUnavailable(t *testing.T) {
	cases := []SVGPhase{
		{Label: "x", Available: true},
		{Label: "x", Available: true, Samples: [][]*float64{}},
		phaseFromJSON(t, `{"label":"x","samples":[[0,1,5,1,null,null]],"available":false}`),
		phaseFromJSON(t, `{"label":"x","available":true,"samples":[[0,1,null,null,null,null],[1,1,null,null,null,null]]}`),
		phaseFromJSON(t, `{"label":"x","available":true,"samples":[[0],[]]}`),
	}
	for i, p := range cases {
		for _, s := range []string{SpeedSVG(p), LatencySVG(p)} {
			wellFormed(t, s)
			if !strings.Contains(s, "gráfica no disponible") {
				t.Errorf("caso %d sin texto de no disponible", i)
			}
		}
	}
}

func TestSVGNonFiniteAndNullRTT(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	one, two := 1.0, 2.0
	p := SVGPhase{Label: "x", Available: true, NominalMbps: &inf, Samples: [][]*float64{
		{&one, &one, &nan, &nan}, {&two, &one, &inf, &one}, {&two, &one, &two, &two},
	}}
	for _, s := range []string{SpeedSVG(p), LatencySVG(p)} {
		wellFormed(t, s)
		for _, bad := range []string{"NaN", "Inf"} {
			if strings.Contains(s, bad) {
				t.Errorf("contiene %s", bad)
			}
		}
	}
}

func TestSVGHugeAndTiny(t *testing.T) {
	big, tiny, one := 1e308, 5e-324, 1.0
	huge := 9e11
	for i, p := range []SVGPhase{
		{Label: "x", Available: true, NominalMbps: &big, Samples: [][]*float64{{&one, &one, &big, &big}}},
		{Label: "x", Available: true, Samples: [][]*float64{{&huge, &huge, &huge, &huge}, {&one, &one, &one, &one}}},
		{Label: "x", Available: true, Samples: [][]*float64{{&tiny, &tiny, &tiny, &tiny}, {&one, &one, &tiny, &tiny}}},
		{Label: "x", Available: true, NominalMbps: &tiny, RTTInitial: &SVGRTT{Avg: tiny}, Samples: [][]*float64{{&tiny, &tiny, &tiny, &tiny}}},
	} {
		for _, s := range []string{SpeedSVG(p), LatencySVG(p)} {
			wellFormed(t, s)
			for _, bad := range []string{"NaN", "Inf"} {
				if strings.Contains(s, bad) {
					t.Errorf("caso %d contiene %s", i, bad)
				}
			}
		}
	}
}

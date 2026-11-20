package ui

import (
	"math"
	"regexp"
	"strconv"
	"testing"
)

type rgb struct{ r, g, b float64 }

var (
	hexRe   = regexp.MustCompile(`^#([0-9a-fA-F]{6})$`)
	rgbaRe  = regexp.MustCompile(`^rgba\((\d+),\s*(\d+),\s*(\d+),\s*([0-9.]+)\)$`)
	varRe   = regexp.MustCompile(`(--[a-z-]+):([^;}]+)`)
	lightRe = regexp.MustCompile(`(?s)\n:root\{([^}]*--bg-canvas[^}]*)\}`)
	darkRe  = regexp.MustCompile(`(?s)@media \(prefers-color-scheme:dark\)\{:root\{([^}]*--bg-canvas[^}]*)\}`)
)

func parseColor(t *testing.T, v string, under rgb) rgb {
	t.Helper()
	if m := hexRe.FindStringSubmatch(v); m != nil {
		n, _ := strconv.ParseUint(m[1], 16, 32)
		return rgb{float64(n >> 16 & 255), float64(n >> 8 & 255), float64(n & 255)}
	}
	if m := rgbaRe.FindStringSubmatch(v); m != nil {
		r, _ := strconv.ParseFloat(m[1], 64)
		g, _ := strconv.ParseFloat(m[2], 64)
		b, _ := strconv.ParseFloat(m[3], 64)
		a, _ := strconv.ParseFloat(m[4], 64)
		return rgb{r*a + under.r*(1-a), g*a + under.g*(1-a), b*a + under.b*(1-a)}
	}
	t.Fatalf("color no reconocido: %q", v)
	return rgb{}
}

func lum(c rgb) float64 {
	f := func(x float64) float64 {
		x /= 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.r) + 0.7152*f(c.g) + 0.0722*f(c.b)
}

func ratio(a, b rgb) float64 {
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// Todo token de texto o de estado (fg) debe llegar a AA (4,5:1) sobre los
// fondos de la app y sobre su propio fondo tintado; el texto de los botones
// primarios, sobre el acento y su hover.
func TestTokenContrastAA(t *testing.T) {
	css := read(t, "app.css")
	l := lightRe.FindStringSubmatch(css)
	d := darkRe.FindStringSubmatch(css)
	if l == nil || d == nil {
		t.Fatal("faltan los bloques de tokens claro u oscuro en app.css")
	}
	for name, block := range map[string]string{"claro": l[1], "oscuro": d[1]} {
		tok := map[string]string{}
		for _, m := range varRe.FindAllStringSubmatch(block, -1) {
			tok[m[1]] = m[2]
		}
		surf := parseColor(t, tok["--bg-surface"], rgb{})
		bgs := []string{"--bg-canvas", "--bg-sidebar", "--bg-surface", "--bg-surface-hover", "--bg-input", "--bg-subtle"}
		check := func(fg, bg string, c1, c2 rgb) {
			if r := ratio(c1, c2); r < 4.5 {
				t.Errorf("%s: %s sobre %s = %.2f (< 4,5)", name, fg, bg, r)
			}
		}
		for _, fg := range []string{"--text-primary", "--text-secondary", "--text-muted", "--accent-primary", "--ok-fg", "--warn-fg", "--err-fg"} {
			if tok[fg] == "" {
				t.Fatalf("%s: falta %s", name, fg)
			}
			for _, bg := range bgs {
				check(fg, bg, parseColor(t, tok[fg], rgb{}), parseColor(t, tok[bg], rgb{}))
			}
		}
		for fg, bg := range map[string]string{"--accent-primary": "--accent-primary-soft", "--ok-fg": "--ok-bg", "--warn-fg": "--warn-bg", "--err-fg": "--err-bg"} {
			check(fg, bg+" sobre surface", parseColor(t, tok[fg], rgb{}), parseColor(t, tok[bg], surf))
		}
		fgOn := parseColor(t, tok["--text-on-accent"], rgb{})
		for _, bg := range []string{"--accent-primary", "--accent-primary-hover"} {
			check("--text-on-accent", bg, fgOn, parseColor(t, tok[bg], rgb{}))
		}
	}
}

func TestContrastDetector(t *testing.T) {
	if r := ratio(rgb{0, 0, 0}, rgb{255, 255, 255}); math.Abs(r-21) > 0.01 {
		t.Fatalf("ratio negro/blanco = %.2f", r)
	}
}

package ui

import (
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"nperf/internal/app"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(Files(), name)
	if err != nil {
		t.Fatalf("falta %s embebido: %v", name, err)
	}
	if len(b) == 0 {
		t.Fatalf("%s está vacío", name)
	}
	return string(b)
}

func TestEmbedded(t *testing.T) {
	for _, n := range []string{"index.html", "app.css", "app.js", "mock.js"} {
		read(t, n)
	}
}

func TestIndexSecurity(t *testing.T) {
	h := read(t, "index.html")
	if !strings.Contains(h, "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'") {
		t.Error("CSP estricta ausente")
	}
	if strings.Contains(h, "mock.js") {
		t.Error("index.html no debe cargar mock.js")
	}
	if regexp.MustCompile(`(?i)<script[^>]*>\s*[^<\s]|\son\w+\s*=|https?://`).MatchString(h) {
		t.Error("index.html no debe tener scripts en línea, manejadores on* ni URLs externas")
	}
}

var unsafeJS = regexp.MustCompile(`innerHTML|insertAdjacentHTML|outerHTML|document\.write|\beval\s*\(|\bFunction\s*\(|setAttribute|\.href\b|\.src\b|javascript:|\.on\w+\s*=`)

// unsafeUse devuelve los usos peligrosos del DOM encontrados en un JS.
func unsafeUse(js string) []string { return unsafeJS.FindAllString(js, -1) }

func TestNoUnsafeDOM(t *testing.T) {
	for _, n := range []string{"app.js", "mock.js"} {
		if bad := unsafeUse(read(t, n)); len(bad) > 0 {
			t.Errorf("%s usa API insegura: %v", n, bad)
		}
	}
}

func TestUnsafeDetector(t *testing.T) {
	for _, js := range []string{"x.innerHTML = a", "x.setAttribute('a',b)", "a.href = u", "i.src=u", "u='javascript:1'", "new Function('x')", "b.onclick = f", "eval (x)", "document.write(1)"} {
		if len(unsafeUse(js)) == 0 {
			t.Errorf("no detecta %q", js)
		}
	}
	if len(unsafeUse("el.textContent = a; e.addEventListener('click', f)")) != 0 {
		t.Error("falso positivo")
	}
}

// Cada método de la API que usa app.js corresponde a un método de App.
func TestBindingsExistInApp(t *testing.T) {
	js := read(t, "app.js")
	m := regexp.MustCompile(`NPERF_METHODS = \[([^\]]*)\]`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal("NPERF_METHODS no encontrado")
	}
	typ := reflect.TypeOf(&app.App{})
	names := regexp.MustCompile(`'(\w+)'`).FindAllStringSubmatch(m[1], -1)
	if len(names) != 10 {
		t.Fatalf("se esperaban 10 métodos, hay %d", len(names))
	}
	for _, n := range names {
		goName := strings.ToUpper(n[1][:1]) + n[1][1:]
		if _, ok := typ.MethodByName(goName); !ok {
			t.Errorf("app.App no tiene %s (JS %s)", goName, n[1])
		}
		if !strings.Contains(js, "nperf."+n[1]+"(") && n[1] != "state" {
			t.Errorf("app.js no usa nperf.%s", n[1])
		}
	}
}

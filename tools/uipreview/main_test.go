package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func setup(t *testing.T) http.Handler {
	t.Helper()
	d := t.TempDir()
	w := func(n, s string) {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w("index.html", `<head><link rel="stylesheet" href="app.css"></head><script src="chart.js"></script><script src="app.js"></script>`)
	w("app.css", "a{}")
	w("mock.js", "//mock")
	w("maqueta.html", `<html><script src="shell.js"></script></html>`)
	w("secreto.txt", "x")
	w("otro.js", "//x")
	w("shell.js", "//shell")
	return Handler(d, filepath.Join(d, "maqueta.html"), "example.com")
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCSPYMock(t *testing.T) {
	h := setup(t)
	for _, p := range []string{"/app", "/maqueta", "/app.css", "/shell.js"} {
		w := get(h, p)
		if w.Code != 200 || w.Header().Get("Content-Security-Policy") != csp {
			t.Fatalf("%s: código %d, CSP %q", p, w.Code, w.Header().Get("Content-Security-Policy"))
		}
	}
	app := get(h, "/app").Body.String()
	if !strings.Contains(app, `src="mock.js"`) || strings.Index(app, "mock.js") > strings.Index(app, "chart.js") {
		t.Fatalf("mock no inyectado antes del resto: %s", app)
	}
	if strings.Contains(get(h, "/maqueta").Body.String(), "mock.js") {
		t.Fatal("mock inyectado en la maqueta")
	}
}

func TestNoEncontrado(t *testing.T) {
	h := setup(t)
	for _, p := range []string{"/secreto.txt", "/../x", "/index.html", "/maqueta.html", "/otro.js", "/nada"} {
		if c := get(h, p).Code; c != 404 {
			t.Errorf("%s: código %d, quería 404", p, c)
		}
	}
}

func TestHostYMetodo(t *testing.T) {
	h := setup(t)
	r := httptest.NewRequest("GET", "/app", nil)
	r.Host = "evil.com"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("host ajeno: %d", w.Code)
	}
	r = httptest.NewRequest("POST", "/app", nil)
	r.Host = "example.com"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestCSPIgualQueGUI(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "internal", "gui", "static.go"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`const csp = "([^"]+)"`).FindSubmatch(b)
	if m == nil || string(m[1]) != csp {
		t.Fatalf("CSP divergente de internal/gui/static.go: %q", m)
	}
}

func TestListaDeIndex(t *testing.T) {
	m := permitidos([]byte(`<link rel="stylesheet" href="app.css"><script src="sidebar.js"></script><script src="https://x/y.js"></script>`))
	for _, n := range []string{"app.css", "sidebar.js", "mock.js", "shell.js"} {
		if m[n] == "" {
			t.Errorf("falta %s", n)
		}
	}
	if len(m) != 4 {
		t.Fatalf("lista inesperada: %v", m)
	}
}

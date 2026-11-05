package gui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"nperf/internal/ui"
)

func get(t *testing.T, h http.Handler, method, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+host+path, nil)
	r.Host = host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestStaticHandler(t *testing.T) {
	h := StaticHandler(ui.Files(), "tok", "127.0.0.1:1", "sec123")
	for p, ct := range map[string]string{"/tok/": "text/html", "/tok/index.html": "text/html", "/tok/app.js": "text/javascript", "/tok/app.css": "text/css"} {
		w := get(t, h, "GET", "127.0.0.1:1", p)
		if got := w.Header().Get("Content-Security-Policy"); got != csp || !strings.Contains(got, "frame-ancestors 'none'") {
			t.Errorf("%s: CSP: %q", p, got)
		}
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), ct) || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: %d %v", p, w.Code, w.Header())
		}
	}
	for _, p := range []string{"/", "/index.html", "/tok", "/tokx/", "/bad/app.js", "/tok/mock.js", "/tok/../app.js", "/tok//app.js", "/tok/sub/app.js"} {
		if w := get(t, h, "GET", "127.0.0.1:1", p); w.Code != 404 {
			t.Errorf("%s debía ser 404, es %d", p, w.Code)
		}
	}
	if w := get(t, h, "GET", "evil.example:1", "/tok/"); w.Code != 403 {
		t.Errorf("host ajeno: %d", w.Code)
	}
	if w := get(t, h, "POST", "127.0.0.1:1", "/tok/"); w.Code != 405 {
		t.Errorf("POST: %d", w.Code)
	}
	// un archivo permitido pero ausente del fs es 404
	h2 := StaticHandler(fstest.MapFS{}, "tok", "h", "s")
	if w := get(t, h2, "GET", "h", "/tok/app.js"); w.Code != 404 {
		t.Errorf("ausente: %d", w.Code)
	}
}

func TestServeUILocalOnly(t *testing.T) {
	url, closeFn, err := ServeUI(ui.Files(), "sec123")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("url: %s", url)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), "Content-Security-Policy") {
		t.Fatalf("UI: %d", resp.StatusCode)
	}
	// sin token no hay nada
	root := url[:strings.Index(url[len("http://"):], "/")+len("http://")]
	resp, err = http.Get(root + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("sin token: %d", resp.StatusCode)
	}
}

// La cabecera debe contener la política del meta de index.html tal cual.
func TestCSPHeaderMatchesMeta(t *testing.T) {
	b, _ := fs.ReadFile(ui.Files(), "index.html")
	m := regexp.MustCompile(`Content-Security-Policy" content="([^"]*)"`).FindSubmatch(b)
	if m == nil || !strings.HasPrefix(csp, string(m[1])) {
		t.Fatalf("la CSP de la cabecera no extiende la del meta: %q", m)
	}
}

func TestSessionMetaInjected(t *testing.T) {
	orig, _ := fs.ReadFile(ui.Files(), "index.html")
	if strings.Contains(string(orig), "nperf-session") {
		t.Fatal("el index.html embebido no debe llevar el secreto")
	}
	h := StaticHandler(ui.Files(), "tok", "h", "abc123")
	w := get(t, h, "GET", "h", "/tok/")
	if !strings.Contains(w.Body.String(), `<meta name="nperf-session" content="abc123">`) {
		t.Fatal("falta el meta nperf-session")
	}
	if strings.Contains(get(t, h, "GET", "h", "/tok/app.js").Body.String(), "abc123") {
		t.Fatal("el secreto solo va en index.html")
	}
}

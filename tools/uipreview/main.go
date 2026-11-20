// Command uipreview es un servidor de desarrollo para previsualizar la interfaz en un
// navegador. Solo desarrollo: no forma parte de los binarios de NPerf.
//
// Sirve internal/ui/web desde disco en 127.0.0.1 (puerto aleatorio) con la misma CSP que
// internal/gui. /maqueta es tools/uipreview/maqueta.html; /app es index.html con mock.js
// inyectado antes del resto de scripts.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// csp es la misma política que internal/gui (StaticHandler).
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// extras se sirven siempre (la maqueta carga shell.js y maqueta.js; mock.js solo se inyecta en /app).
var extras = []string{"shell.js", "mock.js", "maqueta.js"}

var refRe = regexp.MustCompile(`(?:src|href)="([A-Za-z0-9_.-]+\.(?:js|css))"`)

// permitidos deriva la lista blanca de los <script>/<link> de index.html más extras.
func permitidos(index []byte) map[string]string {
	m := map[string]string{}
	add := func(n string) {
		if strings.HasSuffix(n, ".css") {
			m[n] = "text/css; charset=utf-8"
		} else {
			m[n] = "text/javascript; charset=utf-8"
		}
	}
	for _, g := range refRe.FindAllSubmatch(index, -1) {
		add(string(g[1]))
	}
	for _, n := range extras {
		add(n)
	}
	return m
}

const html = "text/html; charset=utf-8"

// Handler sirve webDir y la maqueta leyendo del disco en cada petición.
func Handler(webDir, maqueta, host string) http.Handler {
	send := func(w http.ResponseWriter, ct string, b []byte) {
		h := w.Header()
		h.Set("Content-Type", ct)
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		w.Write(b)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "host no permitido", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
			return
		}
		switch p := r.URL.Path; p {
		case "/maqueta":
			b, err := os.ReadFile(maqueta)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			send(w, html, b)
		case "/app", "/":
			b, err := os.ReadFile(filepath.Join(webDir, "index.html"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			b = bytes.Replace(b, []byte(`<script src="chart.js">`), []byte(`<script src="mock.js"></script>`+"\n"+`<script src="chart.js">`), 1)
			send(w, html, b)
		default:
			idx, err := os.ReadFile(filepath.Join(webDir, "index.html"))
			if err != nil || len(p) < 2 {
				http.NotFound(w, r)
				return
			}
			name := p[1:]
			ct, ok := permitidos(idx)[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			dir := webDir
			if name == "maqueta.js" { // script propio de la maqueta, junto a maqueta.html
				dir = filepath.Dir(maqueta)
			}
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			send(w, ct, b)
		}
	})
}

func main() {
	web := flag.String("web", filepath.Join("internal", "ui", "web"), "directorio de la interfaz")
	mq := flag.String("maqueta", filepath.Join("tools", "uipreview", "maqueta.html"), "archivo de la maqueta")
	flag.Parse()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	host := ln.Addr().String()
	fmt.Printf("Maqueta: http://%s/maqueta\nApp con mock: http://%s/app\n", host, host)
	srv := &http.Server{Handler: Handler(*web, *mq, host), ReadHeaderTimeout: 5 * time.Second}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

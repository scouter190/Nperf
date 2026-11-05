package gui

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

// Por qué un servidor local: la CSP de index.html (script-src 'self', style-src 'self',
// connect-src 'none') exige un origen real. Con SetHtml/NavigateToString el origen es
// opaco y 'self' no coincide, por lo que app.js y app.css se bloquearían; la alternativa
// sería relajar la CSP ('unsafe-inline'), que está prohibido. El servidor solo sirve los
// archivos estáticos embebidos (públicos), escucha SOLO en 127.0.0.1 con puerto aleatorio,
// exige un token aleatorio en la ruta y valida Host (contra DNS rebinding). El puente con
// Go NO pasa por HTTP: usa window.chrome.webview.postMessage.

// servidos es la lista blanca de archivos (mock.js no se sirve nunca).
var servidos = map[string]string{
	"index.html": "text/html; charset=utf-8",
	"app.css":    "text/css; charset=utf-8",
	"app.js":     "text/javascript; charset=utf-8",
}

// csp es la misma política que el meta de index.html más las directivas que un meta no admite.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// StaticHandler sirve fsys bajo /<token>/ solo con GET/HEAD y Host == host.
// En index.html se inyecta en memoria el meta nperf-session con el secreto del puente.
func StaticHandler(fsys fs.FS, token, host, secret string) http.Handler {
	prefix := "/" + token + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "host no permitido", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
			return
		}
		got := r.URL.Path
		if len(got) < len(prefix) || subtle.ConstantTimeCompare([]byte(got[:len(prefix)]), []byte(prefix)) != 1 {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(got, prefix)
		if name == "" {
			name = "index.html"
		}
		ct, ok := servidos[path.Clean(name)]
		if !ok || path.Clean(name) != name {
			http.NotFound(w, r)
			return
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if name == "index.html" {
			b = bytes.Replace(b, []byte("<head>"), []byte(`<head>`+"\n"+`<meta name="nperf-session" content="`+secret+`">`), 1)
		}
		h := w.Header()
		h.Set("Content-Type", ct)
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		w.Write(b) // HEAD: net/http descarta el cuerpo
	})
}

// ServeUI abre el servidor en 127.0.0.1:<puerto aleatorio> y devuelve la URL de la UI
// (con token) y la función que lo cierra.
func ServeUI(fsys fs.FS, secret string) (url string, closeFn func(), err error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		ln.Close()
		return "", nil, errors.New("no se pudo generar el token de la interfaz")
	}
	token := hex.EncodeToString(tok[:])
	host := ln.Addr().String()
	srv := &http.Server{
		Handler:           StaticHandler(fsys, token, host, secret),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go srv.Serve(ln)
	return "http://" + host + "/" + token + "/", func() { srv.Close() }, nil
}

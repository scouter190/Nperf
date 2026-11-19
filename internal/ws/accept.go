package ws

import (
	"errors"
	"net/http"
	"strings"
)

// Accept hace el lado servidor del handshake. Es mínimo y solo para
// servidores de prueba: no negocia extensiones.
func Accept(w http.ResponseWriter, r *http.Request, o Options) (*Conn, error) {
	if r.Method != http.MethodGet || !headerHasToken(r.Header, "Connection", "upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" || r.Header.Get("Sec-WebSocket-Key") == "" {
		http.Error(w, "handshake WebSocket inválido", http.StatusBadRequest)
		return nil, errors.New("ws: handshake inválido")
	}
	sub := ""
	if o.Subprotocol != "" {
		for _, v := range r.Header.Values("Sec-WebSocket-Protocol") {
			for _, p := range strings.Split(v, ",") {
				if strings.TrimSpace(p) == o.Subprotocol {
					sub = o.Subprotocol
				}
			}
		}
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "sin Hijack", http.StatusInternalServerError)
		return nil, errors.New("ws: ResponseWriter sin Hijack")
	}
	nc, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		acceptKey(r.Header.Get("Sec-WebSocket-Key")) + "\r\n"
	if sub != "" {
		resp += "Sec-WebSocket-Protocol: " + sub + "\r\n"
	}
	resp += "\r\n"
	if _, err := nc.Write([]byte(resp)); err != nil {
		nc.Close()
		return nil, err
	}
	return &Conn{c: nc, br: rw.Reader, max: maxOf(o), sub: sub}, nil
}

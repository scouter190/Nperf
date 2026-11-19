// Package ws implementa el subconjunto de WebSocket (RFC 6455) que necesita
// NDT7: cliente sobre TLS con la librería estándar. Un solo lector y varios
// escritores concurrentes son seguros.
package ws

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Tipos de mensaje.
const (
	Text   = 1
	Binary = 2
)

const (
	opCont  = 0
	opClose = 8
	opPing  = 9
	opPong  = 10
)

// Códigos de cierre usados.
const (
	CloseNormal      = 1000
	CloseProtocol    = 1002
	CloseInvalidData = 1007
	CloseTooBig      = 1009
)

const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// DefaultMaxMessage es el tope de mensaje si Options.MaxMessage es 0.
const DefaultMaxMessage = 1 << 24

// Dialer abre la conexión TCP.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// Options configura Dial y Accept.
type Options struct {
	TLS         *tls.Config // nil: configuración por defecto (verificación normal)
	Dialer      Dialer      // nil: net.Dialer
	UserAgent   string
	Subprotocol string
	MaxMessage  int64
}

// CloseError es el cierre recibido del par.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("websocket cerrado: código %d %q", e.Code, e.Reason)
}

// ErrTooBig indica un mensaje mayor que MaxMessage.
var ErrTooBig = errors.New("ws: mensaje mayor que el tope")

// Conn es una conexión WebSocket.
type Conn struct {
	c         net.Conn
	br        *bufio.Reader
	client    bool
	max       int64
	sub       string
	wmu       sync.Mutex
	sentClose bool
	closeOnce sync.Once
}

// Subprotocol devuelve el subprotocolo negociado.
func (c *Conn) Subprotocol() string { return c.sub }

func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(h[:])
}

// Dial abre una conexión wss:// y hace el handshake. La URL se usa tal cual.
func Dial(ctx context.Context, rawurl string, o Options) (*Conn, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, fmt.Errorf("ws: URL inválida: %w", err)
	}
	if u.Scheme != "wss" || u.Hostname() == "" {
		return nil, errors.New("ws: solo se admiten URLs wss://")
	}
	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "443")
	}
	dial := o.Dialer
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	raw, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{}
	if o.TLS != nil {
		cfg = o.TLS.Clone()
	}
	cfg.NextProtos = []string{"http/1.1"}
	if cfg.ServerName == "" {
		cfg.ServerName = u.Hostname()
	}
	tc := tls.Client(raw, cfg)
	c := &Conn{c: tc, client: true, max: maxOf(o)}
	stop := c.arm(ctx, armBoth)
	defer stop()
	fail := func(err error) (*Conn, error) {
		tc.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := tc.HandshakeContext(ctx); err != nil {
		return fail(err)
	}
	if p := tc.ConnectionState().NegotiatedProtocol; p != "" && p != "http/1.1" {
		return fail(fmt.Errorf("ws: ALPN inesperado %q", p))
	}
	var kb [16]byte
	if _, err := rand.Read(kb[:]); err != nil {
		return fail(err)
	}
	key := base64.StdEncoding.EncodeToString(kb[:])
	req := &http.Request{Method: "GET", URL: u, Host: u.Host, Header: http.Header{}}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")
	if o.Subprotocol != "" {
		req.Header.Set("Sec-WebSocket-Protocol", o.Subprotocol)
	}
	if o.UserAgent != "" {
		req.Header.Set("User-Agent", o.UserAgent)
	}
	if err := req.Write(tc); err != nil {
		return fail(err)
	}
	c.br = bufio.NewReader(tc)
	resp, err := http.ReadResponse(c.br, req)
	if err != nil {
		return fail(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return fail(fmt.Errorf("ws: el servidor respondió %d en lugar de 101", resp.StatusCode))
	}
	h := resp.Header
	switch {
	case !strings.EqualFold(strings.TrimSpace(h.Get("Upgrade")), "websocket"):
		return fail(errors.New("ws: falta Upgrade: websocket"))
	case !headerHasToken(h, "Connection", "upgrade"):
		return fail(errors.New("ws: falta Connection: Upgrade"))
	case h.Get("Sec-WebSocket-Accept") != acceptKey(key):
		return fail(errors.New("ws: Sec-WebSocket-Accept incorrecto"))
	case h.Get("Sec-WebSocket-Extensions") != "":
		return fail(errors.New("ws: extensiones no solicitadas"))
	case h.Get("Sec-WebSocket-Protocol") != o.Subprotocol:
		return fail(fmt.Errorf("ws: subprotocolo %q distinto del solicitado %q", h.Get("Sec-WebSocket-Protocol"), o.Subprotocol))
	}
	c.sub = o.Subprotocol
	return c, nil
}

func maxOf(o Options) int64 {
	if o.MaxMessage > 0 {
		return o.MaxMessage
	}
	return DefaultMaxMessage
}

func headerHasToken(h http.Header, name, tok string) bool {
	for _, v := range h.Values(name) {
		for _, p := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(p), tok) {
				return true
			}
		}
	}
	return false
}

// Modos de arm.
const (
	armRead = iota
	armWrite
	armBoth
)

// arm aplica el plazo del contexto a la dirección indicada y la interrumpe si
// se cancela; lectura y escritura tienen plazos independientes.
func (c *Conn) arm(ctx context.Context, mode int) (stop func()) {
	set := func(t time.Time) {
		if mode != armWrite {
			_ = c.c.SetReadDeadline(t)
		}
		if mode != armRead {
			_ = c.c.SetWriteDeadline(t)
		}
	}
	if d, ok := ctx.Deadline(); ok {
		set(d)
	} else {
		set(time.Time{})
	}
	done := make(chan struct{})
	s := context.AfterFunc(ctx, func() {
		defer close(done)
		set(time.Unix(1, 0))
	})
	return func() {
		if !s() { // ya se disparó: esperar a que termine para no pisar el siguiente plazo
			<-done
		}
	}
}

// ReadMessage lee el siguiente mensaje completo (fragmentos reensamblados).
// Responde a ping y atiende el cierre: al recibir un cierre lo repite y
// devuelve *CloseError. Solo una goroutine debe leer.
func (c *Conn) ReadMessage(ctx context.Context) (int, []byte, error) {
	stop := c.arm(ctx, armRead)
	defer stop()
	typ, data, err := c.readMessage()
	return typ, data, ctxErr(ctx, err)
}

func (c *Conn) readMessage() (int, []byte, error) {
	var (
		buf  []byte
		typ  int
		open bool
	)
	for {
		fin, op, payload, err := c.readFrame(c.max - int64(len(buf)))
		if err != nil {
			return 0, nil, err
		}
		switch op {
		case opPing:
			if err := c.writeFrame(opPong, payload); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			ce := &CloseError{Code: 1005}
			if len(payload) == 1 {
				return 0, nil, c.protoErr("cierre con payload de 1 byte")
			}
			if len(payload) >= 2 {
				ce.Code = int(binary.BigEndian.Uint16(payload))
				ce.Reason = string(payload[2:])
				if !validCloseCode(ce.Code) {
					return 0, nil, c.protoErr("código de cierre inválido")
				}
				if !utf8.Valid(payload[2:]) {
					c.sendClose(CloseInvalidData, "motivo no UTF-8")
					return 0, nil, errors.New("ws: motivo de cierre no UTF-8")
				}
			}
			c.closeWith(echoPayload(payload))
			return 0, nil, ce
		case Text, Binary:
			if open {
				return 0, nil, c.protoErr("nuevo mensaje en medio de una fragmentación")
			}
			typ, open = op, true
		case opCont:
			if !open {
				return 0, nil, c.protoErr("continuación sin mensaje")
			}
		}
		buf = append(buf, payload...)
		if fin {
			if typ == Text && !utf8.Valid(buf) {
				c.sendClose(CloseInvalidData, "texto no UTF-8")
				return 0, nil, errors.New("ws: texto no UTF-8")
			}
			return typ, buf, nil
		}
	}
}

func validCloseCode(c int) bool {
	switch {
	case c >= 1000 && c <= 1003, c >= 1007 && c <= 1014, c >= 3000 && c <= 4999:
		return true
	}
	return false
}

func echoPayload(p []byte) []byte {
	if len(p) >= 2 {
		return p[:2]
	}
	return nil
}

func (c *Conn) protoErr(msg string) error {
	c.sendClose(CloseProtocol, msg)
	return errors.New("ws: error de protocolo: " + msg)
}

// readFrame lee una trama; limit es el espacio restante de payload.
func (c *Conn) readFrame(limit int64) (fin bool, op int, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(c.br, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	if h[0]&0x70 != 0 {
		err = c.protoErr("RSV distinto de cero")
		return
	}
	op = int(h[0] & 0x0f)
	masked := h[1]&0x80 != 0
	n := int64(h[1] & 0x7f)
	ctrl := op >= 8
	switch {
	case (op > Binary && op < opClose) || op > opPong:
		err = c.protoErr("opcode desconocido")
		return
	case c.client && masked:
		err = c.protoErr("trama del servidor enmascarada")
		return
	case !c.client && !masked:
		err = c.protoErr("trama del cliente sin máscara")
		return
	case ctrl && (!fin || n > 125):
		err = c.protoErr("trama de control inválida")
		return
	}
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return
		}
		n = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return
		}
		u := binary.BigEndian.Uint64(b[:])
		if u>>63 != 0 {
			err = c.protoErr("longitud inválida")
			return
		}
		n = int64(u)
	}
	if !ctrl && n > limit {
		c.sendClose(CloseTooBig, "mensaje demasiado grande")
		err = ErrTooBig
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i&3]
		}
	}
	return
}

// WriteMessage envía un mensaje completo (Text o Binary) en una trama.
func (c *Conn) WriteMessage(ctx context.Context, typ int, data []byte) error {
	if typ != Text && typ != Binary {
		return errors.New("ws: tipo de mensaje inválido")
	}
	if int64(len(data)) > c.max {
		return ErrTooBig
	}
	stop := c.arm(ctx, armWrite)
	defer stop()
	err := c.writeFrame(typ, data)
	return ctxErr(ctx, err)
}

// ctxErr traduce el error de E/S por el del contexto cuando este lo causó.
func ctxErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	var ne net.Error
	if _, ok := ctx.Deadline(); ok && errors.As(err, &ne) && ne.Timeout() {
		return context.DeadlineExceeded
	}
	return err
}

func (c *Conn) writeFrame(op int, payload []byte) error {
	return c.writeFrameFin(op, payload, true)
}

func (c *Conn) writeFrameFin(op int, payload []byte, fin bool) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeLocked(op, payload, fin)
}

func (c *Conn) writeLocked(op int, payload []byte, fin bool) error {
	hdr := make([]byte, 0, 14)
	b0 := byte(op)
	if fin {
		b0 |= 0x80
	}
	hdr = append(hdr, b0)
	var mb byte
	if c.client {
		mb = 0x80
	}
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, mb|byte(n))
	case n <= 0xffff:
		hdr = append(hdr, mb|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, mb|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	out := payload
	if c.client {
		var m [4]byte
		if _, err := rand.Read(m[:]); err != nil {
			return err
		}
		hdr = append(hdr, m[:]...)
		out = make([]byte, n)
		for i := range payload {
			out[i] = payload[i] ^ m[i&3]
		}
	}
	if _, err := c.c.Write(append(hdr, out...)); err != nil {
		return err
	}
	return nil
}

func (c *Conn) sendClose(code int, reason string) {
	p := binary.BigEndian.AppendUint16(nil, uint16(code))
	if len(reason) > 123 {
		reason = reason[:123]
	}
	c.closeWith(append(p, reason...))
}

// closeWith envía una trama de cierre una sola vez (best effort).
func (c *Conn) closeWith(payload []byte) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.sentClose {
		return
	}
	c.sentClose = true
	_ = c.c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = c.writeLocked(opClose, payload, true)
}

// Close envía el cierre con el código dado, espera brevemente el cierre del
// par y cierra la conexión. Es seguro llamarlo varias veces.
func (c *Conn) Close(code int, reason string) error {
	c.sendClose(code, reason)
	var err error
	c.closeOnce.Do(func() {
		_ = c.c.SetReadDeadline(time.Now().Add(2 * time.Second))
		for { // drena hasta el cierre del par
			_, op, _, e := c.readFrameLoose()
			if e != nil || op == opClose {
				break
			}
		}
		err = c.c.Close()
	})
	return err
}

// readFrameLoose lee tramas sin validar tope (descarta payloads grandes).
func (c *Conn) readFrameLoose() (bool, int, []byte, error) {
	return c.readFrame(c.max)
}

// Package proto define los mensajes del canal de control: JSON, uno por línea.
package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// Textos de error del handshake.
const (
	ErrBadKey       = "clave incorrecta"
	ErrIncompatible = "versión incompatible"
	ErrQueueFull    = "cola llena"
	ErrIPLimit      = "demasiadas pruebas simultáneas desde esta IP"
)

// Tipos de mensaje.
const (
	TypeHello  = "hello"
	TypeAuth   = "auth"
	TypeParams = "params"
	TypeStart  = "start"
	TypeResult = "result"
	TypeError  = "error"
	TypeData   = "data" // primera línea de una conexión de datos
	TypePing   = "ping" // RTT por el canal de control, antes de params
	TypePong   = "pong"
	TypeTurn   = "turn"   // cliente: pide turno tras autenticar
	TypeQueue  = "queue"  // servidor: en cola, con Position (1 = el siguiente)
	TypeReady  = "ready"  // servidor: turno concedido; ya se puede medir
	TypeStatus = "status" // cliente: consulta pública de estado, primer mensaje, antes del handshake
)

// Valores de params.
const (
	ProtoTCP  = "tcp"
	ProtoUDP  = "udp"
	ProtoBoth = "both" // solo en el cliente: RTT, TCP y UDP
	DirFwd    = "fwd"
	DirRev    = "rev"
	DirBoth   = "both" // solo en el cliente: fwd y luego rev en una misma sesión
)

// Flujos TCP paralelos.
const (
	DefaultStreams = 4
	MaxStreams     = 64
)

// Link resume el enlace de red de un extremo (sin BSSID ni MAC por construcción). Viaja solo
// por el canal ya autenticado: en "turn" (cliente) y "ready" (servidor).
type Link struct {
	Kind      string `json:"kind,omitempty"` // eth, wifi u other; vacío = desconocido
	TxBps     int64  `json:"tx_bps,omitempty"`
	RxBps     int64  `json:"rx_bps,omitempty"`
	SSID      string `json:"ssid,omitempty"`
	Band      string `json:"band,omitempty"`
	Standard  string `json:"standard,omitempty"`
	SignalPct int    `json:"signal_pct,omitempty"`
}

const (
	maxLinkBps   = int64(1e12) // 1 Tbps
	maxLinkLabel = 16
)

func cleanText(v string, max int) string {
	if len(v) > max {
		return ""
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == utf8.RuneError {
			return ""
		}
	}
	return v
}

// Sanitize devuelve una copia validada de un enlace recibido de la red (nil -> nil): tipo
// solo eth/wifi/other, SSID <= 32 B sin control, Band/Standard acotados, velocidades en
// [0, 1 Tbps] y señal 0-100; lo inválido pasa a "desconocido" (vacío o 0).
func (l *Link) Sanitize() *Link {
	if l == nil {
		return nil
	}
	o := *l
	switch o.Kind {
	case "eth", "wifi", "other":
	default:
		o.Kind = ""
	}
	o.SSID = cleanText(o.SSID, 32)
	o.Band = cleanText(o.Band, maxLinkLabel)
	o.Standard = cleanText(o.Standard, maxLinkLabel)
	if o.TxBps < 0 || o.TxBps > maxLinkBps {
		o.TxBps = 0
	}
	if o.RxBps < 0 || o.RxBps > maxLinkBps {
		o.RxBps = 0
	}
	if o.SignalPct < 0 || o.SignalPct > 100 {
		o.SignalPct = 0
	}
	return &o
}

// Message es el mensaje único del canal; cada tipo usa solo sus campos.
type Message struct {
	Type       string  `json:"type"`
	Version    string  `json:"version,omitempty"`
	Session    string  `json:"session,omitempty"`
	Nonce      string  `json:"nonce,omitempty"`
	MAC        string  `json:"mac,omitempty"`
	Proto      string  `json:"proto,omitempty"`
	Direction  string  `json:"direction,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Streams    int     `json:"streams,omitempty"`
	Position   int     `json:"position,omitempty"` // queue: puesto en la cola
	RateBps    float64 `json:"rate_bps,omitempty"` // UDP rev: tasa que debe enviar el servidor
	Packets    int64   `json:"packets,omitempty"`  // rev: datagramas enviados por el servidor
	Bytes      int64   `json:"bytes,omitempty"`
	Mbps       float64 `json:"mbps,omitempty"`
	Error      string  `json:"error,omitempty"`
	// Retrans: retransmisiones TCP del emisor en result (nil = no disponible).
	Retrans    *uint64 `json:"retrans,omitempty"`
	RetransEst bool    `json:"retrans_est,omitempty"` // el valor es una estimación
	// Resultado UDP medido por el receptor.
	Received int64   `json:"received,omitempty"`
	Expected int64   `json:"expected,omitempty"`
	Lost     int64   `json:"lost,omitempty"`
	LossPct  float64 `json:"loss_pct,omitempty"`
	JitterMS float64 `json:"jitter_ms,omitempty"`
	// Link: enlace del emisor del mensaje (solo en turn y ready; opcional).
	Link *Link `json:"link,omitempty"`
}

// Codec lee y escribe mensajes por línea sobre una conexión.
type Codec struct {
	r *bufio.Reader
	w io.Writer
}

func NewCodec(rw io.ReadWriter) *Codec {
	return &Codec{r: bufio.NewReader(rw), w: rw}
}

// Reader devuelve el lector con buffer (para leer datos tras el primer mensaje).
func (c *Codec) Reader() *bufio.Reader { return c.r }

func (c *Codec) Write(m Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = c.w.Write(append(b, '\n'))
	return err
}

func (c *Codec) Read() (Message, error) {
	var m Message
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return m, fmt.Errorf("mensaje inválido: %w", err)
	}
	return m, nil
}

// MaxPings es el máximo de ping que el servidor atiende por sesión.
const MaxPings = 64

// ClientTurn pide turno y espera "ready". onQueue (puede ser nil) se llama con cada posición
// en cola recibida; mientras espera no hay que aplicar plazos de lectura. Devuelve el "ready".
func ClientTurn(c *Codec, onQueue func(pos int)) error {
	_, err := ClientTurnLink(c, nil, onQueue)
	return err
}

// ClientTurnLink es ClientTurn enviando además el enlace local (nil = sin campo) y devolviendo
// el mensaje "ready" (su Link es el del servidor, nil si el par es de una versión previa).
func ClientTurnLink(c *Codec, link *Link, onQueue func(pos int)) (Message, error) {
	if err := c.Write(Message{Type: TypeTurn, Link: link}); err != nil {
		return Message{}, err
	}
	for {
		m, err := c.Read()
		if err != nil {
			return m, fmt.Errorf("canal de control cerrado: %w", err)
		}
		switch m.Type {
		case TypeReady:
			return m, nil
		case TypeQueue:
			if onQueue != nil {
				onQueue(m.Position)
			}
		case TypeError:
			return m, fmt.Errorf("el servidor respondió: %s", m.Error)
		default:
			return m, fmt.Errorf("mensaje inesperado %q (se esperaba %q)", m.Type, TypeReady)
		}
	}
}

// Estados de la consulta pública.
const (
	StateAvailable = "available"
	StateBusy      = "busy"
)

// StatusReply es la respuesta a "status": expone solo state y queue (nada de sesiones, IPs,
// nombres, versión ni detalles de la prueba en curso).
type StatusReply struct {
	State string `json:"state"`
	Queue int    `json:"queue"`
}

// WriteStatus envía la respuesta de estado (una línea JSON con exactamente state y queue).
func (c *Codec) WriteStatus(r StatusReply) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = c.w.Write(append(b, '\n'))
	return err
}

// ReadStatus lee la respuesta de estado y valida que el estado sea conocido.
func (c *Codec) ReadStatus() (StatusReply, error) {
	var r StatusReply
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(line, &r); err != nil {
		return r, fmt.Errorf("respuesta de estado inválida: %w", err)
	}
	if r.State != StateAvailable && r.State != StateBusy {
		return r, fmt.Errorf("respuesta de estado inválida: estado %q", r.State)
	}
	return r, nil
}

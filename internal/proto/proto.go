// Package proto define los mensajes del canal de control: JSON, uno por línea.
package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	TypeTurn   = "turn"  // cliente: pide turno tras autenticar
	TypeQueue  = "queue" // servidor: en cola, con Position (1 = el siguiente)
	TypeReady  = "ready" // servidor: turno concedido; ya se puede medir
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
// en cola recibida; mientras espera no hay que aplicar plazos de lectura.
func ClientTurn(c *Codec, onQueue func(pos int)) error {
	if err := c.Write(Message{Type: TypeTurn}); err != nil {
		return err
	}
	for {
		m, err := c.Read()
		if err != nil {
			return fmt.Errorf("canal de control cerrado: %w", err)
		}
		switch m.Type {
		case TypeReady:
			return nil
		case TypeQueue:
			if onQueue != nil {
				onQueue(m.Position)
			}
		case TypeError:
			return fmt.Errorf("el servidor respondió: %s", m.Error)
		default:
			return fmt.Errorf("mensaje inesperado %q (se esperaba %q)", m.Type, TypeReady)
		}
	}
}

// Package proto define los mensajes del canal de control: JSON, uno por línea.
package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Tipos de mensaje.
const (
	TypeHello  = "hello"
	TypeParams = "params"
	TypeStart  = "start"
	TypeResult = "result"
	TypeError  = "error"
	TypeData   = "data" // primera línea de una conexión de datos
)

// Valores de params.
const (
	ProtoTCP = "tcp"
	ProtoUDP = "udp"
	DirFwd   = "fwd"
)

// Message es el mensaje único del canal; cada tipo usa solo sus campos.
type Message struct {
	Type       string  `json:"type"`
	Version    string  `json:"version,omitempty"`
	Session    string  `json:"session,omitempty"`
	Proto      string  `json:"proto,omitempty"`
	Direction  string  `json:"direction,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Streams    int     `json:"streams,omitempty"`
	Bytes      int64   `json:"bytes,omitempty"`
	Mbps       float64 `json:"mbps,omitempty"`
	Error      string  `json:"error,omitempty"`
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

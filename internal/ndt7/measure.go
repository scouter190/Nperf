package ndt7

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"nperf/internal/hrtime"
	"nperf/internal/version"
	"nperf/internal/ws"
)

// Subprotocol es el subprotocolo WebSocket de ndt7.
const Subprotocol = "net.measurementlab.ndt.v7"

const (
	// MaxTotal es el tope de cada sentido (conexión incluida).
	MaxTotal = 15 * time.Second
	// UploadTime es lo que el cliente envía en la subida.
	UploadTime = 10 * time.Second

	maxMeasurement = 64 << 10 // un Measurement mayor se ignora
	minChunk       = 1 << 13
	maxChunk       = 1 << 20
)

// MeasureOptions configura Download y Upload.
type MeasureOptions struct {
	URL string     // URL wss devuelta por Locate, sin modificar
	WS  ws.Options // TLS, Dialer, UserAgent; el subprotocolo se fija aquí
	// Now es un reloj monotónico de alta resolución en ns (nil: hrtime.Now).
	Now        func() int64
	MaxTotal   time.Duration // 0: MaxTotal (15 s)
	UploadTime time.Duration // 0: UploadTime (10 s)
	// MinNormal es el tiempo mínimo para dar por completo un cierre normal
	// (0: 9 s); un cierre anterior se marca incompleto.
	MinNormal time.Duration
}

func (o MeasureOptions) clock() func() int64 {
	if o.Now != nil {
		return o.Now
	}
	return hrtime.Now
}

func (o MeasureOptions) total() time.Duration {
	if o.MaxTotal > 0 {
		return o.MaxTotal
	}
	return MaxTotal
}

func (o MeasureOptions) minNormal() time.Duration {
	if o.MinNormal > 0 {
		return o.MinNormal
	}
	return 9 * time.Second
}

func (o MeasureOptions) upTime() time.Duration {
	if o.UploadTime > 0 {
		return o.UploadTime
	}
	return UploadTime
}

// Direction es el resultado de un sentido.
type Direction struct {
	Mbps       float64  `json:"mbps"`
	Bytes      int64    `json:"bytes"`
	DurationS  float64  `json:"duration_s"`
	MinRTTms   *float64 `json:"min_rtt_ms"`
	Incomplete bool     `json:"incomplete"`
	Reason     string   `json:"reason,omitempty"`
	Notes      []string `json:"notes,omitempty"`
}

// measurement es el subconjunto usado del Measurement de ndt7.
type measurement struct {
	AppInfo *struct {
		ElapsedTime int64
		NumBytes    int64
	}
	TCPInfo *struct {
		ElapsedTime   int64
		BytesReceived int64
		MinRTT        int64
	}
}

// parseMeasurement devuelve false si el mensaje no es un Measurement usable.
func parseMeasurement(b []byte) (measurement, bool) {
	var m measurement
	if len(b) > maxMeasurement || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}

func mbps(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytes) * 8 / d.Seconds() / 1e6
}

func (o MeasureOptions) dial(ctx context.Context) (*ws.Conn, error) {
	w := o.WS
	if w.UserAgent == "" {
		w.UserAgent = "NPerf/" + version.Current
	}
	w.Subprotocol = Subprotocol
	if w.MaxMessage == 0 {
		w.MaxMessage = ws.DefaultMaxMessage
	}
	return ws.Dial(ctx, o.URL, w)
}

func rttFrom(m measurement, cur *float64) *float64 {
	if m.TCPInfo != nil && m.TCPInfo.MinRTT > 0 {
		v := float64(m.TCPInfo.MinRTT) / 1000
		return &v
	}
	return cur
}

// endReason clasifica el fin de la lectura: ok si fue un cierre normal.
func endReason(err error) (ok bool, reason string) {
	var ce *ws.CloseError
	switch {
	case errors.As(err, &ce):
		if ce.Code == ws.CloseNormal {
			return true, ""
		}
		return false, fmt.Sprintf("el servidor cerró con el código %d", ce.Code)
	case errors.Is(err, ws.ErrTooBig):
		return false, "mensaje del servidor demasiado grande"
	default:
		return false, "conexión interrumpida: " + err.Error()
	}
}

// Download mide la descarga: cuenta los bytes de los mensajes binarios.
// Solo falla (error) si no se pudo establecer la conexión; un corte posterior
// devuelve lo medido con Incomplete y su motivo.
func Download(ctx context.Context, o MeasureOptions) (Direction, error) {
	now := o.clock()
	cctx, cancel, capped := capCtx(ctx, o.total())
	defer cancel()
	c, err := o.dial(cctx)
	if err != nil {
		return Direction{}, err
	}
	var (
		d        Direction
		start    = now()
		lastData = start
	)
	var end int64
	for {
		typ, data, err := c.ReadMessage(cctx)
		if err != nil {
			end = now()
			switch {
			case capped.Load():
				// El servidor no cerró: se mide hasta el último dato.
				end = lastData
				d.Notes = append(d.Notes, fmt.Sprintf("el servidor no cerró; cortada a los %d s", int(o.total().Seconds())))
				if time.Duration(end-start) < o.minNormal() {
					d.Incomplete, d.Reason = true, "tiempo medido insuficiente"
				}
			case ctx.Err() != nil:
				d.Incomplete, d.Reason = true, "cancelada"
			default:
				if ok, reason := endReason(err); !ok {
					d.Incomplete, d.Reason = true, reason
					end = lastData
				} else if time.Duration(end-start) < o.minNormal() {
					d.Incomplete, d.Reason = true, "el servidor cerró antes de tiempo"
				}
			}
			break
		}
		switch typ {
		case ws.Binary:
			d.Bytes += int64(len(data))
			lastData = now()
		case ws.Text:
			if m, ok := parseMeasurement(data); ok {
				d.MinRTTms = rttFrom(m, d.MinRTTms)
			}
		}
	}
	closeConn(c)
	dur := time.Duration(end - start)
	d.DurationS = dur.Seconds()
	d.Mbps = mbps(d.Bytes, dur)
	return d, nil
}

// capCtx cancela a los d y marca capped; así el tope se distingue de una
// cancelación o plazo del llamador sin carreras entre relojes.
func capCtx(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc, *atomic.Bool) {
	cctx, cancel := context.WithCancel(ctx)
	var capped atomic.Bool
	t := time.AfterFunc(d, func() { capped.Store(true); cancel() })
	return cctx, func() { t.Stop(); cancel() }, &capped
}

func closeConn(c *ws.Conn) {
	_ = c.Close(ws.CloseNormal, "")
}

// Upload mide la subida con lo que informa el servidor: BytesReceived y
// ElapsedTime del último Measurement. Lectura y escritura son concurrentes y
// con contextos propios.
func Upload(ctx context.Context, o MeasureOptions) (Direction, error) {
	now := o.clock()
	rctx, rcancel, capped := capCtx(ctx, o.total())
	defer rcancel()
	c, err := o.dial(rctx)
	if err != nil {
		return Direction{}, err
	}
	start := now()
	wctx, wcancel := context.WithTimeout(rctx, o.upTime())
	defer wcancel()

	var (
		mu       sync.Mutex
		sent     int64
		lastM    *measurement
		rtt      *float64
		readErr  error
		writeErr error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // lector único
		defer wg.Done()
		defer wcancel()
		for {
			typ, data, err := c.ReadMessage(rctx)
			if err != nil {
				mu.Lock()
				readErr = err
				mu.Unlock()
				return
			}
			if typ != ws.Text {
				continue
			}
			if m, ok := parseMeasurement(data); ok {
				mu.Lock()
				rtt = rttFrom(m, rtt)
				if m.TCPInfo != nil || m.AppInfo != nil {
					mm := m
					lastM = &mm
				}
				mu.Unlock()
			}
		}
	}()
	go func() { // escritor
		defer wg.Done()
		buf := make([]byte, maxChunk)
		_, _ = rand.Read(buf)
		size := minChunk
		for wctx.Err() == nil {
			if err := c.WriteMessage(wctx, ws.Binary, buf[:size]); err != nil {
				// Un plazo/cancelación propio no es un fallo de escritura.
				if wctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					mu.Lock()
					writeErr = err
					mu.Unlock()
					wcancel()
					rcancel() // sin escritura no hay subida: no esperar al tope
				}
				return
			}
			mu.Lock()
			sent += int64(size)
			s := sent
			mu.Unlock()
			if size < maxChunk && int64(size) < s/16 {
				size *= 2
			}
		}
	}()
	wg.Wait()
	end := now()
	closeConn(c)

	var d Direction
	mu.Lock()
	defer mu.Unlock()
	d.MinRTTms = rtt
	// abrupt: el servidor cortó el TCP; es un fin normal si ya informó de casi
	// todo el tiempo de subida (puede cortar con datos en vuelo).
	abrupt := func(err error) {
		_, reason := endReason(err)
		d.Incomplete, d.Reason = true, reason
		var ce *ws.CloseError
		if !errors.As(err, &ce) && !errors.Is(err, ws.ErrTooBig) && lastM != nil &&
			serverElapsed(*lastM) >= max(o.upTime()*9/10, o.minNormal()) {
			d.Incomplete, d.Reason = false, ""
			d.Notes = append(d.Notes, "el servidor cortó la conexión al final")
		}
	}
	switch {
	case ctx.Err() != nil:
		d.Incomplete, d.Reason = true, "cancelada"
	case writeErr != nil:
		abrupt(writeErr)
	case capped.Load():
		d.Notes = append(d.Notes, fmt.Sprintf("el servidor no cerró; cortada a los %d s", int(o.total().Seconds())))
		if lastM == nil || serverElapsed(*lastM) < o.minNormal() {
			d.Incomplete, d.Reason = true, "tiempo medido insuficiente"
		}
	case readErr != nil:
		if ok, _ := endReason(readErr); !ok {
			abrupt(readErr)
		} else if lastM == nil || serverElapsed(*lastM) < o.minNormal() {
			d.Incomplete, d.Reason = true, "el servidor cerró antes de tiempo"
		}
	}
	var bytes int64
	var dur time.Duration
	if lastM != nil {
		if t := lastM.TCPInfo; t != nil && t.BytesReceived > 0 && t.ElapsedTime > 0 {
			bytes, dur = t.BytesReceived, time.Duration(t.ElapsedTime)*time.Microsecond
		} else if a := lastM.AppInfo; a != nil && a.NumBytes > 0 && a.ElapsedTime > 0 {
			bytes, dur = a.NumBytes, time.Duration(a.ElapsedTime)*time.Microsecond
		}
	}
	if dur <= 0 {
		bytes, dur = sent, time.Duration(end-start)
		d.Notes = append(d.Notes, "estimado por el cliente")
	}
	d.Bytes, d.DurationS, d.Mbps = bytes, dur.Seconds(), mbps(bytes, dur)
	return d, nil
}

// serverElapsed es el tiempo que el servidor informa en un Measurement.
func serverElapsed(m measurement) time.Duration {
	switch {
	case m.TCPInfo != nil && m.TCPInfo.ElapsedTime > 0:
		return time.Duration(m.TCPInfo.ElapsedTime) * time.Microsecond
	case m.AppInfo != nil:
		return time.Duration(m.AppInfo.ElapsedTime) * time.Microsecond
	}
	return 0
}

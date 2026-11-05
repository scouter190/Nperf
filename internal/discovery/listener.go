package discovery

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Anti-amplificación: la consulta debe medir al menos QuerySize (128) bytes y
// el anuncio puede llegar a MaxSize (512), un factor de hasta ~4x. Como UDP permite falsear el origen,
// se responde como máximo una vez por IP cada replyInterval y se recuerdan a
// lo sumo maxTracked IPs, de modo que un atacante en la LAN obtiene como
// mucho ~512 bytes por segundo y por IP falseada, y la tabla no crece sin límite.
const (
	replyInterval = time.Second
	maxTracked    = 1024
)

// Listener atiende el puerto de descubrimiento: responde a la consulta válida (rellena a 128 bytes)
// con el anuncio propio y entrega los anuncios recibidos a OnAnnounce.
type Listener struct {
	Conn net.PacketConn
	// Self devuelve el anuncio propio; ok=false si no hay nada que anunciar.
	Self func() (Announce, bool)
	// OnAnnounce recibe cada anuncio válido recibido (puede ser nil). Se llama
	// desde la goroutine de lectura: no debe bloquear.
	OnAnnounce func(Announce, netip.AddrPort)

	mu   sync.Mutex
	last map[netip.Addr]time.Time
}

// Serve lee hasta que ctx termine o el socket se cierre. Cierra Conn al salir
// y no deja goroutines. Devuelve nil si terminó por ctx.
func (l *Listener) Serve(ctx context.Context) error {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = l.Conn.Close()
		case <-stop:
		}
	}()
	defer func() { close(stop); <-done }()

	buf := make([]byte, MaxSize+1)
	for {
		n, from, err := l.Conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			// Errores transitorios (p. ej. ICMP en Windows): seguir.
			time.Sleep(10 * time.Millisecond)
			continue
		}
		ua, ok := from.(*net.UDPAddr)
		if !ok {
			continue
		}
		ap := ua.AddrPort()
		ap = netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
		if reply := l.handle(buf[:n], ap, time.Now()); reply != nil {
			_, _ = l.Conn.WriteTo(reply, from)
		}
	}
}

// handle procesa un datagrama y devuelve la respuesta (nil si no hay). Nunca
// entra en pánico con datos arbitrarios.
func (l *Listener) handle(data []byte, from netip.AddrPort, now time.Time) []byte {
	if isQuery(data) {
		if l.Self == nil || !l.allow(from.Addr(), now) {
			return nil
		}
		a, ok := l.Self()
		if !ok {
			return nil
		}
		msg, err := a.Marshal()
		if err != nil {
			return nil
		}
		return msg
	}
	if l.OnAnnounce != nil {
		if a, err := Parse(data); err == nil {
			l.OnAnnounce(a, from)
		}
	}
	return nil
}

// allow aplica el límite simple por IP.
func (l *Listener) allow(ip netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last == nil {
		l.last = make(map[netip.Addr]time.Time)
	}
	if t, ok := l.last[ip]; ok && now.Sub(t) < replyInterval {
		return false
	}
	if len(l.last) >= maxTracked {
		for k, t := range l.last {
			if now.Sub(t) >= replyInterval {
				delete(l.last, k)
			}
		}
		if len(l.last) >= maxTracked {
			return false
		}
	}
	l.last[ip] = now
	return true
}

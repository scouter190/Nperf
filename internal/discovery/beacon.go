package discovery

import (
	"context"
	"net"
	"net/netip"
	"time"

	"nperf/internal/netinfo"
	"nperf/internal/proto"
)

const (
	// DefaultDiscoveryPort es el puerto UDP de descubrimiento.
	DefaultDiscoveryPort = 5301
	// DefaultInterval es el intervalo entre anuncios.
	DefaultInterval = 3 * time.Second
)

// SendFunc envía data desde la IP local al destino. Inyectable en tests.
type SendFunc func(local netip.Addr, dst netip.AddrPort, data []byte) error

// Beacon emite periódicamente un anuncio por interfaz IPv4 activa, a su
// broadcast, con la IP propia de esa interfaz. Las interfaces se vuelven a
// enumerar en cada ciclo, así que las que aparecen o desaparecen se manejan
// sin reiniciar. Los fallos de una interfaz no afectan a las demás.
type Beacon struct {
	Alias         string
	InstanceID    string
	MeasurePort   int              // puerto de medición (default 5300 lo fija el llamador)
	DiscoveryPort int              // 0 = DefaultDiscoveryPort
	Interval      time.Duration    // 0 = DefaultInterval; ignorado si Ticks != nil
	Ticks         <-chan time.Time // opcional: reloj inyectado
	Ifaces        IfaceSource
	Status        func() (state string, queue int) // estado actual del server
	// LinkFor devuelve el enlace de la interfaz con esa IP (nil o campo nil = sin campos de enlace).
	LinkFor func(netip.Addr) *proto.Link
	Send    SendFunc                         // nil = UDP real
	Logf    func(format string, args ...any) // opcional; nil = sin log
	Now     func() time.Time                 // opcional: reloj para limitar el log

	lastLog map[string]time.Time
}

const logEvery = time.Minute

// logErr registra un error como máximo una vez por minuto por (clave, error).
func (b *Beacon) logErr(key string, err error, format string, args ...any) {
	if b.Logf == nil {
		return
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	k := key + "|" + err.Error()
	t := now()
	if last, ok := b.lastLog[k]; ok && t.Sub(last) < logEvery {
		return
	}
	if b.lastLog == nil {
		b.lastLog = map[string]time.Time{}
	}
	b.lastLog[k] = t
	b.Logf(format, args...)
}

// Run emite un anuncio inmediato y luego uno por tick, hasta que ctx termine.
func (b *Beacon) Run(ctx context.Context) {
	ticks := b.Ticks
	if ticks == nil {
		iv := b.Interval
		if iv <= 0 {
			iv = DefaultInterval
		}
		t := time.NewTicker(iv)
		defer t.Stop()
		ticks = t.C
	}
	b.cycle()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() != nil {
				return
			}
			b.cycle()
		}
	}
}

func (b *Beacon) cycle() {
	ifs, err := b.Ifaces()
	if err != nil {
		b.logErr("ifaces", err, "descubrimiento: no se pudieron enumerar las interfaces: %v", err)
		return
	}
	state, queue := b.Status()
	port := b.DiscoveryPort
	if port == 0 {
		port = DefaultDiscoveryPort
	}
	send := b.Send
	if send == nil {
		send = sendUDP
	}
	for _, ifc := range ifs {
		a := Announce{
			Alias: b.Alias, IP: ifc.IP, Port: b.MeasurePort,
			State: state, Queue: queue, InstanceID: b.InstanceID,
		}
		if b.LinkFor != nil {
			a = a.WithLink(b.LinkFor(ifc.IP))
		}
		data, err := a.Marshal()
		if err != nil {
			continue
		}
		if err := send(ifc.IP, netip.AddrPortFrom(ifc.Broadcast, uint16(port)), data); err != nil {
			b.logErr(ifc.IP.String(), err, "descubrimiento: falló el anuncio en %s (%s): %v", ifc.Name, ifc.IP, err)
		}
	}
}

// sendUDP envía un datagrama de broadcast desde la IP local (Go activa
// SO_BROADCAST en sockets UDP).
func sendUDP(local netip.Addr, dst netip.AddrPort, data []byte) error {
	c, err := net.DialUDP("udp4", &net.UDPAddr{IP: local.AsSlice()}, net.UDPAddrFromAddrPort(dst))
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = c.Write(data)
	return err
}

// WithLink copia al anuncio los campos públicos de l (tipo, velocidades, SSID); nil = sin cambios.
// Marshal omite lo inválido.
func (a Announce) WithLink(l *proto.Link) Announce {
	if l == nil {
		return a
	}
	a.Link = netinfo.Kind(l.Kind)
	a.TxBps = l.TxBps
	a.RxBps = l.RxBps
	a.SSID = l.SSID
	return a
}

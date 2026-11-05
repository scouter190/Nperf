package app

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"nperf/internal/config"
	"nperf/internal/discovery"
	"nperf/internal/netutil"
	"nperf/internal/server"
)

// DefaultMeasurePort es el puerto de medición (TCP y UDP).
const DefaultMeasurePort = 5300

// RuntimeOptions configura Start. La configuración (alias, clave, puertos, subredes) se lee
// solo en Start; para aplicar cambios hay que hacer Stop+Start.
// Todo lo opcional tiene valor por defecto;
// los campos inyectables existen para las pruebas.
type RuntimeOptions struct {
	ConfigDir   string // directorio de config del usuario ("" = valores por defecto, sin clave)
	Alias       string // alias propio ya resuelto (App.Alias)
	MeasurePort int    // 0 = DefaultMeasurePort
	// DiscoveryPort sustituye al discovery_port de la configuración si es != 0.
	DiscoveryPort int
	Interval      time.Duration // anuncios y consultas; 0 = discovery.DefaultInterval
	Ifaces        discovery.IfaceSource
	Send          discovery.SendFunc // nil = UDP real
	Warn          func(string)       // avisos (p. ej. sin clave); nil = ignorar

	// Inyección para tests: sockets ya abiertos (el Runtime los cierra).
	MeasureListener net.Listener
	MeasureUDP      *net.UDPConn
	DiscoveryConn   net.PacketConn
}

// Runtime mantiene activos, mientras la app está abierta, el server de
// medición, el emisor de anuncios, el receptor de descubrimiento y las
// consultas unicast. Su configuración se fija en Start: para aplicar cambios hay que Stop+Start.
type Runtime struct {
	Table      *discovery.Table
	SelfID     string
	MeasureAdr net.Addr // dirección de escucha de la medición

	srv    *server.Server
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// SelfState es el estado propio (disponible/ocupado y cola) para App.
func (r *Runtime) SelfState() (state string, queue int) { return r.srv.Status() }

// Start abre los puertos y lanza todo. Si algo falla, cierra lo ya abierto.
func Start(parent context.Context, o RuntimeOptions) (*Runtime, error) {
	cfg := config.Defaults()
	if o.ConfigDir != "" {
		c, err := config.LoadFull(o.ConfigDir)
		if err != nil {
			return nil, err
		}
		cfg = c
	}
	warn := o.Warn
	if warn == nil {
		warn = func(string) {}
	}
	if cfg.Key == "" {
		warn("aviso: sin clave, cualquiera puede usar este servidor")
	}
	mport := o.MeasurePort
	if mport == 0 {
		mport = DefaultMeasurePort
	}
	dport := cfg.DiscoveryPort
	if o.DiscoveryPort != 0 {
		dport = o.DiscoveryPort
	}
	id, err := discovery.NewInstanceID()
	if err != nil {
		return nil, fmt.Errorf("no se pudo generar el id de instancia: %w", err)
	}
	ifaces := o.Ifaces
	if ifaces == nil {
		ifaces = discovery.SystemIfaces(false)
	}

	ln := o.MeasureListener
	uc := o.MeasureUDP
	pc := o.DiscoveryConn
	closeAll := func() {
		if ln != nil {
			ln.Close()
		}
		if uc != nil {
			uc.Close()
		}
		if pc != nil {
			pc.Close()
		}
	}
	if ln == nil {
		ln, err = netutil.Listen(context.Background(), ":"+strconv.Itoa(mport), 15*time.Second)
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("el puerto %d ya está en uso: ¿hay otra instancia de NPerf o iperf? (%v)", mport, err)
		}
	}
	if uc == nil {
		ta := ln.Addr().(*net.TCPAddr)
		uc, err = netutil.ListenUDP("udp", &net.UDPAddr{IP: ta.IP, Port: ta.Port, Zone: ta.Zone})
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("el puerto UDP %d ya está en uso: ¿hay otra instancia de NPerf o iperf? (%v)", ta.Port, err)
		}
	}
	if pc == nil {
		pc, err = netutil.ListenPacket("udp4", ":"+strconv.Itoa(dport))
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("el puerto de descubrimiento %d ya está en uso: ¿hay otra instancia de NPerf? (%v)", dport, err)
		}
	}
	mport = ln.Addr().(*net.TCPAddr).Port

	srv := server.New(server.WithKey(cfg.Key), server.WithUDPConn(uc))
	table := &discovery.Table{SelfID: id}
	var addrs []netip.Addr
	for _, s := range cfg.ExtraSubnets {
		a, err := discovery.ParseSubnet(s)
		if err != nil { // una subred inválida no impide arrancar: se ignora y se avisa
			warn("aviso: subred ignorada: " + err.Error())
			continue
		}
		addrs = append(addrs, a...)
	}
	iv := o.Interval
	if iv <= 0 {
		iv = discovery.DefaultInterval
	}

	ctx, cancel := context.WithCancel(parent)
	r := &Runtime{Table: table, SelfID: id, MeasureAdr: ln.Addr(), srv: srv, cancel: cancel}

	beacon := &discovery.Beacon{
		Alias: o.Alias, InstanceID: id, MeasurePort: mport, DiscoveryPort: dport,
		Interval: iv, Ifaces: ifaces, Status: srv.Status, Send: o.Send,
		Logf: func(f string, a ...any) { warn(fmt.Sprintf(f, a...)) },
	}
	self := func(from netip.Addr) (discovery.Announce, bool) {
		ifs, err := ifaces()
		if err != nil || len(ifs) == 0 {
			return discovery.Announce{}, false
		}
		ip := pickIP(ifs, from)
		st, q := srv.Status()
		return discovery.Announce{Alias: o.Alias, IP: ip, Port: mport, State: st, Queue: q, InstanceID: id}, true
	}
	lis := &discovery.Listener{Conn: pc, Self: self, OnAnnounce: table.Update}

	r.wg.Add(3)
	go func() { defer r.wg.Done(); _ = srv.Serve(ctx, ln) }()
	go func() { defer r.wg.Done(); beacon.Run(ctx) }()
	go func() { defer r.wg.Done(); _ = lis.Serve(ctx) }()
	if len(addrs) > 0 {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			t := time.NewTicker(iv)
			defer t.Stop()
			for {
				qctx, qc := context.WithTimeout(ctx, iv)
				_ = discovery.SendQueries(qctx, pc, addrs, dport)
				qc()
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}
	return r, nil
}

// Stop detiene todo y espera a que terminen las goroutines. Es idempotente.
func (r *Runtime) Stop() {
	r.once.Do(r.cancel)
	r.wg.Wait()
}

// pickIP elige la IP de la interfaz que comparte subred con from; si ninguna, la primera.
func pickIP(ifs []discovery.Iface, from netip.Addr) netip.Addr {
	for _, f := range ifs {
		if f.Prefix.IsValid() && f.Prefix.Contains(from) {
			return f.IP
		}
	}
	return ifs[0].IP
}

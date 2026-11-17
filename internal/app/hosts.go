package app

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"nperf/internal/config"
	"nperf/internal/probe"
)

// Identificadores y textos de los equipos sin NPerf.
const (
	GatewayID       = "gateway"
	GatewayAlias    = "Puerta de enlace (sin NPerf)"
	hostIDPrefix    = "host:"
	StatusNoAgent   = "noagent"
	NoAgentLabel    = "sin NPerf"
	NoAgentBlocked  = "los equipos sin NPerf no admiten TEST!; use Ping, Traceroute, MTU o Puertos"
	gatewayCacheTTL = 5 * time.Second
)

// gwCache guarda la última puerta de enlace consultada.
type gwCache struct {
	at   time.Time
	ip   netip.Addr
	have bool // hay consulta guardada
}

func hostID(target string) string { return hostIDPrefix + strings.ToLower(target) }

// cleanAlias quita caracteres de control y espacios sobrantes y recorta a 64 bytes.
func cleanAlias(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	for len(s) > config.MaxHostAliasLen {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return strings.TrimSpace(s)
}

// gateway devuelve la puerta de enlace con caché de 5 s (sin Router o sin puerta: inválida).
func (a *App) gateway() netip.Addr {
	if a.d.Router == nil {
		return netip.Addr{}
	}
	a.gwMu.Lock()
	defer a.gwMu.Unlock()
	now := a.d.Now()
	if a.gw.have && now.Sub(a.gw.at) < gatewayCacheTTL {
		return a.gw.ip
	}
	ip, err := a.d.Router.Gateway()
	if err != nil {
		ip = netip.Addr{}
	}
	a.gw = gwCache{at: now, ip: ip, have: true}
	return ip
}

// AddHost añade un destino sin NPerf (IP o nombre) sin resolverlo ni enviar tráfico.
func (a *App) AddHost(target, alias string) error {
	t, err := probe.ParseTarget(target)
	if err != nil {
		return err
	}
	if t.IP.IsValid() {
		var local []netip.Prefix
		if a.d.Router != nil {
			local, _ = a.d.Router.LocalPrefixes()
		}
		if err := probe.CheckAddr(t.IP, local); err != nil {
			return err
		}
		if t.IP == a.gateway() {
			return errors.New("esa dirección ya está en la lista como puerta de enlace")
		}
	}
	alias = cleanAlias(alias)
	id := hostID(t.Raw)
	var verr error
	uerr := a.update(func(c *config.Config) {
		if len(c.ManualHosts) >= config.MaxManualHosts {
			verr = fmt.Errorf("se alcanzó el máximo de %d equipos añadidos a mano", config.MaxManualHosts)
			return
		}
		for _, h := range c.ManualHosts {
			if hostID(h.Target) == id {
				verr = fmt.Errorf("%s ya está en la lista", t.Raw)
				return
			}
		}
		c.ManualHosts = append(c.ManualHosts, config.ManualHost{Target: t.Raw, Alias: alias})
	})
	if verr != nil {
		return verr
	}
	return uerr
}

// RemoveHost quita un destino añadido a mano; la puerta de enlace no se puede quitar.
func (a *App) RemoveHost(id string) error {
	if id == GatewayID {
		return errors.New("la puerta de enlace no se puede quitar de la lista")
	}
	if !strings.HasPrefix(id, hostIDPrefix) {
		return errors.New("solo se pueden quitar equipos sin NPerf añadidos a mano")
	}
	found := false
	var verr error
	uerr := a.update(func(c *config.Config) {
		out := make([]config.ManualHost, 0, len(c.ManualHosts))
		for _, h := range c.ManualHosts {
			if hostID(h.Target) == id {
				found = true
				continue
			}
			out = append(out, h)
		}
		if !found {
			verr = errors.New("el equipo no está en la lista")
			return
		}
		c.ManualHosts = out
	})
	if verr != nil {
		return verr
	}
	if uerr != nil {
		return uerr
	}
	a.mu.Lock()
	if a.selected == id {
		a.selected = ""
	}
	a.mu.Unlock()
	return nil
}

// noAgentDevices arma el grupo "sin NPerf": puerta de enlace primero y luego los manuales.
func (a *App) noAgentDevices(cfg config.Config) []Device {
	out := []Device{}
	if gw := a.gateway(); gw.IsValid() {
		d := device(GatewayID, GatewayAlias, []string{gw.String()}, StatusNoAgent, 0, 0)
		d.Label = NoAgentLabel
		out = append(out, d.withLink("", 0, 0, ""))
	}
	for _, h := range cfg.ManualHosts {
		alias := h.Alias
		if alias == "" {
			alias = h.Target
		}
		d := device(hostID(h.Target), alias, []string{h.Target}, StatusNoAgent, 0, 0)
		d.Label = NoAgentLabel
		out = append(out, d.withLink("", 0, 0, ""))
	}
	return out
}

// isNoAgent indica si id es la puerta de enlace actual o un manual guardado.
func (a *App) isNoAgent(id string) bool {
	cfg, _ := a.cfg()
	for _, d := range a.noAgentDevices(cfg) {
		if d.ID == id {
			return true
		}
	}
	return false
}

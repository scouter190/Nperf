package discovery

import (
	"net/netip"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultUnreachableAfter: sin anuncio durante este plazo, el equipo pasa a inalcanzable.
	DefaultUnreachableAfter = 10 * time.Second
	// DefaultExpireAfter: sin anuncio durante este plazo, la entrada se elimina.
	DefaultExpireAfter = 5 * time.Minute
	// DefaultMaxPeers: tope de entradas de la tabla.
	DefaultMaxPeers = 256
)

// Peer es un equipo descubierto (copia inmutable para la UI).
type Peer struct {
	Announce
	Reachable bool      // false = "inalcanzable"
	LastSeen  time.Time // último anuncio recibido
}

// Table guarda los equipos descubiertos, deduplicados por id de instancia.
// Es segura para uso concurrente. Update tiene la firma de Listener.OnAnnounce
// y no bloquea.
//
// Al llegar al tope (MaxPeers) se purgan primero las entradas caducadas; si
// sigue llena, se desaloja la entrada INALCANZABLE más antigua para el id
// nuevo; si todas son alcanzables, se DESCARTA el anuncio nuevo (los ya
// conocidos siguen actualizándose). Así ids falsos que dejan de anunciarse
// no bloquean a equipos reales más de ~UnreachableAfter.
type Table struct {
	SelfID           string           // instancia propia, se excluye ("" = no excluir)
	Now              func() time.Time // reloj inyectable; nil = time.Now
	UnreachableAfter time.Duration    // 0 = DefaultUnreachableAfter
	ExpireAfter      time.Duration    // 0 = DefaultExpireAfter
	MaxPeers         int              // 0 = DefaultMaxPeers

	mu    sync.Mutex
	peers map[string]*entry
}

type entry struct {
	a    Announce
	seen time.Time
}

func (t *Table) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (t *Table) limits() (unreach, expire time.Duration, max int) {
	unreach, expire, max = t.UnreachableAfter, t.ExpireAfter, t.MaxPeers
	if unreach <= 0 {
		unreach = DefaultUnreachableAfter
	}
	if expire <= 0 {
		expire = DefaultExpireAfter
	}
	if max <= 0 {
		max = DefaultMaxPeers
	}
	return
}

// Update registra un anuncio recibido (del campo IP del anuncio, que es donde se mide).
func (t *Table) Update(a Announce, _ netip.AddrPort) {
	if a.InstanceID == "" || a.InstanceID == t.SelfID {
		return
	}
	unreach, expire, max := t.limits()
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.peers == nil {
		t.peers = make(map[string]*entry)
	}
	if e, ok := t.peers[a.InstanceID]; ok {
		e.a, e.seen = a, now
		return
	}
	if len(t.peers) >= max {
		t.purge(now, expire)
		if len(t.peers) >= max {
			// Desalojar la inalcanzable con `seen` más antiguo.
			oldest := ""
			var oldSeen time.Time
			for k, e := range t.peers {
				if now.Sub(e.seen) >= unreach && (oldest == "" || e.seen.Before(oldSeen)) {
					oldest, oldSeen = k, e.seen
				}
			}
			if oldest == "" {
				return
			}
			delete(t.peers, oldest)
		}
	}
	t.peers[a.InstanceID] = &entry{a: a, seen: now}
}

func (t *Table) purge(now time.Time, expire time.Duration) {
	for k, e := range t.peers {
		if now.Sub(e.seen) >= expire {
			delete(t.peers, k)
		}
	}
}

// Snapshot devuelve los equipos ordenados por alias, luego IP y id; elimina las entradas muy viejas.
func (t *Table) Snapshot() []Peer {
	unreach, expire, _ := t.limits()
	now := t.now()
	t.mu.Lock()
	t.purge(now, expire)
	out := make([]Peer, 0, len(t.peers))
	for _, e := range t.peers {
		out = append(out, Peer{Announce: e.a, Reachable: now.Sub(e.seen) < unreach, LastSeen: e.seen})
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Alias != b.Alias {
			return a.Alias < b.Alias
		}
		if c := a.IP.Compare(b.IP); c != 0 {
			return c < 0
		}
		return a.InstanceID < b.InstanceID
	})
	return out
}

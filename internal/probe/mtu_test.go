package probe

import (
	"context"
	"net/netip"
	"slices"
	"testing"
	"time"
)

// mtuEcho simula una ruta: tamaños IP > path no pasan (frag_needed o timeout).
type mtuEcho struct {
	path    int
	mute    bool
	blackh  bool
	sizes   []int
	cancel  func()
	cancelN int
}

func (m *mtuEcho) Echo(_ context.Context, r EchoRequest) (Reply, error) {
	m.sizes = append(m.sizes, r.Size+28)
	if m.cancel != nil && len(m.sizes) == m.cancelN {
		m.cancel()
	}
	if !r.DF {
		panic("DF requerido")
	}
	switch {
	case m.mute:
		return Reply{Status: StatusTimeout}, nil
	case r.Size+28 <= m.path:
		return Reply{Status: StatusOK}, nil
	case m.blackh:
		return Reply{Status: StatusTimeout}, nil
	}
	return Reply{Status: StatusFragNeeded}, nil
}

var mtuDst = netip.MustParseAddr("192.0.2.1")

func TestMTUPaths(t *testing.T) {
	for _, p := range []int{1500, 1492, 1400, 576, 577, 1499} {
		m := &mtuEcho{path: p}
		r, err := MTU(context.Background(), m, mtuDst, 0, time.Second)
		if err != nil || !r.Available || r.MTU != p || r.MaxPayload != p-28 {
			t.Fatalf("path %d: %+v %v", p, r, err)
		}
		if r.Probes > 20 || r.Probes != len(m.sizes) || len(r.Notes) != 0 {
			t.Fatalf("path %d: sondas %d notas %v", p, r.Probes, r.Notes)
		}
	}
}

func TestMTUInterfaceBound(t *testing.T) {
	m := &mtuEcho{path: 9000}
	r, _ := MTU(context.Background(), m, mtuDst, 9000, time.Second)
	if r.MTU != 9000 || r.Probes > 20 {
		t.Fatalf("%+v", r)
	}
	if slices.Max(m.sizes) > 9000 {
		t.Fatal("excede el MTU de interfaz")
	}
	r, _ = MTU(context.Background(), &mtuEcho{path: 1500}, mtuDst, 100, time.Second)
	if r.InterfaceMTU != 1500 {
		t.Fatalf("desconocido debe ser 1500: %+v", r)
	}
}

func TestMTUMute(t *testing.T) {
	m := &mtuEcho{mute: true}
	r, err := MTU(context.Background(), m, mtuDst, 1500, time.Second)
	if err != nil || r.Available || r.Reason != "el destino no responde a ICMP" || r.Probes != 2 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestMTUBlackHole(t *testing.T) {
	m := &mtuEcho{path: 1400, blackh: true}
	r, err := MTU(context.Background(), m, mtuDst, 1500, time.Second)
	if err != nil || !r.Available || r.Probes > 20 || r.MTU < 576 || r.MTU > 1400 {
		t.Fatalf("%+v %v", r, err)
	}
	if !slices.Contains(r.Notes, "posible agujero negro de MTU") {
		t.Fatalf("falta nota: %v", r.Notes)
	}
}

func TestMTUCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &mtuEcho{path: 1400, cancel: cancel, cancelN: 3}
	_, err := MTU(ctx, m, mtuDst, 1500, time.Second)
	if err == nil || len(m.sizes) != 3 {
		t.Fatalf("err=%v sondas=%d", err, len(m.sizes))
	}
}

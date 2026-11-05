package proto

import (
	"net"
	"strings"
	"testing"

	"nperf/internal/auth"
)

// fakeServer firma con la clave correcta; tamper altera Session tras firmar.
func fakeServer(t *testing.T, key string, tamper bool) *Codec {
	t.Helper()
	cs, ss := net.Pipe()
	t.Cleanup(func() { cs.Close(); ss.Close() })
	go func() {
		c := NewCodec(ss)
		h, err := c.Read()
		if err != nil {
			return
		}
		c.Write(Message{Type: TypeHello, Version: h.Version, Nonce: "sn"})
		a, err := c.Read()
		if err != nil {
			return
		}
		sess := "sesion-real"
		mac := auth.Sum(key, auth.RoleServer, "sn", a.Nonce, sess)
		if tamper {
			sess = "sesion-falsa"
		}
		c.Write(Message{Type: TypeAuth, MAC: mac, Session: sess})
	}()
	return NewCodec(cs)
}

func TestSessionTamperedRejected(t *testing.T) {
	id, err := ClientHandshake(fakeServer(t, "k", true), "k", "0.1.0")
	if err == nil || !strings.Contains(err.Error(), ErrBadKey) || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestSessionUntamperedAccepted(t *testing.T) {
	id, err := ClientHandshake(fakeServer(t, "k", false), "k", "0.1.0")
	if err != nil || id != "sesion-real" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

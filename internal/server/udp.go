package server

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
)

// Saludo UDP: UDPMagic + id de sesión. El servidor responde UDPMagic + "ok".
const UDPMagic = "NPUDP1 "

const udpQueue = 8192

type udpState struct {
	mu     sync.Mutex
	conn   *net.UDPConn
	byID   map[string]*udpSession
	byAddr map[netip.AddrPort]*udpSession
}

// udpSession es el lector virtual de los datagramas de una sesión (cumple meter.PacketReader).
type udpSession struct {
	addr     netip.AddrPort
	q        chan []byte
	wake     chan struct{}
	mu       sync.Mutex
	deadline time.Time
}

func (u *udpSession) SetReadDeadline(t time.Time) error {
	u.mu.Lock()
	u.deadline = t
	u.mu.Unlock()
	select {
	case u.wake <- struct{}{}:
	default:
	}
	return nil
}

func (u *udpSession) Read(b []byte) (int, error) {
	for {
		select {
		case p := <-u.q:
			return copy(b, p), nil
		default:
		}
		u.mu.Lock()
		d := u.deadline
		u.mu.Unlock()
		var tc <-chan time.Time
		if !d.IsZero() {
			dur := time.Until(d)
			if dur <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			t := time.NewTimer(dur)
			tc = t.C
			select {
			case p := <-u.q:
				t.Stop()
				return copy(b, p), nil
			case <-tc:
				return 0, os.ErrDeadlineExceeded
			case <-u.wake:
				t.Stop()
			}
			continue
		}
		select {
		case p := <-u.q:
			return copy(b, p), nil
		case <-u.wake:
		}
	}
}

// startUDP abre el socket UDP en el mismo puerto que el listener TCP y lanza el despachador.
func (s *Server) startUDP(ctx context.Context, ln net.Listener) error {
	ta, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return nil
	}
	uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: ta.IP, Port: ta.Port, Zone: ta.Zone})
	if err != nil {
		return err
	}
	uc.SetReadBuffer(4 << 20)
	s.udp.mu.Lock()
	s.udp.conn = uc
	s.udp.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { uc.Close() }) // desbloquea la lectura al cancelar
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer stop()
		defer uc.Close()
		s.dispatchUDP(uc)
	}()
	return nil
}

func (s *Server) dispatchUDP(uc *net.UDPConn) {
	buf := make([]byte, 65536)
	ack := []byte(UDPMagic + "ok")
	for {
		n, ap, err := uc.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if n == 0 && isClosed(err) {
				return
			}
			continue
		}
		pkt := buf[:n]
		s.udp.mu.Lock()
		if bytes.HasPrefix(pkt, []byte(UDPMagic)) {
			sess := s.udp.byID[string(pkt[len(UDPMagic):])]
			ok := false
			if sess != nil {
				switch {
				case !sess.addr.IsValid(): // primera asociación
					sess.addr = ap
					s.udp.byAddr[ap] = sess
					ok = true
				case sess.addr == ap: // reintento legítimo
					ok = true
				} // otra dirección: se ignora, sin acuse
			}
			s.udp.mu.Unlock()
			if ok {
				uc.WriteToUDPAddrPort(ack, ap)
			}
			continue
		}
		sess := s.udp.byAddr[ap]
		s.udp.mu.Unlock()
		if sess != nil {
			select {
			case sess.q <- append([]byte(nil), pkt...):
			default: // cola llena: se cuenta como pérdida
			}
		}
	}
}

func isClosed(err error) bool {
	return err != nil && (errors.Is(err, net.ErrClosed))
}

func (s *Server) controlUDP(ctx context.Context, codec *proto.Codec, id string, p proto.Message) {
	fail := func(msg string) { codec.Write(proto.Message{Type: proto.TypeError, Error: msg}) }
	s.udp.mu.Lock()
	ready := s.udp.conn != nil
	var sess *udpSession
	if ready {
		sess = &udpSession{q: make(chan []byte, udpQueue), wake: make(chan struct{}, 1)}
		s.udp.byID[id] = sess
	}
	s.udp.mu.Unlock()
	if !ready {
		fail("UDP no disponible en este servidor")
		return
	}
	defer func() {
		s.udp.mu.Lock()
		delete(s.udp.byID, id)
		if sess.addr.IsValid() && s.udp.byAddr[sess.addr] == sess {
			delete(s.udp.byAddr, sess.addr)
		}
		s.udp.mu.Unlock()
	}()
	if err := codec.Write(proto.Message{Type: proto.TypeStart}); err != nil {
		return
	}
	d := time.Duration(p.DurationMS) * time.Millisecond
	rctx, cancel := context.WithTimeout(ctx, d+margin)
	defer cancel()
	sess.SetReadDeadline(time.Now().Add(d + margin))
	st, err := meter.ReceiveUDP(rctx, sess, time.Second)
	if ctx.Err() != nil {
		return
	}
	if st.Received == 0 {
		fail("no llegaron datagramas UDP")
		return
	}
	if err != nil {
		fail("error de medición: " + err.Error())
		return
	}
	codec.Write(proto.Message{Type: proto.TypeResult, Bytes: st.Bytes, DurationMS: st.Elapsed.Milliseconds(),
		Mbps: st.Mbps(), Received: st.Received, Expected: st.Expected, Lost: st.Lost,
		LossPct: st.LossPercent(), JitterMS: float64(st.Jitter) / float64(time.Millisecond)})
}

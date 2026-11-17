package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"nperf/internal/meter"
	"nperf/internal/proto"
	"nperf/internal/server"
	"nperf/internal/testutil"
	"nperf/internal/version"
)

// liveSession abre un control autenticado con turno concedido.
func liveSession(t *testing.T, addr string) (net.Conn, *proto.Codec, string) {
	t.Helper()
	cc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cc.Close() })
	cc.SetDeadline(time.Now().Add(30 * time.Second))
	codec := proto.NewCodec(cc)
	id, err := proto.ClientHandshake(codec, "", version.Current)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.ClientTurn(codec, nil); err != nil {
		t.Fatal(err)
	}
	return cc, codec, id
}

// tcpFwdPhase lanza una fase TCP fwd de 2 s con 1 flujo; send se ejecuta durante la medición
// (después de start). Devuelve start y todos los mensajes hasta result.
func tcpFwdPhase(t *testing.T, addr string, live bool, during func(*proto.Codec)) (proto.Message, []proto.Message) {
	t.Helper()
	_, codec, id := liveSession(t, addr)
	return phaseOn(t, addr, codec, id, live, during)
}

func phaseOn(t *testing.T, addr string, codec *proto.Codec, id string, live bool, during func(*proto.Codec)) (proto.Message, []proto.Message) {
	t.Helper()
	d := 2 * time.Second
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoTCP, Direction: proto.DirFwd,
		DurationMS: d.Milliseconds(), Streams: 1, Live: live}); err != nil {
		t.Fatal(err)
	}
	start, err := codec.Read()
	if err != nil || start.Type != proto.TypeStart {
		t.Fatalf("start: %+v %v", start, err)
	}
	dc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Close()
	if err := proto.NewCodec(dc).Write(proto.Message{Type: proto.TypeData, Session: id}); err != nil {
		t.Fatal(err)
	}
	go meter.SendTCP(context.Background(), dc, d)
	if during != nil {
		go during(codec)
	}
	var msgs []proto.Message
	for {
		m, err := codec.Read()
		if err != nil {
			t.Fatalf("leyendo: %v (recibidos %d)", err, len(msgs))
		}
		msgs = append(msgs, m)
		if m.Type == proto.TypeResult || m.Type == proto.TypeError {
			return start, msgs
		}
	}
}

func startServer(t *testing.T) string {
	t.Helper()
	ln, uc := testutil.Listen(t)
	srv := server.New(server.WithUDPConn(uc))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { testutil.ServeErr(t, ctx, srv.Serve(ctx, ln)); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String()
}

func TestLiveTCPSamplesAndPongs(t *testing.T) {
	addr := startServer(t)
	start, msgs := tcpFwdPhase(t, addr, true, func(c *proto.Codec) {
		for seq := int64(1); seq <= 3; seq++ {
			c.Write(proto.Message{Type: proto.TypePing, Seq: seq})
			time.Sleep(400 * time.Millisecond)
		}
	})
	if !start.Live {
		t.Fatal("start sin live")
	}
	res := msgs[len(msgs)-1]
	if res.Type != proto.TypeResult {
		t.Fatalf("último: %+v", res)
	}
	var sum int64
	samples, pongs := 0, 0
	for i, m := range msgs[:len(msgs)-1] {
		switch m.Type {
		case proto.TypeSample:
			s, err := proto.SampleFromMessage(m, samples-1, 2000)
			if err != nil || s.Index != samples {
				t.Fatalf("muestra %d: %+v %v", i, m, err)
			}
			sum += s.Bytes
			samples++
		case proto.TypePong:
			pongs++
			if m.Seq != int64(pongs) {
				t.Errorf("pong seq %d, esperado %d", m.Seq, pongs)
			}
		default:
			t.Errorf("mensaje inesperado %+v", m)
		}
	}
	if samples != 2 || pongs != 3 {
		t.Fatalf("muestras=%d pongs=%d", samples, pongs)
	}
	if diff := float64(res.Bytes-sum) / float64(res.Bytes); diff < -0.02 || diff > 0.02 {
		t.Errorf("suma de muestras %d vs result %d", sum, res.Bytes)
	}
}

func TestNoLiveNoSamplesNoPongs(t *testing.T) {
	addr := startServer(t)
	start, msgs := tcpFwdPhase(t, addr, false, nil)
	if start.Live {
		t.Error("start live sin pedirlo")
	}
	if len(msgs) != 1 || msgs[0].Type != proto.TypeResult {
		t.Fatalf("esperaba solo result: %+v", msgs)
	}
}

func TestLivePingBeforeResultThenNextPhase(t *testing.T) {
	addr := startServer(t)
	_, codec, id := liveSession(t, addr)
	// Ping justo antes de que acabe la fase: o se contesta en fase o entre fases.
	_, msgs := phaseOn(t, addr, codec, id, true, func(c *proto.Codec) {
		time.Sleep(1990 * time.Millisecond)
		c.Write(proto.Message{Type: proto.TypePing, Seq: 7})
	})
	pongs := 0
	for _, m := range msgs {
		if m.Type == proto.TypePong {
			pongs++
		}
	}
	// Siguiente fase: el pong tardío puede llegar antes de start.
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoTCP, Direction: proto.DirRev,
		DurationMS: 1000, Streams: 1, Live: true}); err != nil {
		t.Fatal(err)
	}
	for {
		m, err := codec.Read()
		if err != nil {
			t.Fatal(err)
		}
		if m.Type == proto.TypePong {
			pongs++
			continue
		}
		if m.Type != proto.TypeStart || !m.Live {
			t.Fatalf("esperaba start live: %+v", m)
		}
		break
	}
	if pongs != 1 {
		t.Errorf("pongs=%d, esperado 1", pongs)
	}
}

// udpFwdPhase lanza una fase UDP fwd de 2 s a 20 Mbps; devuelve start y los mensajes hasta result.
func udpFwdPhase(t *testing.T, addr string, live bool, during func(*proto.Codec)) (proto.Message, []proto.Message) {
	t.Helper()
	_, codec, id := liveSession(t, addr)
	d := 2 * time.Second
	if err := codec.Write(proto.Message{Type: proto.TypeParams, Proto: proto.ProtoUDP, Direction: proto.DirFwd,
		DurationMS: d.Milliseconds(), RateBps: 20e6, Live: live}); err != nil {
		t.Fatal(err)
	}
	start, err := codec.Read()
	if err != nil || start.Type != proto.TypeStart {
		t.Fatalf("start: %+v %v", start, err)
	}
	uc, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	buf := make([]byte, 64)
	for i := 0; ; i++ {
		if i > 20 {
			t.Fatal("sin acuse UDP")
		}
		uc.Write([]byte(server.UDPMagic + id))
		uc.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		if _, err := uc.Read(buf); err == nil {
			break
		}
	}
	go meter.SendUDP(context.Background(), uc, 20e6, meter.DefaultUDPSize, d)
	if during != nil {
		go during(codec)
	}
	var msgs []proto.Message
	for {
		m, err := codec.Read()
		if err != nil {
			t.Fatalf("leyendo: %v (recibidos %d)", err, len(msgs))
		}
		msgs = append(msgs, m)
		if m.Type == proto.TypeResult || m.Type == proto.TypeError {
			return start, msgs
		}
	}
}

func TestLiveUDPSamplesAndPongs(t *testing.T) {
	addr := startServer(t)
	start, msgs := udpFwdPhase(t, addr, true, func(c *proto.Codec) {
		for seq := int64(1); seq <= 2; seq++ {
			c.Write(proto.Message{Type: proto.TypePing, Seq: seq})
			time.Sleep(500 * time.Millisecond)
		}
	})
	if !start.Live {
		t.Fatal("start sin live")
	}
	res := msgs[len(msgs)-1]
	if res.Type != proto.TypeResult {
		t.Fatalf("último: %+v", res)
	}
	var sum, rcv int64
	samples, pongs := 0, 0
	for _, m := range msgs[:len(msgs)-1] {
		switch m.Type {
		case proto.TypeSample:
			s, err := proto.SampleFromMessage(m, samples-1, 2000)
			if err != nil || s.Index != samples {
				t.Fatalf("muestra: %+v %v", m, err)
			}
			if s.Expected < s.Received || s.Lost != s.Expected-s.Received && s.Lost != 0 {
				t.Errorf("datagramas incoherentes: %+v", s)
			}
			sum += s.Bytes
			rcv += s.Received
			samples++
		case proto.TypePong:
			pongs++
		default:
			t.Errorf("mensaje inesperado %+v", m)
		}
	}
	if samples != 2 || pongs != 2 {
		t.Fatalf("muestras=%d pongs=%d", samples, pongs)
	}
	if d := float64(res.Bytes-sum) / float64(res.Bytes); d < -0.02 || d > 0.02 {
		t.Errorf("suma de muestras %d vs result %d", sum, res.Bytes)
	}
	if rcv != res.Received {
		t.Errorf("recibidos en muestras %d vs result %d", rcv, res.Received)
	}
}

func TestNoLiveUDPOnlyResult(t *testing.T) {
	addr := startServer(t)
	start, msgs := udpFwdPhase(t, addr, false, nil)
	if start.Live || len(msgs) != 1 || msgs[0].Type != proto.TypeResult {
		t.Fatalf("start=%+v msgs=%+v", start, msgs)
	}
}

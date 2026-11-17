package proto

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestLegacyMessagesUnchanged(t *testing.T) {
	var buf bytes.Buffer
	c := NewCodec(&buf)
	_ = c.Write(Message{Type: TypeParams, Proto: "tcp", Direction: "fwd", DurationMS: 5000, Streams: 4})
	_ = c.Write(Message{Type: TypeStart})
	_ = c.Write(Message{Type: TypePing})
	want := `{"type":"params","proto":"tcp","direction":"fwd","duration_ms":5000,"streams":4}` + "\n" +
		`{"type":"start"}` + "\n" + `{"type":"ping"}` + "\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}

func TestSampleRoundTrip(t *testing.T) {
	in := Sample{Index: 3, TMS: 3000, DurationMS: 1000, Bytes: 1234, Mbps: 9.9, Received: 10, Expected: 12, Lost: 2, LossPct: 16.5, JitterMS: 0.4}
	var buf bytes.Buffer
	c := NewCodec(&buf)
	if err := c.Write(SampleMessage(in)); err != nil {
		t.Fatal(err)
	}
	m, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	out, err := SampleFromMessage(m, 2, 10000)
	if err != nil || out != in {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	// Index 0 (omitempty) y seq/live ida y vuelta.
	buf.Reset()
	_ = c.Write(Message{Type: TypeStart, Live: true})
	_ = c.Write(Message{Type: TypePong, Seq: 7})
	a, _ := c.Read()
	b, _ := c.Read()
	if !a.Live || b.Seq != 7 {
		t.Fatal("live/seq perdidos")
	}
	z, err := SampleFromMessage(Message{Type: TypeSample}, -1, 1000)
	if err != nil || z.Index != 0 {
		t.Fatalf("%+v %v", z, err)
	}
}

func TestSampleRejections(t *testing.T) {
	ok := Message{Type: TypeSample, Index: 2, DurationMS: 1000}
	cases := []struct {
		name string
		m    Message
		last int
		dur  int64
	}{
		{"tipo", Message{Type: TypePong}, -1, 5000},
		{"índice negativo", Message{Type: TypeSample, Index: -1}, -1, 5000},
		{"no creciente", ok, 2, 5000},
		{"retrocede", ok, 3, 5000},
		{"fuera de ⌈D⌉", Message{Type: TypeSample, Index: 5}, 4, 5000},
		{"sobre 300", Message{Type: TypeSample, Index: 300}, 299, 400000},
		{"duración cero", ok, -1, 0},
		{"t negativo", Message{Type: TypeSample, TMS: -1}, -1, 5000},
		{"bytes negativos", Message{Type: TypeSample, Bytes: -1}, -1, 5000},
		{"mbps negativo", Message{Type: TypeSample, Mbps: -1}, -1, 5000},
		{"lost negativo", Message{Type: TypeSample, Lost: -1}, -1, 5000},
		{"loss > 100", Message{Type: TypeSample, LossPct: 100.1}, -1, 5000},
		{"mbps > 1T", Message{Type: TypeSample, Mbps: 1000001}, -1, 5000},
		{"jitter > 60 s", Message{Type: TypeSample, JitterMS: 60001}, -1, 5000},
		{"bytes incoherentes", Message{Type: TypeSample, DurationMS: 1000, Bytes: 125000000001}, -1, 5000},
		{"bytes sin duración", Message{Type: TypeSample, Bytes: 1}, -1, 5000},
		{"duración > 300 s", Message{Type: TypeSample, DurationMS: 300001}, -1, 5000},
		{"jitter negativo", Message{Type: TypeSample, JitterMS: -0.1}, -1, 5000},
	}
	for _, c := range cases {
		if _, err := SampleFromMessage(c.m, c.last, c.dur); err == nil {
			t.Errorf("%s: debía rechazarse", c.name)
		}
	}
	if _, err := SampleFromMessage(Message{Type: TypeSample, Index: 4}, 3, 4001); err != nil {
		t.Errorf("⌈4.001⌉=5, índice 4 válido: %v", err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	c := NewCodec(&buf)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = c.Write(Message{Type: TypeSample, Index: i, Bytes: 1})
			}
		}()
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 1600 {
		t.Fatalf("líneas %d", len(lines))
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, `{"type":"sample"`) || !strings.HasSuffix(l, "}") {
			t.Fatalf("línea mezclada: %q", l)
		}
	}
}

// Un loopback rápido (> 100 Gbps) o una ventana de 1 ms no son muestras inválidas.
func TestSampleFromMessageFastLoopback(t *testing.T) {
	m := Message{Type: TypeSample, DurationMS: 1000, Bytes: 20e9, Mbps: 160000}
	if _, err := SampleFromMessage(m, -1, 5000); err != nil {
		t.Fatal(err)
	}
}

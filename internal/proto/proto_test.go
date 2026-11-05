package proto

import (
	"bytes"
	"strings"
	"testing"
)

type rw struct {
	*bytes.Buffer
}

func TestRoundTrip(t *testing.T) {
	b := rw{&bytes.Buffer{}}
	c := NewCodec(b)
	in := Message{Type: TypeResult, Bytes: 42, DurationMS: 1000, Mbps: 1.5}
	if err := c.Write(in); err != nil {
		t.Fatal(err)
	}
	out, err := c.Read()
	if err != nil || out != in {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestInvalid(t *testing.T) {
	c := NewCodec(rw{bytes.NewBufferString("no es json\n")})
	if _, err := c.Read(); err == nil || !strings.Contains(err.Error(), "inválido") {
		t.Fatalf("se esperaba error, got %v", err)
	}
}

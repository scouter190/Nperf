package proto

import (
	"bytes"
	"encoding/json"
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

func TestRetransOptional(t *testing.T) {
	var none, some Message
	n := uint64(0)
	some.Retrans = &n
	for _, tc := range []struct {
		m    Message
		want bool
	}{{none, false}, {some, true}} {
		b, _ := json.Marshal(tc.m)
		var back Message
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if (back.Retrans != nil) != tc.want {
			t.Errorf("%s: presente=%v, quería %v", b, back.Retrans != nil, tc.want)
		}
	}
}

func TestLinkSanitize(t *testing.T) {
	var nl *Link
	if nl.Sanitize() != nil {
		t.Fatal("nil debe seguir nil")
	}
	h := (&Link{Kind: "<b>", TxBps: -5, RxBps: 1 << 60, SSID: "a\x00b", Band: "0123456789abcdefX", Standard: "w\ni", SignalPct: 150}).Sanitize()
	if *h != (Link{}) {
		t.Fatalf("hostil no saneado: %+v", *h)
	}
	if (&Link{SSID: strings.Repeat("a", 33)}).Sanitize().SSID != "" {
		t.Fatal("SSID de 33 B")
	}
	ok := Link{Kind: "wifi", TxBps: 1e9, RxBps: 2e9, SSID: "casa", Band: "5 GHz", Standard: "WiFi 6", SignalPct: 100}
	if g := ok.Sanitize(); *g != ok {
		t.Fatalf("válido alterado: %+v", *g)
	}
}

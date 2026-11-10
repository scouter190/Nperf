package gui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const sec = "s3creto"

type fake struct {
	calls []string
	err   error
}

func (f *fake) rec(s string) error { f.calls = append(f.calls, s); return f.err }
func (f *fake) State() any         { f.calls = append(f.calls, "state"); return map[string]int{"x": 1} }
func (f *fake) SelectPeer(s string) error {
	return f.rec("selectPeer:" + s)
}
func (f *fake) SetDirection(s string) error { return f.rec("setDirection:" + s) }
func (f *fake) SetDuration(n int) error {
	return f.rec("setDuration:" + string(rune('0'+n%10)))
}
func (f *fake) SetAlias(s string) error     { return f.rec("setAlias:" + s) }
func (f *fake) SetKey(s string) error       { return f.rec("setKey:" + s) }
func (f *fake) AddSubnet(s string) error    { return f.rec("addSubnet:" + s) }
func (f *fake) RemoveSubnet(s string) error { return f.rec("removeSubnet:" + s) }
func (f *fake) StartTest() error            { return f.rec("startTest") }
func (f *fake) CancelTest()                 { f.calls = append(f.calls, "cancelTest") }

func (f *fake) Adapters(d bool) (any, error) {
	if d {
		return "det", f.rec("adapters:true")
	}
	return "cmp", f.rec("adapters:false")
}
func (f *fake) OpenLocationSettings() error { return f.rec("openLocationSettings") }

func TestBridgeAdapters(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f}
	if v, err := b.Call(sec, "adapters", raw(`true`)...); err != nil || v != "det" {
		t.Fatalf("adapters true: %v %v", v, err)
	}
	if _, err := b.Call(sec, "openLocationSettings"); err != nil {
		t.Fatal(err)
	}
	bad := []struct {
		name string
		args []json.RawMessage
	}{
		{"adapters", nil}, {"adapters", raw(`"true"`)}, {"adapters", raw(`null`)},
		{"adapters", raw(`true`, `false`)}, {"openLocationSettings", raw(`"ms-settings:x"`)},
	}
	for _, c := range bad {
		if _, err := b.Call(sec, c.name, c.args...); err == nil {
			t.Errorf("%s %v debía fallar", c.name, c.args)
		}
	}
	if len(f.calls) != 2 {
		t.Errorf("llamadas inesperadas: %v", f.calls)
	}
}

func raw(vs ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(vs))
	for i, v := range vs {
		out[i] = json.RawMessage(v)
	}
	return out
}

func TestBridgeValid(t *testing.T) {
	f := &fake{}
	changes := 0
	b := &Bridge{Secret: sec, B: f, OnConfigChange: func() { changes++ }}
	cases := []struct {
		name string
		args []json.RawMessage
		want string
	}{
		{"state", nil, "state"},
		{"selectPeer", raw(`"abc"`), "selectPeer:abc"},
		{"setDirection", raw(`"fwd"`), "setDirection:fwd"},
		{"setDuration", raw(`7`), "setDuration:7"},
		{"setAlias", raw(`"PC"`), "setAlias:PC"},
		{"setKey", raw(`"k"`), "setKey:k"},
		{"addSubnet", raw(`"10.0.0.0/24"`), "addSubnet:10.0.0.0/24"},
		{"removeSubnet", raw(`"10.0.0.0/24"`), "removeSubnet:10.0.0.0/24"},
		{"startTest", nil, "startTest"},
		{"cancelTest", nil, "cancelTest"},
	}
	for _, c := range cases {
		if _, err := b.Call(sec, c.name, c.args...); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	for i, c := range cases {
		if f.calls[i] != c.want {
			t.Errorf("llamada %d = %q, quería %q", i, f.calls[i], c.want)
		}
	}
	if changes != 4 {
		t.Errorf("OnConfigChange = %d veces, quería 4 (alias, clave, 2 subredes)", changes)
	}
	if v, _ := b.Call(sec, "state"); v == nil {
		t.Error("state debe devolver el snapshot")
	}
}

func TestBridgeRejects(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f, OnConfigChange: func() { t.Error("no debe notificar cambios") }}
	bad := []struct {
		name string
		args []json.RawMessage
	}{
		{"desconocido", nil},
		{"State", nil},
		{"__proto__", nil},
		{"", nil},
		{"state", raw(`1`)},
		{"startTest", raw(`"x"`)},
		{"cancelTest", raw(`1`)},
		{"selectPeer", nil},
		{"selectPeer", raw(`1`)},
		{"selectPeer", raw(`null`)},
		{"selectPeer", raw(`"a"`, `"b"`)},
		{"setAlias", raw(`{"a":1}`)},
		{"setKey", raw(`["k"]`)},
		{"addSubnet", raw(`true`)},
		{"setDuration", raw(`"10"`)},
		{"setDuration", raw(`1.5`)},
		{"setDuration", raw(`null`)},
		{"setDuration", nil},
	}
	for _, c := range bad {
		if _, err := b.Call(sec, c.name, c.args...); err == nil {
			t.Errorf("%s %v debía fallar", c.name, c.args)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("no debió llegar nada al backend: %v", f.calls)
	}
}

func TestBridgeBackendErrorNoRestart(t *testing.T) {
	f := &fake{err: errors.New("fallo")}
	b := &Bridge{Secret: sec, B: f, OnConfigChange: func() { t.Error("no reinicia si falla el guardado") }}
	_, err := b.Call(sec, "setAlias", json.RawMessage(`"x"`))
	if err == nil || !strings.Contains(err.Error(), "fallo") {
		t.Fatalf("err = %v", err)
	}
}

func TestBridgeSecret(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f, OnConfigChange: func() { t.Error("sin efectos") }}
	for _, bad := range []string{"", "otro", sec + "x", sec[:3]} {
		for _, n := range []string{"state", "startTest", "setKey", "desconocido"} {
			_, err := b.Call(bad, n, json.RawMessage(`"k"`))
			if err == nil || err.Error() != "no autorizado" {
				t.Errorf("secreto %q método %s: err=%v", bad, n, err)
			}
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("efectos con secreto incorrecto: %v", f.calls)
	}
	if _, err := (&Bridge{B: f}).Call("", "state"); err == nil {
		t.Fatal("un puente sin secreto configurado no debe aceptar nada")
	}
	if _, err := b.Call(sec, "state"); err != nil {
		t.Fatal(err)
	}
}

func TestNewSecretUnique(t *testing.T) {
	a, _ := NewSecret()
	c, _ := NewSecret()
	if len(a) != 64 || a == c {
		t.Fatalf("secretos: %q %q", a, c)
	}
}

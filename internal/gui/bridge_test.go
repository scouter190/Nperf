package gui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"nperf/internal/app"
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
func (f *fake) AddHost(t, a string) error  { return f.rec("addHost:" + t + "|" + a) }
func (f *fake) RemoveHost(id string) error { return f.rec("removeHost:" + id) }
func (f *fake) StartProbe(k, id string) error {
	return f.rec("startProbe:" + k + "|" + id)
}
func (f *fake) Monitor(a bool) (any, error) {
	if a {
		return "on", f.rec("monitor:true")
	}
	return "off", f.rec("monitor:false")
}
func (f *fake) CancelProbe()                { f.calls = append(f.calls, "cancelProbe") }
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
		{"addHost", raw(`"10.0.0.9"`, `"NAS"`), "addHost:10.0.0.9|NAS"},
		{"removeHost", raw(`"h1"`), "removeHost:h1"},
		{"startProbe", raw(`"ping"`, `"h1"`), "startProbe:ping|h1"},
		{"startProbe", raw(`"trace"`, `"h1"`), "startProbe:trace|h1"},
		{"startProbe", raw(`"mtu"`, `"h1"`), "startProbe:mtu|h1"},
		{"startProbe", raw(`"ports"`, `"h1"`), "startProbe:ports|h1"},
		{"cancelProbe", nil, "cancelProbe"},
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
		{"addHost", raw(`"a"`)},
		{"addHost", raw(`"a"`, `1`)},
		{"addHost", raw(`null`, `"a"`)},
		{"addHost", raw(`"a"`, `"b"`, `"c"`)},
		{"addHost", raw(`"`+strings.Repeat("x", 256)+`"`, `"a"`)},
		{"addHost", raw(`"a"`, `"`+strings.Repeat("x", 129)+`"`)},
		{"removeHost", nil},
		{"removeHost", raw(`1`)},
		{"removeHost", raw(`"a"`, `"b"`)},
		{"startProbe", raw(`"ping"`)},
		{"startProbe", raw(`"nmap"`, `"h1"`)},
		{"startProbe", raw(`"PING"`, `"h1"`)},
		{"startProbe", raw(`"ping"`, `1`)},
		{"startProbe", raw(`"ping"`, `"h1"`, `"x"`)},
		{"cancelProbe", raw(`"x"`)},
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

func TestBridgeMonitor(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f}
	if v, err := b.Call(sec, "monitor", raw(`true`)...); err != nil || v != "on" {
		t.Fatalf("monitor true: %v %v", v, err)
	}
	if v, err := b.Call(sec, "monitor", raw(`false`)...); err != nil || v != "off" {
		t.Fatalf("monitor false: %v %v", v, err)
	}
	for _, a := range [][]json.RawMessage{nil, raw(`"true"`), raw(`1`), raw(`null`), raw(`true`, `false`)} {
		if _, err := b.Call(sec, "monitor", a...); err == nil {
			t.Errorf("monitor %v debía fallar", a)
		}
	}
	if len(f.calls) != 2 {
		t.Errorf("llamadas inesperadas: %v", f.calls)
	}
}

func (f *fake) History(h, k string, s, u time.Time) (any, error) {
	return "hist", f.rec("history:" + h + "|" + k + "|" + s.UTC().Format(time.RFC3339))
}
func (f *fake) HistoryRecord(id string) (any, error) { return "rec", f.rec("historyRecord:" + id) }
func (f *fake) DeleteHistory(ids []string) (int, error) {
	return len(ids), f.rec("deleteHistory:" + strings.Join(ids, ","))
}
func (f *fake) DeleteAllHistory() (int, error) { return 7, f.rec("deleteAllHistory") }
func (f *fake) ExportHistory(ids []string, format string, o app.ExportOpts, path string) ([]string, error) {
	return []string{path}, f.rec("exportHistory:" + strings.Join(ids, ",") + "|" + format + "|" + o.Client + "|" + path)
}
func (f *fake) SetHistoryEnabled(on bool) error {
	if on {
		return f.rec("setHistoryEnabled:true")
	}
	return f.rec("setHistoryEnabled:false")
}
func (f *fake) SetInternetTargets(t []string) error {
	return f.rec("setInternetTargets:" + strings.Join(t, ","))
}
func (f *fake) StartInternet() error       { return f.rec("startInternet") }
func (f *fake) StartSpeed(c bool) error    { return f.rec("startSpeed") }
func (f *fake) OpenPrivacy(w string) error { return f.rec("openPrivacy:" + w) }

const (
	id1 = "20260102-030405-abcdef01"
	id2 = "20260102-030406-abcdef01"
)

func TestBridgeHistoryAccepted(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f}
	for _, c := range []struct {
		name, args, want string
	}{
		{"history", `{"host":"pc","kind":"test","since":"2026-01-02T03:04:05Z"}`, "history:pc|test|2026-01-02T03:04:05Z"},
		{"historyRecord", `"` + id1 + `"`, "historyRecord:" + id1},
		{"deleteHistory", `["` + id1 + `","` + id2 + `"]`, "deleteHistory:" + id1 + "," + id2},
		{"setHistoryEnabled", `false`, "setHistoryEnabled:false"},
		{"setInternetTargets", `["1.1.1.1","8.8.4.4"]`, "setInternetTargets:1.1.1.1,8.8.4.4"},
		{"openPrivacy", `"mlab"`, "openPrivacy:mlab"},
		{"startSpeed", `true`, "startSpeed"},
	} {
		f.calls = nil
		if _, err := b.Call(sec, c.name, rawj(c.args)...); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(f.calls) != 1 || f.calls[0] != c.want {
			t.Errorf("%s: %v, quería %q", c.name, f.calls, c.want)
		}
	}
	for _, n := range []string{"deleteAllHistory", "startInternet"} {
		if _, err := b.Call(sec, n); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
}

func TestBridgeHistoryRejected(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f, SaveDialog: func(string, string) (string, error) { return `C:\x.csv`, nil }}
	long := strings.Repeat("a", 300)
	bad := []struct{ name, args string }{
		{"history", ``},
		{"history", `{}, {}`},
		{"history", `"x"`},
		{"history", `null`},
		{"history", `{"path":"C:\\x"}`},
		{"history", `{"since":"ayer"}`},
		{"history", `{"host":"` + long + `"}`},
		{"historyRecord", ``},
		{"historyRecord", `"..\\..\\x"`},
		{"historyRecord", `"` + id1 + `","x"`},
		{"historyRecord", `5`},
		{"deleteHistory", `["../x"]`},
		{"deleteHistory", `"` + id1 + `"`},
		{"deleteHistory", `[5]`},
		{"deleteHistory", `["` + id1 + `"], 1`},
		{"deleteAllHistory", `1`},
		{"setHistoryEnabled", `"true"`},
		{"setHistoryEnabled", `true, false`},
		{"setHistoryEnabled", ``},
		{"setInternetTargets", `"1.1.1.1"`},
		{"setInternetTargets", `["1","2","3","4","5"]`},
		{"setInternetTargets", `["` + long + `"]`},
		{"setInternetTargets", `[1]`},
		{"startInternet", `1`},
		{"startSpeed", `false`},
		{"startSpeed", ``},
		{"startSpeed", `"true"`},
		{"startSpeed", `true, true`},
		{"openPrivacy", `"otra"`},
		{"openPrivacy", `"https://x.example"`},
		{"openPrivacy", ``},
		{"exportHistory", ``},
		{"exportHistory", `["` + id1 + `"], "csv"`},
		{"exportHistory", `[], "csv", {}`},
		{"exportHistory", `["` + id1 + `"], "exe", {}`},
		{"exportHistory", `["` + id1 + `"], "csv", {"path":"C:\\x.csv"}`},
		{"exportHistory", `["` + id1 + `"], "csv", {}, "C:\\x.csv"`},
		{"exportHistory", `["C:\\x"], "csv", {}`},
		{"exportHistory", `["` + id1 + `"], "csv", {"notes":"` + strings.Repeat("n", 2001) + `"}`},
		{"exportHistory", `["` + id1 + `"], 1, {}`},
	}
	for _, c := range bad {
		f.calls = nil
		var args []json.RawMessage
		if c.args != "" {
			args = rawj(c.args)
		}
		if _, err := b.Call(sec, c.name, args...); err == nil {
			t.Errorf("%s(%s) debería fallar", c.name, c.args)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s(%s) llegó al backend: %v", c.name, c.args, f.calls)
		}
	}
}

func TestBridgeExportPathFromDialog(t *testing.T) {
	f := &fake{}
	var gotFmt, gotName string
	b := &Bridge{Secret: sec, B: f, Now: func() time.Time { return time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC) },
		SaveDialog: func(format, sug string) (string, error) {
			gotFmt, gotName = format, sug
			return `C:\elegida\h.html`, nil
		}}
	v, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "html", {"client":"ACME"}`)...)
	if err != nil {
		t.Fatal(err)
	}
	if gotFmt != "html" || gotName != "nperf-historial-20260304.html" {
		t.Errorf("diálogo: %q %q", gotFmt, gotName)
	}
	if want := "exportHistory:" + id1 + "|html|ACME|" + `C:\elegida\h.html`; f.calls[0] != want {
		t.Errorf("llamada %q, quería %q", f.calls[0], want)
	}
	if p, ok := v.([]string); !ok || len(p) != 1 {
		t.Errorf("resultado %v", v)
	}
}

func TestBridgeExportCancelled(t *testing.T) {
	f := &fake{}
	b := &Bridge{Secret: sec, B: f, SaveDialog: func(string, string) (string, error) { return "", ErrSaveCancelled }}
	v, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...)
	if err != nil || v != "cancelado" || len(f.calls) != 0 {
		t.Fatalf("cancelado: %v %v %v", v, err, f.calls)
	}
	b.SaveDialog = func(string, string) (string, error) { return "", errors.New("boom") }
	if _, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...); err == nil || len(f.calls) != 0 {
		t.Fatal("el error del diálogo debe propagarse sin exportar")
	}
	b.SaveDialog = nil
	if _, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...); err == nil {
		t.Fatal("sin diálogo debe fallar")
	}
}

// rawj convierte una lista de argumentos JSON separados por comas en argumentos del puente.
func rawj(s string) []json.RawMessage {
	var out []json.RawMessage
	if err := json.Unmarshal([]byte("["+s+"]"), &out); err != nil {
		panic(err)
	}
	return out
}

func TestBridgeSaveDialogReentrant(t *testing.T) {
	f := &fake{}
	inside, release := make(chan struct{}), make(chan struct{})
	b := &Bridge{Secret: sec, B: f}
	b.SaveDialog = func(string, string) (string, error) {
		close(inside)
		<-release
		return "", ErrSaveCancelled
	}
	done := make(chan any)
	go func() {
		v, _ := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...)
		done <- v
	}()
	<-inside
	if _, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...); err == nil || !strings.Contains(err.Error(), "ya hay un diálogo") {
		t.Fatalf("segunda llamada debe rechazarse: %v", err)
	}
	if _, err := b.Call(sec, "startInternet"); err != nil {
		t.Fatalf("otros métodos deben funcionar: %v", err)
	}
	close(release)
	if v := <-done; v != "cancelado" {
		t.Fatalf("primera: %v", v)
	}
	b.SaveDialog = func(string, string) (string, error) { return "", ErrSaveCancelled }
	if v, err := b.Call(sec, "exportHistory", rawj(`["`+id1+`"], "csv", {}`)...); err != nil || v != "cancelado" {
		t.Fatalf("guard no liberado: %v %v", v, err)
	}
}

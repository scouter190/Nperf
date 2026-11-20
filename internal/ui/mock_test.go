package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"nperf/internal/app"
)

// jsonKeys devuelve las claves JSON de un struct.
func jsonKeys(typ reflect.Type) []string {
	var out []string
	for i := 0; i < typ.NumField(); i++ {
		if tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	return out
}

// El mock debe usar las mismas claves que el State real y cubrir los escenarios.
func TestMockKeysMatchState(t *testing.T) {
	mock := regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(read(t, "mock.js"), "")
	for _, typ := range []reflect.Type{reflect.TypeOf(app.State{}), reflect.TypeOf(app.TestState{}), reflect.TypeOf(app.ProbeState{})} {
		for _, k := range jsonKeys(typ) {
			if k == "history_id" {
				continue // se comprueba ejecutando el mock (TestMockScenariosHistoryID)
			}
			if !strings.Contains(mock, k+":") {
				t.Errorf("mock.js no usa la clave %s de %s", k, typ.Name())
			}
		}
	}
	for _, s := range []string{"vacio", "cola", "midiendo", "terminado", "cancelado", "error", "diagnostico", "internet", "velocidad", "historial_desactivado", "config_error"} {
		if !strings.Contains(mock, s+": function") {
			t.Errorf("mock.js no define el escenario %s", s)
		}
	}
	if !strings.Contains(mock, "'escenario'") {
		t.Error("mock.js debe leer ?escenario=")
	}
}

// Ejecuta mock.js con node por escenario y comprueba history_id en el objeto real.
func TestMockScenariosHistoryID(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	mock := filepath.Join(dir, "mock.js")
	if err := os.WriteFile(mock, []byte(read(t, "mock.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
const src=fs.readFileSync(process.argv[2],'utf8');
function load(q){const ctx={window:{location:{search:q}},URLSearchParams,setTimeout,clearTimeout,Date,JSON,Promise,Object,Math,Error};vm.createContext(ctx);vm.runInContext(src,ctx);return ctx.window.__mock.state;}
const withId={terminado:'test',diagnostico:'probe',internet:'probe',velocidad:'probe'};
for(const [e,k] of Object.entries(withId)){const s=load('?escenario='+e);assert(s[k].history_id,e+': falta history_id');}
for(const e of ['historial_desactivado','vacio','cola','midiendo','cancelado','error','config_error']){const s=load('?escenario='+e);assert(!s.test.history_id&&!s.probe.history_id,e+': history_id inesperado');}
const d=load('?escenario=historial_desactivado');assert.strictEqual(d.history_enabled,false);
assert.strictEqual(load('?escenario=config_error').config_error!=='',true);
assert.strictEqual(load('').test.status,'idle');
console.log('ok');`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, mock).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("escenarios del mock: %v\n%s", err, out)
	}
}

package ui

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"nperf/internal/app"
	"nperf/internal/ndt7"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(Files(), name)
	if err != nil {
		t.Fatalf("falta %s embebido: %v", name, err)
	}
	if len(b) == 0 {
		t.Fatalf("%s está vacío", name)
	}
	return string(b)
}

func TestEmbedded(t *testing.T) {
	for _, n := range []string{"index.html", "app.css", "app.js", "mock.js", "chart.js", "history.js", "internet.js", "shell.js", "sidebar.js", "panel.js", "drawers.js", "modals.js"} {
		read(t, n)
	}
}

func TestIndexSecurity(t *testing.T) {
	h := read(t, "index.html")
	if !strings.Contains(h, "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'none'") {
		t.Error("CSP estricta ausente")
	}
	if strings.Contains(h, "mock.js") {
		t.Error("index.html no debe cargar mock.js")
	}
	if regexp.MustCompile(`(?i)<script[^>]*>\s*[^<\s]|\son\w+\s*=|https?://`).MatchString(h) {
		t.Error("index.html no debe tener scripts en línea, manejadores on* ni URLs externas")
	}
}

var unsafeJS = regexp.MustCompile(`innerHTML|insertAdjacentHTML|outerHTML|document\.write|\beval\s*\(|\bFunction\s*\(|setAttribute|\.href\b|\.src\b|javascript:|\.on\w+\s*=`)

// unsafeUse devuelve los usos peligrosos del DOM encontrados en un JS.
func unsafeUse(js string) []string { return unsafeJS.FindAllString(js, -1) }

func TestNoUnsafeDOM(t *testing.T) {
	for _, n := range []string{"app.js", "mock.js", "history.js", "internet.js", "shell.js", "sidebar.js", "panel.js", "drawers.js", "modals.js"} {
		if bad := unsafeUse(read(t, n)); len(bad) > 0 {
			t.Errorf("%s usa API insegura: %v", n, bad)
		}
	}
}

// chart.js necesita atributos SVG (x, y, points...). Única ampliación: exactamente un setAttribute,
// dentro de attr(), con lista fija de nombres y valores numéricos finitos o de caracteres seguros.
// El resto de la lista de APIs peligrosas sigue prohibida.
func TestChartJSSafe(t *testing.T) {
	js := read(t, "chart.js")
	if bad := unsafeUse(strings.Replace(js, "el.setAttribute(name, s)", "", 1)); len(bad) > 0 {
		t.Errorf("chart.js usa API insegura: %v", bad)
	}
	for _, s := range []string{"createElementNS", "role: 'img'", "'aria-label'", "ATTRS.indexOf(name) < 0", "isFinite(v)", "FIXED.test(s)"} {
		if !strings.Contains(js, s) {
			t.Errorf("chart.js no contiene %q", s)
		}
	}
	m := regexp.MustCompile(`ATTRS = \[([^\]]*)\]`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal("ATTRS no encontrado en chart.js")
	}
	for _, n := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
		if l := strings.ToLower(n[1]); l == "href" || l == "src" || l == "style" || strings.HasPrefix(l, "xlink") || strings.HasPrefix(l, "on") {
			t.Errorf("ATTRS contiene el atributo peligroso %q", n[1])
		}
	}
	if !strings.Contains(read(t, "index.html"), `<script src="chart.js"></script>`) {
		t.Error("index.html no carga chart.js")
	}
	// El espacio de nombres SVG es un identificador, no una carga de red.
	if ext := strings.ReplaceAll(js, "http://www.w3.org/2000/svg", ""); strings.Contains(ext, "https:") || strings.Contains(ext, "http:") {
		t.Error("chart.js no debe tener URLs externas")
	}
	css := read(t, "app.css")
	for _, s := range []string{".chart-svg .ref", ".mk.s3", "--c3"} {
		if !strings.Contains(css, s) {
			t.Errorf("app.css sin %q", s)
		}
	}
}

func TestUnsafeDetector(t *testing.T) {
	for _, js := range []string{"x.innerHTML = a", "x.setAttribute('a',b)", "a.href = u", "i.src=u", "u='javascript:1'", "new Function('x')", "b.onclick = f", "eval (x)", "document.write(1)"} {
		if len(unsafeUse(js)) == 0 {
			t.Errorf("no detecta %q", js)
		}
	}
	if len(unsafeUse("el.textContent = a; e.addEventListener('click', f)")) != 0 {
		t.Error("falso positivo")
	}
}

// Cada método de la API que usa app.js corresponde a un método de App.
func TestBindingsExistInApp(t *testing.T) {
	js := read(t, "app.js")
	m := regexp.MustCompile(`NPERF_METHODS = \[([^\]]*)\]`).FindStringSubmatch(js)
	if m == nil {
		t.Fatal("NPERF_METHODS no encontrado")
	}
	typ := reflect.TypeOf(&app.App{})
	names := regexp.MustCompile(`'(\w+)'`).FindAllStringSubmatch(m[1], -1)
	if len(names) != 27 {
		t.Fatalf("se esperaban 27 métodos, hay %d", len(names))
	}
	for _, n := range names {
		goName := strings.ToUpper(n[1][:1]) + n[1][1:]
		if _, ok := typ.MethodByName(goName); !ok {
			t.Errorf("app.App no tiene %s (JS %s)", goName, n[1])
		}
		if !strings.Contains(js+read(t, "history.js")+read(t, "internet.js")+read(t, "sidebar.js")+read(t, "panel.js")+read(t, "drawers.js"), "nperf."+n[1]+"(") && n[1] != "state" {
			t.Errorf("la UI no usa nperf.%s", n[1])
		}
	}
}

// La UI cubre iconos, panel de tarjetas, aviso de ubicación y semáforo, sin recursos externos.
func TestNetworkDetectionUI(t *testing.T) {
	js, dr, css, html := read(t, "app.js"), read(t, "drawers.js"), read(t, "app.css"), read(t, "index.html")
	for _, s := range []string{"NPerfChart.render", "gráfica no disponible: el otro equipo no envía muestras", "diagnosis", "'parcial'"} {
		if !strings.Contains(js, s) {
			t.Errorf("app.js no contiene %q", s)
		}
	}
	for _, s := range []string{"nperf.adapters(true)", "nperf.monitor(true)", "nperf.monitor(false)", "NPerfChart.render", "nperf.openLocationSettings(", "ADAPTERS_MS = 5000", "MONITOR_MS = 1000"} {
		if !strings.Contains(dr, s) {
			t.Errorf("drawers.js no contiene %q", s)
		}
	}
	if !strings.Contains(read(t, "sidebar.js"), "d.ssid") {
		t.Error("sidebar.js no muestra el SSID")
	}
	for _, s := range []string{".ico.eth", ".ico.wifi", ".ico.other", ".ico.unknown", ".dot.green", ".dot.amber", ".dot.red", "prefers-color-scheme:dark"} {
		if !strings.Contains(css, s) {
			t.Errorf("app.css no contiene %q", s)
		}
	}
	if regexp.MustCompile(`(?i)url\((["']?)(https?:|//)`).MatchString(css) {
		t.Error("app.css no debe cargar recursos externos")
	}
	for _, id := range []string{`id="drawer-adapters"`, `id="location-open"`, `id="adapters"`} {
		if !strings.Contains(html, id) {
			t.Errorf("index.html sin %s", id)
		}
	}
}

// history.js: cargado por index.html tras app.js, CSP intacta y cada llamada de nperf.* declarada.
func TestHistoryUI(t *testing.T) {
	h, js, mock := read(t, "index.html"), read(t, "history.js"), read(t, "mock.js")
	if !strings.Contains(h, `<script src="history.js"></script>`) || strings.Index(h, "history.js") < strings.Index(h, "app.js") {
		t.Error("index.html debe cargar history.js después de app.js")
	}
	app := read(t, "app.js")
	m := regexp.MustCompile(`NPERF_METHODS = \[([^\]]*)\]`).FindStringSubmatch(app)
	for _, c := range regexp.MustCompile(`nperf\.(\w+)\(`).FindAllStringSubmatch(js, -1) {
		if !strings.Contains(m[1], "'"+c[1]+"'") {
			t.Errorf("history.js llama a nperf.%s que no está en NPERF_METHODS", c[1])
		}
		if !strings.Contains(mock, c[1]+":") {
			t.Errorf("mock.js no simula %s", c[1])
		}
	}
	for _, s := range []string{"nperf.exportHistory(", "nperf.deleteAllHistory(", "nperf.deleteHistory(", "history_warning", "cancelado", "reportView(", "chartBlock("} {
		if !strings.Contains(js, s) {
			t.Errorf("history.js no contiene %q", s)
		}
	}
	// El campo de ruta nunca se envía: la elige el diálogo nativo.
	if regexp.MustCompile(`\bpath\b`).MatchString(js) {
		t.Error("history.js no debe manejar rutas de entrada")
	}
}

// Rediseño del Historial (spec 008 T16): modales compartidos, sin checkbox ni formulario embebidos.
func TestHistoryRedesign(t *testing.T) {
	js, h := read(t, "history.js"), read(t, "index.html")
	for _, s := range []string{"openExport(", "openConfirmDelete(", "hist-kind-name", "hist-export-one", "Activar en Ajustes"} {
		if !strings.Contains(js+h, s) {
			t.Errorf("falta %q", s)
		}
	}
	for _, id := range []string{`id="hist-enabled"`, `id="hist-export-go"`, `id="hist-confirm"`, `id="hist-format"`} {
		if strings.Contains(h, id) {
			t.Errorf("index.html no debe contener %s", id)
		}
	}
}

// internet.js: aviso de M-Lab idéntico a notice.go, startSpeed solo desde "Aceptar y medir",
// sin URLs externas en el HTML y con los valores exactos de openPrivacy del puente.
func TestInternetUI(t *testing.T) {
	js, html, mock := read(t, "internet.js"), read(t, "index.html"), read(t, "mock.js")
	for _, txt := range []string{ndt7.NoticeTitle, ndt7.NoticeIP, ndt7.NoticeData, ndt7.NoticeLimit, ndt7.Attribution} {
		if !strings.Contains(js, "'"+txt+"'") {
			t.Errorf("internet.js no contiene el texto de notice.go: %q", txt)
		}
	}
	if regexp.MustCompile(`(?i)https?://`).MatchString(html) {
		t.Error("index.html no debe tener URLs externas")
	}
	for _, s := range []string{"nperf.openPrivacy('" + app.PrivacyMLab + "')", "nperf.openPrivacy('" + app.PrivacyNPerf + "')", "nperf.startInternet(", "nperf.setInternetTargets(", "Aceptar y medir"} {
		if !strings.Contains(js+html+read(t, "drawers.js"), s) {
			t.Errorf("falta %q", s)
		}
	}
	if !strings.Contains(html, `<script src="internet.js"></script>`) {
		t.Error("index.html no carga internet.js")
	}
	// Exactamente una llamada a startSpeed, dentro de acceptSpeed, que solo enlaza el botón de aceptar.
	for _, n := range []string{"app.js", "history.js", "internet.js"} {
		src := read(t, n)
		c := strings.Count(src, "nperf.startSpeed(")
		if n == "internet.js" {
			if c != 1 {
				t.Errorf("internet.js debe llamar a startSpeed una sola vez, hay %d", c)
			}
		} else if c != 0 {
			t.Errorf("%s no debe llamar a startSpeed", n)
		}
	}
	m := regexp.MustCompile(`(?s)function acceptSpeed\(\) \{(.*?)\n\}`).FindStringSubmatch(js)
	if m == nil || !strings.Contains(m[1], "nperf.startSpeed(true)") {
		t.Error("startSpeed(true) debe estar dentro de acceptSpeed")
	}
	// acceptSpeed solo se enlaza como onClick de la acción "Aceptar y medir" del modal.
	if strings.Count(js, "acceptSpeed") != 2 || !regexp.MustCompile(`label: 'Aceptar y medir'[^\n]*onClick: function \(\) \{ acceptSpeed\(\); \}`).MatchString(js) {
		t.Error("acceptSpeed solo debe ser el manejador del botón de aceptar")
	}
	if strings.Contains(js, "startSpeed(false") || strings.Contains(mock, "startSpeed(") {
		t.Error("uso inesperado de startSpeed")
	}
}

// El consentimiento de M-Lab usa el modal genérico: foco en Cancelar, enlaces a la izquierda,
// sin diálogo propio en el HTML y Repetir reabre el modal.
func TestInternetConsentModal(t *testing.T) {
	js, html := read(t, "internet.js"), read(t, "index.html")
	if strings.Contains(html, "speed-dialog") || strings.Contains(js, "speed-dialog") {
		t.Error("el consentimiento no debe tener diálogo propio")
	}
	for _, s := range []string{"NPerfModals", "links: [", "buttons.cancel.focus()", "SPEED_LIMIT", "NPerfInternet", "openSpeedConsent"} {
		if !strings.Contains(js, s) {
			t.Errorf("internet.js no contiene %q", s)
		}
	}
	if !strings.Contains(js, "límite es de 40 pruebas al día") {
		t.Error("falta el texto fijo del límite de 40 pruebas al día")
	}
	if regexp.MustCompile(`(?i)innerHTML|insertAdjacentHTML|outerHTML`).MatchString(js) {
		t.Error("internet.js no debe usar HTML dinámico")
	}
}

// Resultados del Panel (spec 008 T10): informes parciales sin error y "—" en lugar de 0.
func TestPanelResultsPartial(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	app := read(t, "app.js")
	core := app[strings.Index(app, "// Núcleo reutilizable"):]
	if err := os.WriteFile(filepath.Join(dir, "core.js"), []byte(core), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "panel.js"), []byte(read(t, "panel.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert'),path=require('path');
function node(tag){return {tag,children:[],textContent:'',className:'',appendChild(c){this.children.push(c);this.lastChild=c;return c;},setAttribute(){},addEventListener(){}};}
const ctx={window:{},document:{createElement:node,createTextNode:(x)=>({text:x}),addEventListener(){},getElementById(){return null;}},JSON,Date,Promise,Object,Array,String,Number,Math,Set,isFinite,setTimeout(){},clearTimeout(){}};
ctx.el=(tag,cls,text)=>{const e=node(tag);e.className=cls||'';if(text!==undefined)e.textContent=String(text);return e;};
vm.createContext(ctx);
for (const f of ['core.js','panel.js']) vm.runInContext(fs.readFileSync(path.join(process.argv[2],f),'utf8'),ctx);
const P=ctx.window.NPerfPanel;
const peer={id:'p1',alias:'Srv',ips:['10.0.0.2']};
const base=()=>({directions:[],self:{alias:'x'},peers:[peer],no_agent:[],selected_id:'p1',direction:'both',can_test:true,can_probe:true,test:{status:'done',report:null,diagnosis:null},probe:{status:'idle'}});
const reports=[{},{tcp:{fwd:{status:'measured',throughput_mbps:5,bytes:1,retransmissions:null}}},{rtt:{status:'measured',min_ms:1,avg_ms:1,max_ms:1},udp:{}},{tcp:{},notes:null},{tcp:{fwd:{status:'unavailable'}},udp:{fwd:{status:'measured',throughput_mbps:1,udp:null}}}];
for (const r of reports) for (const det of [true,false]) {
  ctx.window.NPerfCore.ui.isDetailedMode=det;
  const s=base();s.test.report=r;
  P.buildResult(s,'test');
  const k=P.kpiModels(s);
  assert.strictEqual(k.length,4);
  if(!r.tcp||!r.tcp.rev){const v=k.filter(x=>x.id==='tcp_rev')[0];assert.strictEqual(v.value,'—');assert.strictEqual(v.foot,'No medido');}
  if(!r.rtt){assert.strictEqual(k.filter(x=>x.id==='rtt')[0].value,'—');}
  if(!r.udp||!r.udp.fwd||!r.udp.fwd.udp){assert.strictEqual(k.filter(x=>x.id==='udp')[0].value,'—');}
}
let s=base();s.test.report={tcp:{fwd:{status:'unavailable'}}};
assert.strictEqual(P.kpiModels(s)[0].foot,'No disponible');
// cancelado sin informe ni gráfica, error, probe parcial
s=base();s.test={status:'cancelled'};P.buildResult(s,'test');
s=base();s.test={status:'error',error:'x'};P.buildResult(s,'test');
s=base();s.test={status:'running',phase:'tcp_fwd',chart:{phases:[{phase:'tcp_fwd',available:true,samples:[[0,1,300,null,null,null]],rtt_initial:null,loaded_rtt:null}]}};
assert.strictEqual(P.kpiModels(s)[0].value,'300');assert.strictEqual(P.kpiModels(s)[1].value,'—');
for (const k of ['ping','trace','mtu','ports','internet','speed']){s=base();s.probe={status:'done',kind:k,target_id:'p1',result:{},combined:null};s.test={status:'idle'};assert.strictEqual(P.viewMode(s,false),'probe');P.buildResult(s,'probe');}
// internet y velocidad no dependen del destino seleccionado
s=base();s.selected_id='';s.test={status:'idle'};s.probe={status:'done',kind:'internet',result:{}};assert.strictEqual(P.viewMode(s,false),'probe');
assert(P.headModel(s,'probe').repeat.enabled);
console.log('ok');`
	script := filepath.Join(t.TempDir(), "t.js")
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, dir).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("resultados del Panel: %v\n%s", err, out)
	}
}

// La pila de Escape cierra popover, luego modal, luego drawer; una exportación en curso la bloquea.
func TestShellEscapeStack(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	shell := filepath.Join(dir, "shell.js")
	if err := os.WriteFile(shell, []byte(read(t, "shell.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
const ctx={window:{}};vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const L=ctx.window.NPerfShell.createLayers();const log=[];
L.open('drawer',()=>log.push('drawer'));L.open('modal',()=>log.push('modal'),false);L.open('popover',()=>log.push('popover'));
assert(L.escape());assert(L.escape());assert(L.escape());assert(!L.escape());
assert.deepStrictEqual(log,['popover','modal','drawer']);
L.open('drawer',()=>log.push('d2'));L.open('modal',()=>log.push('m2'),true);
assert(L.escape());assert.deepStrictEqual(log,['popover','modal','drawer']);
L.setBlocking('modal',false);assert(L.escape());assert(L.escape());assert(!L.escape());
assert.deepStrictEqual(log.slice(3),['m2','d2']);
console.log('ok');`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, shell).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("pila de Escape: %v\n%s", err, out)
	}
}

// Núcleo de app.js (spec 008 T6): renderIf, setValueIfIdle, busy y formateadores.
func TestAppCore(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "app.js")
	if err := os.WriteFile(app, []byte(read(t, "app.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
const ctx={window:{},document:{createElement(){return {};},addEventListener(){},getElementById(){return null;},querySelector(){return null;},activeElement:null},JSON,Date,Promise,Object,Array,String,Number,Set,isFinite,setTimeout(){},setInterval(){},clearTimeout(){}};
vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const C=ctx.window.NPerfCore;
let n=0;
assert(C.renderIf('k',{a:1},()=>n++));assert(!C.renderIf('k',{a:1},()=>n++));assert.strictEqual(n,1);
assert(C.renderIf('k',{a:2},()=>n++));assert.strictEqual(n,2);
const inp={value:'x'};ctx.document.activeElement=inp;
assert(!C.setValueIfIdle(inp,'y'));assert.strictEqual(inp.value,'x');
ctx.document.activeElement=null;assert(C.setValueIfIdle(inp,'y'));assert.strictEqual(inp.value,'y');
const F=C.fmt;
for (const v of [null,undefined,{status:'not_measured'},{status:'unavailable'}]) assert.strictEqual(F.mbps(v),'—');
assert.strictEqual(F.metric({status:'not_measured'},'x',F.ms),'—');
assert.strictEqual(F.ms(1.25),'1.3 ms');
const b={disabled:false,textContent:'Ok'};
const p=C.busy(b,Promise.resolve(7),'...');assert(b.disabled&&b.textContent==='...'&&C.ui.pendingActions.has(b));
p.then(v=>{assert.strictEqual(v,7);assert(!b.disabled&&b.textContent==='Ok'&&C.ui.pendingActions.size===0);console.log('ok');});`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, app).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("núcleo app.js: %v\n%s", err, out)
	}
}

var scriptRe = regexp.MustCompile(`<script src="([a-z0-9.]+\.js)"></script>`)

// index.html carga los scripts en el orden de dependencias y todos están embebidos.
func TestIndexScriptsOrder(t *testing.T) {
	h := read(t, "index.html")
	var got []string
	for _, m := range scriptRe.FindAllStringSubmatch(h, -1) {
		got = append(got, m[1])
		read(t, m[1])
	}
	want := []string{"chart.js", "app.js", "shell.js", "sidebar.js", "panel.js", "drawers.js", "modals.js", "internet.js", "history.js"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scripts de index.html: %v, se esperaba %v", got, want)
	}
}

// Cada id que sidebar.js y shell.js piden al DOM existe en index.html.
func TestSidebarIDs(t *testing.T) {
	h := read(t, "index.html")
	re := regexp.MustCompile(`(?:\$|byId)\('([a-z0-9-]+)'\)`)
	ids := map[string]bool{}
	for _, n := range []string{"sidebar.js", "shell.js"} {
		for _, m := range re.FindAllStringSubmatch(read(t, n), -1) {
			ids[m[1]] = true
		}
	}
	for _, id := range []string{"sb-self", "sb-peers", "sb-noagent", "sb-empty", "host-form", "host-open", "hist-badge", "hdr-net", "toasts", "server-pill"} {
		ids[id] = true
	}
	for id := range ids {
		if !strings.Contains(h, `id="`+id+`"`) {
			t.Errorf("index.html no tiene id=%q", id)
		}
	}
	// El aviso de firewall: banner persistente bajo la cabecera, pintado por renderIf con el texto de Go.
	if !strings.Contains(h, `id="firewall"`) || !strings.Contains(read(t, "app.js"), "renderIf('firewall', s.firewall_hint") {
		t.Error("falta el banner persistente de firewall_hint")
	}
	// El sidebar antiguo ya no existe.
	for _, id := range []string{`id="self"`, `id="peers"`, `id="noagent"`, `id="adapters-box"`, `id="history-toggle"`} {
		if strings.Contains(h, id) {
			t.Errorf("index.html conserva %s", id)
		}
	}
}

// Modales (spec 008 T14): foco atrapado, Escape bloqueado durante la exportación, intervalos solo con CSV.
func TestModals(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	shell := write("shell.js", read(t, "shell.js"))
	modals := write("modals.js", read(t, "modals.js"))
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
function el(tag){const e={tagName:tag.toUpperCase(),children:[],textContent:'',className:'',value:'',hidden:false,disabled:false,parentNode:null,ls:{},
 appendChild(c){c.parentNode=this;this.children.push(c);return c;},removeChild(c){this.children=this.children.filter(x=>x!==c);c.parentNode=null;},
 addEventListener(n,f){this.ls[n]=f;},focus(){doc.activeElement=this;},click(){if(!this.disabled&&this.ls.click)this.ls.click({});}};return e;}
const doc={activeElement:null,body:null,createElement:el};doc.body=el('body');
let pending=[];const pushed=[];
const ctx={window:{},document:doc,Promise,Array,String,Object};
vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
ctx.window.NPerfCore={ui:{activeModal:null},pushToast:(m)=>pushed.push(m),busy(b,p,t){b.disabled=true;return Promise.resolve(p).then(v=>{b.disabled=false;return v;},e=>{b.disabled=false;throw e;});}};
vm.runInContext(fs.readFileSync(process.argv[3],'utf8'),ctx);
const M=ctx.window.NPerfModals,S=ctx.window.NPerfShell;
let calls=0,resolve;
const run=()=>{calls++;return new Promise(r=>{resolve=r;});};
const h=M.openExport({ids:['a','b'],run});
assert(h&&M.isOpen()&&!M.openExport({ids:['a'],run}));
const f=h.fields;
assert(!f.intervals.disabled);f.intervals.checked=true;
f.format.value='json';f.format.ls.change();assert(f.intervals.disabled&&!f.intervals.checked);
f.format.value='csv-std';f.format.ls.change();assert(!f.intervals.disabled);
// foco atrapado
const fo=h.focusables();const first=fo[0],last=fo[fo.length-1];
assert.strictEqual(doc.activeElement,first);
let prevented=false;doc.activeElement=last;h.back.ls.keydown({key:'Tab',preventDefault(){prevented=true;}});
assert(prevented&&doc.activeElement===first);
prevented=false;h.back.ls.keydown({key:'Tab',shiftKey:true,preventDefault(){prevented=true;}});
assert(prevented&&doc.activeElement===last);
// exportar: segundo clic no llama de nuevo; Escape bloqueado
h.buttons.go.click();h.buttons.go.click();
setTimeout(()=>{
 assert.strictEqual(calls,1);
 assert(S.layers.escape()&&M.isOpen());
 resolve(['C:\\x.csv']);
 setTimeout(()=>{
 assert(!M.isOpen()&&pushed[0].startsWith('Exportado en'));
 // Escape cierra el modal libre
 const c=M.openConfirmDelete({scope:'all',count:8,run:()=>Promise.resolve(8)});
 assert(M.isOpen()&&c.el.children[1].textContent==='¿Borrar todo el historial (8 registros)? Esta acción no se puede deshacer.');
 assert(S.layers.escape()&&!M.isOpen());
 assert.strictEqual(M.confirmText('selected',2),'¿Borrar los 2 registros seleccionados? Esta acción no se puede deshacer.');
 assert(M.openConfirmDelete({scope:'selected',count:0,run(){}})===null);
 console.log('ok');
 },20);
},20);`
	script := write("t.js", js)
	out, err := exec.Command(node, script, shell, modals).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("modales: %v\n%s", err, out)
	}
}

// drawers.js (spec 008 T12): sin llamadas periodicas a adapters/monitor con el drawer cerrado; 802.11ax -> "Wi-Fi 6".
func TestDrawersPolling(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "drawers.js")
	if err := os.WriteFile(src, []byte(read(t, "drawers.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
const ctx={window:{},document:{addEventListener(){}},Promise,isFinite,String,Number};
vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const D=ctx.window.NPerfDrawers;
assert.strictEqual(D.wifiStandard('802.11ax (WiFi 6)'),'Wi-Fi 6');
assert.strictEqual(D.wifiStandard('802.11ax (WiFi 6E)'),'Wi-Fi 6E');
assert.strictEqual(D.wifiStandard('802.11ac'),'Wi-Fi 5');
assert.strictEqual(D.wifiStandard('802.11n (WiFi 4)'),'Wi-Fi 4');
assert.strictEqual(D.wifiStandard('802.11be'),'Wi-Fi 7');
assert.strictEqual(D.wifiStandard('raro'),'raro');
const calls=[];let open=false,timers=[];
const nperf={adapters(d){calls.push('a'+d);return Promise.resolve({});},monitor(on){calls.push('m'+on);return Promise.resolve({});}};
const P=D.createPoller({nperf,active:()=>open,setInterval:(f)=>{timers.push(f);return timers.length;},clearInterval:(t)=>{timers[t-1]=null;},
 onAdapters(){},onAdaptersError(){},onMonitor(){},onMonitorError(){}});
const tick=()=>new Promise(r=>setTimeout(r,5));
(async()=>{
 P.load();await tick();assert.deepStrictEqual(calls,['atrue']);      // carga inicial
 P.sync();await tick();assert.deepStrictEqual(calls,['atrue']);      // cerrado: nada
 assert.strictEqual(timers.length,0);
 open=true;P.sync();await tick();assert.deepStrictEqual(calls,['atrue','atrue','mtrue']);
 assert.strictEqual(timers.length,2);timers.forEach(f=>f());await tick();
 assert.deepStrictEqual(calls.slice(3),['atrue','mtrue']);
 open=false;P.sync();await tick();assert.strictEqual(calls[calls.length-1],'mfalse');
 assert(timers.every(f=>f===null));assert(!P.isRunning());
 const n=calls.length;P.sync();await tick();assert.strictEqual(calls.length,n);
 // monitor(true) en vuelo al cerrar: al resolver se vuelve a apagar.
 let res;calls.length=0;open=true;
 nperf.monitor=(on)=>{calls.push('m'+on);return on?new Promise(r=>{res=r;}):Promise.resolve({});};
 const Q=D.createPoller({nperf,active:()=>open,setInterval:()=>1,clearInterval(){},onAdapters(){},onAdaptersError(){},onMonitor(){},onMonitorError(){}});
 Q.sync();await tick();open=false;Q.sync();await tick();
 assert.deepStrictEqual(calls.filter(c=>c[0]==='m'),['mtrue','mfalse']);
 res({});await tick();
 assert.deepStrictEqual(calls.filter(c=>c[0]==='m'),['mtrue','mfalse','mfalse']);
 console.log('ok');
})().catch(e=>{console.error(e);process.exit(1);});`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, src).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("drawers.js: %v\n%s", err, out)
	}
}

// Panel (spec 008 T9): fases visibles por sentido y motivo de bloqueo una sola vez.
func TestPanelBarAndStrip(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	panel := filepath.Join(dir, "panel.js")
	if err := os.WriteFile(panel, []byte(read(t, "panel.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
const ctx={window:{}};vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const P=ctx.window.NPerfPanel;
const J=(x)=>JSON.stringify(x);
assert.strictEqual(J(P.phasesFor('fwd')),J(['rtt','tcp_fwd','udp_fwd']));
assert.strictEqual(J(P.phasesFor('rev')),J(['rtt','tcp_rev','udp_rev']));
assert.strictEqual(P.phasesFor('both').length,5);
const peer={id:'p1',alias:'Srv'};
const base=()=>({peers:[peer],no_agent:[],selected_id:'p1',direction:'fwd',can_test:true,test_blocked_reason:'',can_probe:true,probe_blocked_reason:'',test:{status:'idle'},probe:{status:'idle'}});
let s=base();s.test={running:true,status:'running',phase:'tcp_fwd'};
let m=P.stripModel(s);assert.strictEqual(m.chips.map(c=>c.text+':'+c.state).join(),'RTT:done,TCP Ida:cur,UDP Ida:');
s=base();s.test={running:true,status:'queued',queue_position:2};assert(P.stripModel(s).text.includes('#2')&&P.stripModel(s).text.includes('Srv'));
assert.strictEqual(P.stripModel(base()),null);
// Motivo de bloqueo una sola vez: nunca en los dos sitios, y siempre el texto de Go.
const cases=[
 {can_test:false,test_blocked_reason:'T',can_probe:false,probe_blocked_reason:'P'},
 {can_test:false,test_blocked_reason:'T',can_probe:true},
 {can_test:true,can_probe:false,probe_blocked_reason:'P'},
 {can_test:false,test_blocked_reason:'T',can_probe:false,probe_blocked_reason:'P',test:{running:true,status:'running'}},
 {can_test:false,test_blocked_reason:'T',can_probe:false,probe_blocked_reason:'P',probe:{running:true,status:'running',kind:'ping'}}];
for (const c of cases){ s=Object.assign(base(),c); const b=P.barModel(s);
  assert(!(b.testText&&b.probeText),J(c)); assert(['','T','P'].includes(b.testText)&&['','P'].includes(b.probeText)); }
s=Object.assign(base(),cases[0]);assert.strictEqual(P.barModel(s).testText,'T');assert.strictEqual(P.barModel(s).probeText,'');
s=Object.assign(base(),cases[3]);assert.strictEqual(P.barModel(s).mode,'cancel');assert.strictEqual(P.barModel(s).probeText,'P');
s=Object.assign(base(),{test:{status:'cancelling'},can_test:false,test_blocked_reason:'C'});assert.strictEqual(P.barModel(s).mode,'cancelling');
console.log('ok');`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, panel).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("panel.js: %v\n%s", err, out)
	}
}

// drawers.js Ajustes (spec 008 T13): la clave no se escribe en ningun nodo; el reinicio solo con alias/clave/subredes.
func TestSettingsDrawer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "drawers.js")
	if err := os.WriteFile(src, []byte(read(t, "drawers.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "t.js")
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert');
function node(tag){const n={tag,children:[],listeners:{},className:'',textContent:'',value:'',hidden:false,disabled:false,attrs:{},
 appendChild(c){n.children.push(c);return c;},get firstChild(){return n.children[0]||null;},removeChild(c){n.children.splice(n.children.indexOf(c),1);},
 addEventListener(e,f){n.listeners[e]=f;},setAttribute(k,v){n.attrs[k]=v;},click(){return n.listeners.click&&n.listeners.click();}};return n;}
function all(n,out){out=out||[];out.push(n);n.children.forEach(c=>all(c,out));return out;}
const doc={createElement:node,addEventListener(){},activeElement:null};
const pending=new Set();
const core={ui:{pendingActions:pending},setValueIfIdle(i,v){i.value=v;},renderIf(k,v,f){f(v);},
 busy(b,p,t){if(b){pending.add(b);b.disabled=true;}return Promise.resolve(p).then(v=>{if(b){pending.delete(b);b.disabled=false;}return v;},e=>{if(b){pending.delete(b);b.disabled=false;}throw e;});}};
const ctx={window:{},document:doc,Promise,isFinite,String,Number,Set,JSON};
vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const calls=[];let fail=false;
const nperf=new Proxy({},{get:(_,m)=>(...a)=>{calls.push(m);return fail?Promise.reject(new Error('boom')):Promise.resolve();}});
const S=ctx.window.NPerfDrawers.createSettings(core,nperf);
const els=all(S.root);
const byAria=l=>els.find(e=>e.ariaLabel===l);
const btnIn=i=>{const row=els.find(e=>e.className==='hrow'&&e.children.includes(i));return row.children[1];};
const tick=()=>new Promise(r=>setTimeout(r,5));
(async()=>{
 S.render({alias:'PC',has_key:true,history_enabled:true,internet_targets:['1.1.1.1'],extra_subnets:['10.0.0.0/24']});
 assert(els.some(e=>e.textContent==='Configurada'));
 const alias=byAria('Alias'),key=byAria('Nueva clave'),sub=byAria('Subred'),tin=byAria('Nuevo destino de latencia');
 assert.strictEqual(btnIn(alias).disabled,true);           // sin cambios
 alias.value='Otro';alias.listeners.input();assert.strictEqual(btnIn(alias).disabled,false);
 btnIn(alias).click();await tick();assert.strictEqual(S.banner.textContent,'Guardado · servidor reiniciado');
 const SECRET='s3cr3t-clave';key.value=SECRET;key.listeners.input();btnIn(key).click();
 assert.strictEqual(key.value,'');
 await tick();assert.strictEqual(S.banner.textContent,'Guardado · servidor reiniciado');
 assert(!all(S.root).some(e=>String(e.textContent).includes(SECRET)||String(e.value).includes(SECRET)));
 assert.deepStrictEqual(calls,['setAlias','setKey']);
 // destinos: sin texto de reinicio; IPv4 invalida no llama
 tin.value='999.1.1.1';btnIn(tin).click();await tick();assert.deepStrictEqual(calls,['setAlias','setKey']);
 tin.value='9.9.9.9';btnIn(tin).click();await tick();
 assert.strictEqual(S.banner.textContent,'Guardado');assert.strictEqual(calls[2],'setInternetTargets');
 const rest=els.find(e=>e.textContent==='Restaurar valores por defecto');rest.click();await tick();
 assert.strictEqual(S.banner.textContent,'Guardado');
 S.render({alias:'PC',has_key:false,history_enabled:false,internet_targets:['a','b','c','d'],extra_subnets:[]});
 assert.strictEqual(btnIn(tin).disabled,true);              // tope 4
 const h=els.find(e=>e.type==='checkbox');h.checked=true;h.listeners.change();await tick();
 assert.strictEqual(S.banner.textContent,'Guardado');assert.strictEqual(calls[calls.length-1],'setHistoryEnabled');
 sub.value='192.168.2.0/24';btnIn(sub).click();await tick();
 assert.strictEqual(S.banner.textContent,'Guardado · servidor reiniciado');assert.strictEqual(calls[calls.length-1],'addSubnet');
 fail=true;sub.value='192.168.3.0/24';btnIn(sub).click();await tick();
 assert(S.banner.textContent.startsWith('No se pudo guardar'));assert.strictEqual(btnIn(sub).disabled,false);
 console.log('ok');
})().catch(e=>{console.error(e);process.exit(1);});`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, src).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("ajustes: %v\n%s", err, out)
	}
}

// Panel (spec 008 T11): el resumen no filtra MAC, BSSID ni clave; Exportar sin id queda deshabilitado con motivo.
func TestPanelSummaryAndExport(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	app := read(t, "app.js")
	core := app[strings.Index(app, "// Núcleo reutilizable"):]
	if err := os.WriteFile(filepath.Join(dir, "core.js"), []byte(core), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "panel.js"), []byte(read(t, "panel.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	js := `const vm=require('vm'),fs=require('fs'),assert=require('assert'),path=require('path');
function node(tag){return {tag,children:[],textContent:'',className:'',style:{},select(){},appendChild(c){this.children.push(c);this.lastChild=c;return c;},setAttribute(){},addEventListener(){}};}
const ctx={window:{},document:{createElement:node,createTextNode:(x)=>({text:x}),addEventListener(){},getElementById(){return null;}},JSON,Date,Promise,Object,Array,String,Number,Math,Set,isFinite,setTimeout(){},clearTimeout(){}};
ctx.el=(tag,cls,text)=>{const e=node(tag);e.className=cls||'';if(text!==undefined)e.textContent=String(text);return e;};
vm.createContext(ctx);
for (const f of ['core.js','panel.js']) vm.runInContext(fs.readFileSync(path.join(process.argv[2],f),'utf8'),ctx);
const P=ctx.window.NPerfPanel;
const link={kind:'wifi',tx_bps:866000000,rx_bps:866000000,ssid:'Casa',bssid:'aa:bb:cc:dd:ee:ff',mac:'11:22:33:44:55:66'};
const s={directions:[],self:{alias:'x',mac:'11:22:33:44:55:66'},peers:[{id:'p1',alias:'Srv',ips:['10.0.0.2'],mac:'11:22:33:44:55:66',bssid:'aa:bb:cc:dd:ee:ff'}],no_agent:[],selected_id:'p1',direction:'both',can_test:true,can_probe:true,key:'SECRETO-123',history_enabled:true,
 test:{status:'done',history_id:'h1',key:'SECRETO-123',diagnosis:{local:link,remote:link,fwd:{color:'green',percent:90,text:'Sin limitación'}},report:{key:'SECRETO-123',notes:['nota'],tcp:{fwd:{status:'measured',throughput_mbps:500,bytes:1e8,retransmissions:null}},rtt:{status:'measured',min_ms:1,avg_ms:2,max_ms:3}}},probe:{status:'idle'}};
const txt=P.buildSummary(s,'test');
assert(txt.includes('500')&&txt.includes('Srv')&&txt.includes('Sin limitación'),txt);
assert(!/aa:bb|11:22|SECRETO|bssid/i.test(txt),txt);
const p=JSON.parse(JSON.stringify(s));p.test={status:'idle'};p.probe={status:'done',kind:'ping',target_id:'p1',history_id:'h2',result:{sent:4,received:4,loss_pct:0,rtt_status:'ok',min_ms:1,avg_ms:2,max_ms:3,replies:[{from:'aa:bb:cc:dd:ee:ff'}]},combined:{healthy:true,messages:['Todo bien']}};
const ptxt=P.buildSummary(p,'probe');assert(ptxt.includes('Todo bien')&&!/aa:bb|SECRETO/.test(ptxt),ptxt);
// Exportar: con id habilitado; sin id o historial desactivado, deshabilitado con el motivo
let m=P.exportModel(s,'test');assert(m.enabled&&m.id==='h1'&&m.why==='');
m=P.exportModel(Object.assign({},s,{test:{status:'done'}}),'test');assert(!m.enabled&&m.why==='No se guardó en el historial');
m=P.exportModel(Object.assign({},s,{history_enabled:false}),'test');assert(!m.enabled&&m.why==='Historial desactivado');
m=P.exportModel(p,'probe');assert(m.enabled&&m.id==='h2');
// respaldo: sin Clipboard API se usa execCommand
let copied=false;ctx.navigator=undefined;
ctx.document.body={appendChild(){},removeChild(){}};
ctx.document.execCommand=(c)=>{copied=c==='copy';return true;};
P.copyText('x').then(()=>{assert(copied);console.log('ok');});`
	script := filepath.Join(t.TempDir(), "t.js")
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, dir).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("resumen y exportar del Panel: %v\n%s", err, out)
	}
}

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
	for _, n := range []string{"index.html", "app.css", "app.js", "mock.js", "chart.js", "history.js", "internet.js"} {
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
	for _, n := range []string{"app.js", "mock.js", "history.js", "internet.js"} {
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
		if !strings.Contains(js+read(t, "history.js")+read(t, "internet.js"), "nperf."+n[1]+"(") && n[1] != "state" {
			t.Errorf("la UI no usa nperf.%s", n[1])
		}
	}
}

// La UI cubre iconos, panel de tarjetas, aviso de ubicación y semáforo, sin recursos externos.
func TestNetworkDetectionUI(t *testing.T) {
	js, css, html := read(t, "app.js"), read(t, "app.css"), read(t, "index.html")
	for _, s := range []string{"nperf.adapters(", "nperf.monitor(true)", "nperf.monitor(false)", "NPerfChart.render", "gráfica no disponible: el otro equipo no envía muestras", "nperf.openLocationSettings(", "setInterval(refreshAdapters, 5000)", "$('adapters-box').open", "d.ssid", "diagnosis", "'parcial'"} {
		if !strings.Contains(js, s) {
			t.Errorf("app.js no contiene %q", s)
		}
	}
	for _, s := range []string{".ico.eth", ".ico.wifi", ".ico.other", ".ico.unknown", ".dot.green", ".dot.amber", ".dot.red", "prefers-color-scheme:dark"} {
		if !strings.Contains(css, s) {
			t.Errorf("app.css no contiene %q", s)
		}
	}
	if regexp.MustCompile(`(?i)url\((["']?)(https?:|//)`).MatchString(css) {
		t.Error("app.css no debe cargar recursos externos")
	}
	for _, id := range []string{`id="adapters-box"`, `id="location-open"`, `id="adapters"`} {
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
	for _, s := range []string{"nperf.exportHistory(", "nperf.deleteAllHistory(", "nperf.deleteHistory(", "nperf.setHistoryEnabled(", "history_warning", "cancelado", "reportView(", "chartBlock("} {
		if !strings.Contains(js, s) {
			t.Errorf("history.js no contiene %q", s)
		}
	}
	// El campo de ruta nunca se envía: la elige el diálogo nativo.
	if regexp.MustCompile(`\bpath\b`).MatchString(js) {
		t.Error("history.js no debe manejar rutas de entrada")
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
		if !strings.Contains(js+html, s) {
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
	if strings.Count(js, "acceptSpeed") != 2 || !strings.Contains(js, "$('speed-accept').addEventListener('click', acceptSpeed)") {
		t.Error("acceptSpeed solo debe ser el manejador del botón de aceptar")
	}
	if strings.Contains(js, "startSpeed(false") || strings.Contains(mock, "startSpeed(") {
		t.Error("uso inesperado de startSpeed")
	}
}

// reportView no debe fallar con resultados parciales (TEST! cancelado sin rtt, tcp o udp).
func TestReportViewPartial(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node no disponible")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "t.js")
	app := filepath.Join(dir, "app.js")
	if err := os.WriteFile(app, []byte(read(t, "app.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	js := `const vm=require('vm'),fs=require('fs');
function node(tag){return {tag,children:[],textContent:'',className:'',appendChild(c){this.children.push(c);return c;}};}
const ctx={window:{},document:{createElement:node,addEventListener(){},getElementById(){return null;},querySelector(){return null;}},JSON,Date,Promise,Object,Array,String,Number,setTimeout(){},setInterval(){},clearTimeout(){}};
vm.createContext(ctx);vm.runInContext(fs.readFileSync(process.argv[2],'utf8'),ctx);
const st={directions:[{value:'fwd',label:'a'},{value:'rev',label:'b'}],self:{alias:'x'},peers:[],no_agent:[],selected_id:'',test:{report:null,diagnosis:null}};
for (const r of [{}, {tcp:{fwd:{status:'measured',throughput_mbps:5,bytes:1,retransmissions:null}}}, {rtt:{status:'measured',min_ms:1,avg_ms:1,max_ms:1}, udp:{}}, {tcp:{}, notes:null}]) {
  st.test.report=r;
  for (const d of [true,false]) { ctx.__r=r; ctx.__s=st; ctx.__d=d; vm.runInContext('reportView(__s, __d)',ctx); }
}
console.log('ok');`
	if err := os.WriteFile(script, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, app).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("reportView falla con resultados parciales: %v\n%s", err, out)
	}
}

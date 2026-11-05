# 002 - Plan

## Enfoque
- **Capas**: la ventana es una capa fina. Todo lo demás (descubrimiento, backend de la app, config, instancia única, server activo) es Go portable y se prueba en Linux sin WebView2. Lo específico de Windows va en `_windows.go` con alternativa `_other.go`.
- **Puente WebView2**: una sola dependencia Go pura, elegida en T1. Candidatos a revisar: `github.com/jchv/go-webview2` y su fork `github.com/wailsapp/go-webview2`. Descartados de entrada: `github.com/webview/webview_go` y similares que usan cgo. Criterios (todos obligatorios):
  1. Licencia compatible con distribuir un binario cerrado (MIT, BSD, Apache-2.0 o similar).
  2. Mantenimiento: commits o releases en los últimos 12 meses, o issues atendidos; se anota la fecha verificada.
  3. Sin cgo: `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` compila un ejemplo mínimo.
  4. Un solo `.exe`: no requiere `WebView2Loader.dll` aparte (o lo embebe/reimplementa).
  5. Dependencias transitivas: solo la propia librería y `golang.org/x/sys` (ya en el módulo); cualquier otra se informa.
  6. Pasa el CI de windows-latest (`go vet`, `go build`).
  Si ninguno cumple: parar y consultar al usuario (riesgo de la spec). Las versiones las verifica el implementer con `go list -m -versions`; no se fijan aquí.
- **Dos binarios**: `cmd/nperf` sin cambios; `cmd/nperf-gui` se compila con `-ldflags -H=windowsgui`. Un test cruzado (`GOOS=windows`) lee la cabecera PE con `debug/pe` y comprueba Subsystem 3 (consola) y 2 (GUI).
- **Anuncio**: JSON pequeño con firma `NPDISC1`, campos `alias`, `ip`, `port`, `state`, `queue`; máx. 512 bytes; se descarta lo inválido. Emisor cada 3 s por interfaz IPv4 activa (a su broadcast), con la lista de interfaces y el envío inyectables. El receptor deduplica por un id de instancia aleatorio (no es id de sesión ni dato privado) y marca "inalcanzable" a los 10 s con reloj inyectable. Una consulta unicast (`NPDISC1 ?`) en 5301 se responde con el anuncio.
- **Estado del server**: `server.Server` expone `Status()` (estado y cola), lo mismo que ya devuelve `nperf status`.
- **Backend** (`internal/app`): struct `App` con los métodos que llama la UI (lista, selección, ajustes, TEST!, Cancelar, estado). La UI consulta `State()` cada ~500 ms (sondeo, sin eventos push): el contrato queda en Go y se prueba sin ventana.
- **Progreso por fase**: se agrega `Options.OnPhase` opcional a `internal/client`; la CLI no lo usa (sin cambios de comportamiento).
- **Tema**: claro/oscuro con `prefers-color-scheme` en CSS (WebView2 sigue a Windows); sin código Go.
- **Bandeja, mutex y mensajes nativos**: Win32 vía `golang.org/x/sys/windows` (`Shell_NotifyIconW`, `CreateMutexW`, `MessageBoxW`).

## Paquetes y archivos
- `internal/discovery`: `announce.go` (formato), `ifaces.go` (interfaces IPv4 y broadcast), `beacon.go` (emisor), `listener.go` (receptor, tabla, caducidad), `query.go` (CIDR ≤ /24 y unicast).
- `internal/server`: `Status()` público.
- `internal/client`: `Options.OnPhase`.
- `internal/config`: alias, duración, puerto de descubrimiento, subredes extra; `Save` atómico que conserva la clave.
- `internal/app`: `app.go` (API), `test.go` (TEST!/Cancelar), `runtime.go` (server + emisor + receptor), `instance_windows.go` / `instance_other.go`.
- `internal/ui`: `embed.go` + `web/index.html`, `web/app.css`, `web/app.js`.
- `internal/gui` (solo Windows): `window_windows.go`, `tray_windows.go`, `webview_check_windows.go`, `msgbox_windows.go`.
- `cmd/nperf-gui`: `main_windows.go`, `main_other.go` (mensaje "solo Windows"), `pe_test.go`.
- `.github/workflows/ci.yml`, `scripts/build-windows.{sh,ps1}`, `docs/validacion-gui.md`, `docs/decisiones.md` (dependencia elegida).

## Riesgos
- **Dependencia del puente**: opciones escasas y mantenimiento irregular; T1 puede terminar en consulta al usuario y bloquear T15–T17.
- **El CI de Windows no abre ventanas ni prueba la bandeja**: ventana, bandeja, tema, mensaje sin WebView2 y firewall solo se validan a mano (T18, criterios 14–19). Por eso la ventana debe quedar lo más fina posible.
- **Broadcast en CI/contenedores**: puede no estar permitido; los tests usan loopback y envío inyectado, no broadcast real.
- **`-H=windowsgui` olvidado**: un `go build` normal genera un GUI con consola; lo detecta el test PE y el CI.
- **Sondeo de la UI**: 500 ms basta para progreso por fase; si se nota lento, se ajusta sin cambiar el contrato.
- **Runners como administrador**: "sin admin" solo se verifica a mano (criterio 19).
- **Errores de arranque sin consola**: todo error previo a la ventana sale por `MessageBoxW`.

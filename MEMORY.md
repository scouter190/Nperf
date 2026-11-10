# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 5 - Implementando spec 003 (detección de red y cuello de botella).** Specs 001 y 002 completas en código, a falta de validaciones manuales del usuario (docs/validacion-iperf3.md, docs/validacion-gui.md).

## Spec activa
`specs/003-deteccion-red/` (aprobada; hechas T1-T12 de T1-T16; decisiones §18). Spec 002: código completo, T16/T17 pendientes de prueba a mano. Spec 001: cerrada.

## Estado del código (spec 001, CLI)
- Módulo `nperf` (Go 1.27.3). Repo: https://github.com/scouter190/Nperf (rama `main`). Dependencias: jchv/go-webview2 + go-winloader (pseudo-versión fija), golang.org/x/sys v0.49.0; govulncheck sin hallazgos.
- `cmd/nperf` (`server`, `client <host>`, `status <host>`; `-port` 5300, `-duration` 3-300 s validado antes de conectar, `-proto tcp|udp|both` (default both), `-udp-rate`, `-direction fwd|rev|both` (default both), `-streams` 1-64 (default 4), `-key`, `-json`), `internal/{proto,meter,server,client,auth,config,version,hrtime,tcpinfo,report,testutil}`.
- CLI: ayudas en español; códigos 0 ok, 1 error de medición/conexión/auth, 2 error de uso; nota si UDP enviado < 90 % del objetivo. Clave: `-key` o `config.json` (os.UserConfigDir()/NPerf); el flag manda.
- `-json`: stdout solo JSON v1 (`format_version`, unidades en nombres de campo, null + `status` para no medido/no disponible); errores como `{"error"}` con el mismo código. `client.Result{Fwd,Rev,UDPFwd,UDPRev,RTT,Notes}`; `client.Run` usa fwd por defecto (pasar Direction explícito).
- Protocolo: handshake HMAC-SHA256 mutuo (la clave nunca viaja, 10 s) -> `turn` -> `queue{position}`* -> `ready` -> ping* -> fases params/start/result con el turno retenido. Cliente abre siempre las conexiones; en rev mide el cliente.
- Cola: FIFO, 1 en curso + 5 en espera, el 6.º "cola llena"; turno liberado en cuanto el cliente se va (watchControl en todas las esperas). Keepalive TCP 15 s.
- `-proto both`: RTT -> TCP fwd/rev -> UDP fwd/rev; UDP = 90 % del TCP del mismo sentido, tope 10 Gbps (`server.MaxUDPRate`, rechazado al recibir params); 100 Mbps sin TCP previo. Un fallo aborta todo. TCP multiflujo: suma en ventana común.
- Retransmisiones (`internal/tcpinfo`): exactas Linux, estimadas Windows ("~N"). Reloj `internal/hrtime` (QPC). Dial con timeout 5 s.
- `nperf status`: pública sin clave, solo state y queue, antes del handshake, cierra tras responder.
- Tests: `-race`, loopback TCP/UDP, cola, errores; e2e con el binario real (`cmd/nperf/e2e_test.go`, -short lo salta). CI: windows-latest vet/test (-p 1 -count=3)/build de nperf.exe y nperf-gui.exe + ubuntu `-race`.

## Estado del código (spec 002)
- Dos binarios: `cmd/nperf` (consola) y `cmd/nperf-gui` (ventana WebView2; test de cabecera PE). `internal/gui`: UI servida por HTTP en 127.0.0.1 (puerto y token aleatorios, CSP también como cabecera); puente `nperfCall(secreto, nombre, args)` con lista blanca, tipos y secreto por sesión (go-webview2 no permite cancelar navegación); reinicio del runtime al guardar; instancia única y errores con MessageBox. Bandeja (`tray.go` controlador portable + `tray_windows.go` Win32 en el hilo de la UI): cerrar oculta, menú Abrir/Salir, doble clic abre, TaskbarCreated reañade el icono; Salir → Terminate → svc.Stop. WebView2 (`webview_check*.go`): registro EdgeUpdate solo orientativo (Installed/Missing/Unknown), no aborta; si falla crear la ventana y consta Missing → MessageBox con oferta de descarga (URL fija); errores de arranque en español. `internal/netutil`: puertos exclusivos en Windows (SO_EXCLUSIVEADDRUSE).
- `internal/discovery`: anuncio `NPDISC1` ≤ 512 B, solo IPv4 unicast, alias sin caracteres de control; beacon por interfaz cada 3 s (log limitado); consulta unicast `NPDISC1 ?` rellenada a 128 B (anti-amplificación), 1 respuesta/IP/s, responde con la IP de la interfaz de la subred de quien pregunta; subredes extra IP/CIDR ≥ /24; tabla (dedup por id, inalcanzable a 10 s, tope 256 con desalojo del inalcanzable más antiguo). `server.Status()` alimenta beacon y `nperf status`.
- `internal/config`: alias, duración, puerto de descubrimiento 5301, subredes; guardado atómico; campos desconocidos se conservan; `Load` (CLI, solo clave) y `LoadFull` (GUI).
- `internal/app`: instancia única (mutex Local\ en Windows, flock en otros); `App` (lista con "Este equipo" aparte, destino alcanzable, sentido/duración, alias, subredes, sugerencia de firewall a 15 s, `State()` JSON con `can_test`/`config_error`, clave nunca expuesta); `StartTest`/`CancelTest` no bloqueante ("Cancelando…") + `WaitTest`; `Runtime` arranca server + beacon + listener + consultas a subredes (la config se lee solo en Start: reiniciar al guardar). `client.Options.OnPhase`.

## Estado del código (spec 003)
- `internal/netinfo`: Classify/Kind eth/wifi/other, FormatBps, ValidSSID (≤32 B, sin control), BandChannel/Standard/WLANReason; `Source` (Adapters, WiFi(guid): T5 rellena GUID desde AdapterName, T6 consulta por GUID), `Inventory` (error WLAN no fatal), `Cached` (errores no cacheados), `SummaryForLocalIP` → proto.Link. Anuncio: claves opcionales link/tx/rx/ssid validadas, alias truncado (nunca el SSID). `NewSystemSource(wifi)` (GetAdaptersAddresses, velocidad ^0→0), `queryWiFi(guid)` (wlanapi LazyDLL, BSS por BSSID con SecurityEnabled y reintento), `BuildView` (JSON v1). Beacon `LinkFor`; T13: aplicar `.WithLink` al anuncio de `Listener.Self` (respuestas a consultas). CLI `nperf adapters [-json]` con `netinfo.System()`. App: `Deps.Net` (InventorySource; T13 cablea netinfo.Cached(netinfo.System(),…)), `Deps.OpenURL` (T13: ShellExecuteW), Device.Link/LinkSpeed/SSID, `App.Adapters`, `OpenLocationSettings` (URL fija). `internal/diag.Evaluate` (cable 50/80, WiFi 30/50, nominal mín(tx emisor, rx receptor), parcial). `proto.Link` opcional en turn/ready (ClientTurn conserva firma → ClientTurnLink), `Link.Sanitize()` al recibir; sin BSSID/MAC.

## Decisiones cerradas
Puerto 5300 · descubrimiento 5301/UDP · UDP 100 Mbps sin TCP previo · cola máx. 5 · ±10 % vs iperf3 · CLI: comandos y flags en inglés, mensajes en español · interfaz WebView2 (no Walk) · origen siempre "Este equipo".

## Notas
- Subidas: regla en AGENTS.md (wip/actual → CI Windows verde → main, sin pedir permiso; el usuario revisa por tarea).
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.
- UI (`internal/ui`, T14): HTML/CSS/JS plano embebido, CSP estricta, todo dato de la red como texto (test que prohíbe innerHTML y similares), puente `window.nperfCall(nombre, ...args)` → Promise; mock.js solo para previsualizar.

## Próximo paso
Spec 003: T13→T14→T15→T16. Usuario: validacion-gui.md y validacion-iperf3.md.

## Pendientes
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"), no reproducido; si reaparece, analizar ese mensaje.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 7 - Implementando spec 005 (gráficas en vivo).** Specs 001-004 completas en código; validaciones manuales pendientes (iperf3, gui, red, sin-agente). Spec 003: banda/canal WiFi y virtuales validados por el usuario en Windows 11.

## Spec activa
`specs/005-graficas-en-vivo/` (aprobada; hechas T1, T2, T5, T11, T16 de T1-T18; decisiones §20; 17 criterios). Spec 004: completa en código (§19). Spec 003: código completo. Spec 002: T16/T17 pendientes de prueba a mano. Spec 001: cerrada.

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
- `internal/netinfo`: Classify/Kind eth/wifi/other, FormatBps, ValidSSID (≤32 B, sin control), BandChannel/Standard/WLANReason; `Source` (Adapters, WiFi(guid): T5 rellena GUID desde AdapterName, T6 consulta por GUID), `Inventory` (error WLAN no fatal), `Cached` (errores no cacheados), `SummaryForLocalIP` → proto.Link. Anuncio: claves opcionales link/tx/rx/ssid validadas, alias truncado (nunca el SSID). `NewSystemSource(wifi)` (GetAdaptersAddresses, velocidad ^0→0), `queryWiFi(guid)` (wlanapi LazyDLL, BSS por BSSID con SecurityEnabled y reintento), `BuildView` (JSON v1). Beacon `LinkFor`; T13: aplicar `.WithLink` al anuncio de `Listener.Self` (respuestas a consultas). CLI `nperf adapters [-json]` con `netinfo.System()`. App: `Deps.Net` (InventorySource; T13 cablea netinfo.Cached(netinfo.System(),…)), `Deps.OpenURL` (= `gui.OpenURL`, ShellExecuteW en open_windows.go); puente: `adapters(bool)`, `openLocationSettings()`, Device.Link/LinkSpeed/SSID, `App.Adapters`, `OpenLocationSettings` (URL fija). T13: `TestState.Diagnosis` por sentido (fwd cliente→servidor), runtime cablea LinkProvider/LinkFor/Self; `Cache` sin mutex durante la consulta (un refresco en vuelo, dato viejo inmediato, 1.ª llamada tope 2 s). Correcciones tras validación real (T17-T19): `Adapter.Virtual` (Wi-Fi Direct, vEthernet/Hyper-V, Tailscale, WireGuard, TAP, VirtualBox, VMware, loopback → KindOther, fuera de ForLocalIP/resumen/semáforo; vista "virtual (N nominal)" al final); canal WiFi por WlanQueryInterface opcode 8 + `BandFromChannel` si falla el BSS. `internal/diag.Evaluate` (cable 50/80, WiFi 30/50, nominal mín(tx emisor, rx receptor), parcial). `proto.Link` opcional en turn/ready (ClientTurn conserva firma → ClientTurnLink), `Link.Sanitize()` al recibir; sin BSSID/MAC.

## Estado del código (spec 004)
- `internal/probe`: `ParseTarget` (sin resolver; rechaza CIDR/rangos/broadcast/multicast/0.0.0.0/esquemas), tipos ICMP (`Echoer`, `Reply`, `Status` + tabla Win32), estadística (jitter, "< 1 ms"), `Route`/`Router` con `GetBestRoute2` (NextHop 0.0.0.0 = enlace directo: normalizar en addr()). `Env` (env.go: Echoer/Router/Resolver/Clock/Dialer; Dialer en ports.go), `Ping`, `Trace`+`Analyze`, `MTU`, `Ports` (≤4 simultáneos), `Diagnose` (combinado), `NewWinEchoer` (Close espera ecos en vuelo ≤5 s), report.go (Ping/Trace/MTU/Ports Text+JSON, `Dest`, `JSONError` → report.JSONError). Config: `ManualHosts` (máx. 64). App: `AddHost`/`RemoveHost`, `Deps.Router`, gateway id `gateway` (caché 5 s), manuales `host:<target>`, `State.NoAgent`. CLI `nperf ping|trace|mtu|ports` (probe.System()). App: `StartProbe/CancelProbe/WaitProbe`, `State.Probe` (resultado JSON + combinado; MTU < 1500 → `probe.ReducedMTUMessage`), exclusión TEST!/diagnóstico (CanProbe/CanTest). Puente: addHost, removeHost, startProbe(kind lista blanca, id), cancelProbe; probe.System() una vez en NewService, se cierra en Stop. Decisión usuario: sin diagnósticos durante un TEST! ni al revés; sano = pérdida 0 y RTT ≤ 20 ms.

## Estado del código (spec 005)
- `proto`: TypeSample, campos live/seq/index/t_ms omitempty, `SampleFromMessage` (tope 300, cotas: pérdida ≤100, ≤100 Gbps, jitter ≤60 s), Codec.Write con mutex. `meter.Series` (Add TCP, Packet UDP). `client.Prober` (1 sonda/s, una en vuelo, envío fuera del lock), `LoadedRTT`/`BufferbloatNote` (+100 ms). `netinfo.Monitor` (5 min, sin virtuales, solo con Touch). UI `chart.js` (`NPerfChart.render`, único setAttribute en attr() con lista fija; servido en gui/static.go).

## Decisiones cerradas
Puerto 5300 · descubrimiento 5301/UDP · UDP 100 Mbps sin TCP previo · cola máx. 5 · ±10 % vs iperf3 · CLI: comandos y flags en inglés, mensajes en español · interfaz WebView2 (no Walk) · origen siempre "Este equipo".

## Notas
- Subidas: regla en AGENTS.md (wip/actual → CI Windows verde → main, sin pedir permiso; el usuario revisa por tarea).
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.
- UI (`internal/ui`, T14): HTML/CSS/JS plano embebido, CSP estricta, todo dato de la red como texto (test que prohíbe innerHTML y similares), puente `window.nperfCall(nombre, ...args)` → Promise; mock.js solo para previsualizar.

## Próximo paso
Spec 005 olas: 2 = T3,T4,T12,T14 · 3 = T6 · 4 = T7 · 5 = T8,T9,T13 · 6 = T10∥T15 · 7 = T17 · T18. Usuario: validaciones manuales de specs 001-004.

## Pendientes
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"), no reproducido; si reaparece, analizar ese mensaje.

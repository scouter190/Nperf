# 006 - Plan

## Enfoque
- **Paquetes nuevos portables**, sin Win32 salvo el diálogo de guardar: `history` (registros y almacén), `export` (CSV, HTML, SVG), `inet` (latencia y veredicto), `ws` (WebSocket RFC 6455), `ndt7` (locate, medición, límite diario, aviso). Todo con reloj, directorio, `Dialer` y URL base inyectables. `export` no importa `app` (evita ciclos): decodifica la forma compacta de la gráfica (§20) desde el JSON del registro.
- **Registro** (`history.Record`): `format_version` 1, `id`, `time` (RFC 3339 con zona), `kind` (`test`, `ping`, `trace`, `mtu`, `ports`, `internet`, `speed`), `status` (`done`, `cancelled`, `error`), `error`, `version`, `source`/`dest` (`alias`, `ip`, `ssid`, `link`, `nominal_mbps`), `params` y `result` (JSON v1 tal cual: `report.Report`, JSON de `probe`/`inet`/`ndt7`), y en el TEST! `diagnosis`, `chart` y `notes`. Por construcción no hay campos de clave, MAC ni BSSID; `Validate()` además rechaza tipos/tamaños incorrectos al leer.
- **Almacén** (`%LocalAppData%\NPerf\historial`, `os.UserCacheDir()`):
  - Id `AAAAMMDD-HHMMSS-<8 hex aleatorios>` (UTC, así el nombre ordena por fecha); archivo `<id>.json`. `ValidID` (regex fija) en todo id que llegue del puente o la CLI: sin rutas, `..` ni separadores.
  - Escritura: temporal en el mismo directorio + `os.Rename` (como `config.Save`). Nombres únicos → dos procesos no se pisan.
  - Lectura: tope **2 MB** por archivo (un TEST! de 300 s ronda 150 KB); corrupto, mayor o inválido → omitido y contado en `Skipped`. Sin índice: con 50 MB son ~500 archivos y se leen en < 1 s; la app cachea por nombre + tamaño + mtime.
  - Retención tras cada guardado y al arrancar: borra por orden de id lo que tenga > 90 días y, después, lo más antiguo hasta ≤ 50 MB. Errores de borrado no abortan.
  - Desactivado (`history_enabled: false`) → `Save` no escribe. La CLI con `-save` y el historial desactivado no guarda y avisa por stderr (pendiente de confirmar).
- **Config**: `history_enabled` (*bool, ausente = true) e `internet_targets` (máx. 4, IPv4 unicast pública; vacío = 1.1.1.1 y 8.8.8.8). `Load` (CLI) los lee con el mismo criterio tolerante.
- **Latencia a internet** (`inet.Run(ctx, probe.Env, gateway, targets)`): pings en paralelo con `probe.DefaultPing()`; si ningún destino responde ICMP, `Dialer` al 443 (timeout 2 s). `Verdict` es una función pura sobre `probe.Stats` (umbrales de §21). Traceroute (`probe.Trace` + `Analyze`) solo con "proveedor", al primer destino degradado. Puerta de enlace por `Router.Route` como en la 004; caso sin puerta de enlace o que no responde a ICMP: **pendiente de confirmar** (propuesta: veredicto solo con internet y nota "puerta de enlace no responde a ping"). JSON `command: "internet"` y texto en `inet/report.go`.
- **WebSocket** (`ws`): `Dial(ctx, url, Options{TLS, Dialer, UserAgent, Subprotocol, MaxMessage})` sobre `crypto/tls` con ALPN solo `http/1.1`; `Sec-WebSocket-Key` aleatoria y comprobación de `Accept`, 101 y subprotocolo; tramas del cliente enmascaradas (`crypto/rand`); RSV ≠ 0, control > 125 B o fragmentado, opcode desconocido → error; fragmentos reensamblados; ping → pong; cierre con eco del código. `MaxMessage` 1<<24 (ndt7). `Accept` (lado servidor mínimo) solo para servidores de prueba.
- **NDT7** (`ndt7`):
  - Locate v2: `GET {base}/v2/nearest/ndt/ndt7?client_name=nperf&client_version=<v>`, `User-Agent: NPerf/<v>`, `Transport.Proxy = nil`, timeout 10 s, cuerpo ≤ 64 KB. Usa el primer resultado; las URLs `wss:///ndt/v7/download` y `…/upload` se pasan a `ws.Dial` **sin modificar**; esquema distinto de `wss` → fuera de protocolo. 429 → "M-Lab limita las pruebas ahora mismo"; otros fallos con su motivo.
  - Descarga: cuenta bytes de los mensajes binarios; los de texto son `Measurement` (se toma `TCPInfo.MinRTT`, JSON inválido o > 64 KB se ignora). Termina con el cierre del servidor (≈ 10–13 s) o a los 15 s el cliente cierra con nota. Mbps = bytes · 8 / tiempo medido.
  - Subida: mensajes binarios aleatorios de 8 KiB que crecen ×2 mientras sean < enviados/16 (tope 1 MiB), hasta 10 s; luego cierre. Mbps = `TCPInfo.BytesReceived` (o `AppInfo.NumBytes`) / `AppInfo.ElapsedTime` del último `Measurement` del servidor; sin él, bytes enviados / tiempo con nota "estimado por el cliente".
  - Integridad: sin factores ni ajustes, un servidor, una conexión por sentido, sin reintentos. Corte a mitad → lo medido con `incomplete: true` y motivo.
  - `Run(ctx, Options)`: reserva del límite diario → locate → descarga → subida. Resultado `{status, server{machine, city, country}, time, download, upload (mbps, bytes, duration_s, min_rtt_ms|null, incomplete, reason), notes}`.
  - **Límite diario** (`quota.go`): `%LocalAppData%\NPerf\mlab-diario.json` `{"day":"AAAA-MM-DD","count":N}` (día local). `Reserve(now)` bajo un archivo de bloqueo (`O_CREATE|O_EXCL`, reintento ≤ 2 s, bloqueo con > 30 s se rompe) y escritura atómica; cuenta cada prueba iniciada, aunque falle. Contenido corrupto → se reinicia hoy en 0; error de E/S → no se mide ("no se pudo comprobar el límite diario").
  - Aviso y consentimiento (`notice.go`): textos fijos del aviso, `MLabPrivacyURL`, `NPerfPrivacyURL` (URL pública de `docs/privacidad.md`, pendiente de confirmar que el repo es público) y la atribución.
- **Servidor de prueba** (`ndt7/ndt7test`): `httptest.NewTLSServer` con locate (resultados, vacío, 500, 429), descarga a tasa fija, subida que informa `BytesReceived`, y modos de fallo (no cierra, cierra a 13 s, corta a mitad, trama gigante, JSON inválido, subprotocolo erróneo). Cuenta conexiones y peticiones. Lo usan `ndt7`, `app` y `cmd/nperf` (CA inyectada en `tls.Config.RootCAs`, sin red externa).
- **App**: latencia (`internet`) y velocidad (`speed`) son *kinds* del mismo hueco que los diagnósticos (`ProbeState`), así la exclusión de §19 se reutiliza. `StartSpeed(consent bool)`: `false` → error; el consentimiento no se guarda. Guardado en el historial al terminar TEST!, diagnóstico o prueba hacia internet (cualquier estado); fallo → `State.HistoryWarning` "no se pudo guardar en el historial" sin tocar el resultado. API: `History(filter)`, `HistoryRecord(id)`, `DeleteHistory(ids)`, `DeleteAllHistory()`, `ExportHistory(ids, format, opts, path)`, `SetHistoryEnabled`, `SetInternetTargets`, `OpenPrivacy(which)` (solo las dos URL fijas).
- **Exportar**: CSV con `encoding/csv` y separador configurable; columnas fijas en `export/csv.go`; coma decimal con `strconv` + reemplazo; neutraliza `= + - @` (y tab/CR iniciales) con `'` delante, salvo números que formatea NPerf. Intervalos en un segundo archivo `<nombre>-intervalos.csv`. HTML con `html/template` (escape automático), CSS en `<style>`, SVG generado en Go con `export/svg.go`, `@media print` A4 y `prefers-color-scheme`.
- **Diálogo de guardar** (`gui/savedlg_windows.go`): `GetSaveFileNameW` de comdlg32 por LazyDLL en el hilo de la UI, filtro por formato, `OFN_OVERWRITEPROMPT|OFN_NOCHANGEDIR`; `savedlg_other.go` "no soportado".
- **CLI**: `nperf history list|show|export` y `nperf internet [-speed] [-accept-mlab] [-json] [-save]`. Con `-speed` se pregunta al principio (antes de medir latencia) por stderr; terminal detectada con `os.Stdin.Stat()` (`ModeCharDevice`). Códigos: 0 si la latencia se completó (la velocidad "no disponible" va en la salida con motivo), 1 si no se pudo medir nada o el usuario no acepta, 2 uso. Dependencias de `internet` en una variable del paquete que los tests sustituyen (sin flags ocultos).
- **UI**: vista "Historial" (`history.js`), panel "Internet" con diálogo modal de consentimiento y Ajustes (historial y destinos) en `internet.js`; todo con `textContent`. El registro abierto reutiliza el render de resultado y `NPerfChart` en modo solo lectura.

## Paquetes y archivos
- `internal/history/record.go`, `store.go`, `list.go` (nuevos); `store_windows_test.go`.
- `internal/export/csv.go`, `html.go`, `svg.go` (nuevos).
- `internal/inet/verdict.go`, `targets.go`, `latency.go`, `report.go` (nuevos).
- `internal/ws/ws.go`, `accept.go` (nuevos).
- `internal/ndt7/locate.go`, `measure.go`, `run.go`, `report.go`, `quota.go`, `notice.go`, `ndt7test/server.go` (nuevos).
- `internal/config/config.go`.
- `internal/app/history.go`, `internet.go`, `export.go` (nuevos), `test.go`, `probe.go`, `app.go`.
- `internal/gui/bridge.go`, `service.go`, `savedlg_windows.go`, `savedlg_other.go` (nuevos los dos últimos).
- `internal/ui/web/history.js`, `internet.js` (nuevos), `app.js`, `index.html`, `app.css`, `mock.js`; `internal/gui/static.go`; `internal/ui/ui_test.go`.
- `cmd/nperf/history.go`, `internet.go` (nuevos), `main.go`, `probe.go`.
- `docs/privacidad.md`, `docs/validacion-historial-internet.md`; `docs/decisiones.md` §21.

## Riesgos
- **WebSocket/TLS propios**: errores de framing o de ALPN (un servidor que negocie h2 rompería el Upgrade); lo cubren ALPN fijo y el servidor de prueba con casos de fallo.
- **Subida**: la tasa del emisor incluye lo que queda en búferes; por eso se usa la medición del servidor y el ±5 % del criterio 8 se mide frente a lo que recibe el servidor de prueba.
- **Límite diario entre procesos**: el bloqueo por archivo puede quedar huérfano si el proceso muere; se rompe a los 30 s. Cambiar la hora del sistema puede reiniciar el contador (aceptable: es un límite de cortesía local).
- **Diálogo de guardar en el hilo de la UI**: si se llamara desde otra goroutine se colgaría; se despacha con el mismo mecanismo que la bandeja (`tray_windows.go`).
- **Cambios en Locate v2 o ndt7**: el formato de `urls` o los campos de `Measurement` pueden cambiar; cualquier desviación da "no disponible" con "respuesta fuera de protocolo".
- **Tests de las specs 001–005**: añadir `-save` y comandos a `main.go` no debe cambiar la ayuda ni la salida existentes; si algún test previo falla, el implementer lo reporta (no se modifican).
- **Uso comercial**: contribución a M-Lab pedida por la AUP a integradores comerciales; pendiente de producto (§21), no bloquea.
- **Pendientes de confirmar con el usuario**: puerta de enlace que no responde a ICMP o no detectada; `-save` con el historial desactivado; repositorio público para publicar `docs/privacidad.md`.

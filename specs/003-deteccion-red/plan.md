# 003 - Plan

## Enfoque
- **Capas**: toda la lógica (clasificación, tablas WiFi, errores, selección de interfaz, vista, semáforo) es Go portable en paquetes nuevos y se prueba en Linux con datos inyectados. Lo Win32 queda en `_windows.go` finos que solo llenan structs "crudos"; en otros SO, `_other.go` devuelve "no soportado".
- **Fuente inyectable** (`netinfo.Source`): `Adapters() ([]RawAdapter, error)` y `WiFi(guid) (RawWiFi, error)`. `RawAdapter`: nombre, descripción, MAC, IPv4, IfType, OperStatus, tx/rx bps, `Physical *bool` (ConnectorPresent si está disponible). `RawWiFi`: SSID, BSSID, PHY type, calidad %, RSSI, frecuencia kHz, tx/rx kbps, y el código de error Win32 (números portables).
- **Clasificación**: IfType 71 → WiFi; 6 → Ethernet salvo virtual (`Physical=false` o descripción con Hyper-V/Virtual/VPN/TAP/WSL/VMware/VirtualBox) → Otro; 24 loopback se omite; resto → Otro.
- **Errores WLAN → motivo**: 1062 (servicio detenido) / 2 o DLL ausente → "servicio WLAN no disponible"; 5 (acceso denegado) → "permiso de ubicación denegado" + `LocationDenied=true`; 5023 o estado ≠ conectado → "sin conexión". Nunca fatal: tipo y velocidad de `GetAdaptersAddresses` se conservan.
- **Windows**: `windows.GetAdaptersAddresses` (+ `GetIfEntry2Ex` para ConnectorPresent si x/sys lo trae; si no, solo heurística). `wlanapi.dll` por `windows.NewLazySystemDLL`: `WlanOpenHandle`, `WlanEnumInterfaces`, `WlanQueryInterface(wlan_intf_opcode_current_connection)` (SSID, BSSID, PHY, calidad, tx/rx kbps), `WlanGetNetworkBssList` filtrado por BSSID (frecuencia, RSSI), `WlanFreeMemory`, `WlanCloseHandle`. Structs declarados a mano con test de tamaño.
- **Caché**: `netinfo.Cached(src, 2s)` para que beacon (3 s), panel (5 s) y pruebas no consulten Windows de más.
- **Anuncio** (compatibilidad): claves opcionales `link=eth|wifi|other`, `tx=`, `rx=` (bps enteros) y `ssid=` (url.PathEscape). El `Parse` actual ya ignora claves desconocidas, así que versiones previas no se rompen; las nuevas tratan su ausencia como "desconocido". SSID inválido (> 32 B, control) se omite al emitir y se descarta (solo el SSID) al recibir. Se trunca el alias, nunca el SSID, para quedar ≤ 512 B.
- **Resumen por canal autenticado** (compatibilidad): `proto.Link{kind, tx_bps, rx_bps, ssid, band, standard, signal_pct}` (sin campos BSSID/MAC por construcción) como campo opcional `link` de `Message`: el cliente lo envía en `turn` y el servidor en `ready`, ambos posteriores al handshake. `encoding/json` ignora campos desconocidos: un par antiguo ni lo envía ni falla → "parcial". Interfaz de cada extremo = la de `conn.LocalAddr()` del control. Proveedores inyectables: `client.Options.LocalLink` y `server.WithLinkProvider`. `client.Result` gana `LocalLink`/`RemoteLink`; `internal/report` no los usa (salida y JSON v1 sin cambios).
- **Semáforo** (`internal/diag`): función pura `Evaluate(throughputBps, emisor, receptor)` por sentido TCP con las reglas de la spec; el cliente usa su enlace al recibir `ready` (inicio de la prueba). fwd: emisor = Este equipo; rev: emisor = remoto.
- **Backend/UI**: `Device` gana `link`, `link_speed`, `ssid`; `App.Adapters()` devuelve la vista compacta/detallada + aviso de ubicación; `TestState.Diagnosis` con color, %, texto, parcial por sentido. Puente: `adapters` y `openLocationSettings` (URL fija `ms-settings:privacy-location`, sin argumentos, `ShellExecuteW`). La UI sondea `adapters` cada 5 s solo con el panel visible.
- **CLI**: `nperf adapters [-json]` usa la misma vista; JSON `format_version: 1`, null + `status` para "no disponible".

## Paquetes y archivos
- `internal/netinfo` (nuevo): `adapter.go` (tipos, clasificación), `format.go` (bps → texto), `ssid.go` (`ValidSSID`), `wifi.go` (frecuencia → banda/canal, PHY → estándar, error → motivo), `inventory.go` (`Source`, `Inventory`, `Cached`), `select.go` (`ForLocalIP`, `Summary` → `proto.Link`), `view.go` (vista compacta/detallada, JSON con status), `source_windows.go` (GetAdaptersAddresses), `wlan_windows.go` (wlanapi), `source_other.go`.
- `internal/diag` (nuevo): `diag.go`.
- `internal/discovery`: `announce.go`, `beacon.go` (`LinkFor func(netip.Addr)`), `table.go` (Peer ya embebe Announce).
- `internal/proto`: `proto.go` (`Link`, `Message.Link`, `ClientTurn` devuelve el `ready`).
- `internal/client`: `client.go` (`Options.LocalLink`, `Result.LocalLink/RemoteLink`); `internal/server`: `server.go` (`WithLinkProvider`, registra el enlace del cliente).
- `internal/app`: `app.go`, `test.go`, `runtime.go`, `adapters.go`; `internal/gui`: `bridge.go`, `open_windows.go`/`open_other.go`; `internal/ui/web/*`.
- `cmd/nperf`: `adapters.go`, `main.go`; `docs/validacion-red.md`; `docs/decisiones.md` §18.

## Riesgos
- **Structs WlanAPI a mano**: un error de alineación da basura o cuelgues; se fijan tamaños con `unsafe.Sizeof` en tests `_windows` y el CI de Windows lo ejecuta (sin WLAN en el runner: solo se prueba el camino "no disponible"; los datos reales, a mano, criterio 12).
- **Clasificación de virtuales**: la heurística por descripción puede fallar con drivers raros → "Otro" o "Ethernet" erróneo; se revisa en la validación manual.
- **24H2 y ubicación**: el código de error exacto puede variar por build; si aparece otro, se mapea a "no disponible" genérico sin aviso (se ajusta tras el criterio 13).
- **`ClientTurn` cambia de firma**: afecta a cliente y tests de cola; los tests de spec 001/002 deben pasar sin cambios de comportamiento (criterio 11).
- **Coste de consulta**: WlanAPI cada 2–3 s en portátiles; la caché lo limita. Si se nota, subir el TTL sin cambiar contratos.
- **Runners como administrador**: "sin admin" y "servicio WLAN detenido" solo se verifican a mano (criterio 16).

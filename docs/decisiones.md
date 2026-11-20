# memory.md - NPerf

## 1. Concepto
- **NPerf**: utilidad ligera de diagnóstico de red para Windows, uso similar a iperf.
- Mide enlaces punto a punto: equipo-equipo, equipo-router, equipo-AP.
- Detecta tarjetas de red, tipo de conexión y WiFi; señala cuellos de botella.

## 2. Plataforma
- **Windows** únicamente. Lenguaje **Go**. **.exe portable** (sin instalador en v1).
- Go 1.27.3 (go.mod, CI y entorno local).
- Interfaz: **Walk (Win32 nativo)**, consumo mínimo.
- **Sin administrador**. Diagnósticos que lo requieran se omiten con aviso.
- Firewall: la app detecta si está bloqueada y muestra un **aviso guiado** para permitirla.
- Al cerrar la ventana **se minimiza a la bandeja** y sigue respondiendo y midiendo.
- Arranque con Windows: **configurable**.

## 3. Red y descubrimiento
- **Punto a punto total**: cada equipo mide contra cualquier otro.
- Descubrimiento automático en la **subred local**; subredes extra **a mano**.
- **Puerto fijo** por defecto, configurable.
- **Clave compartida**, guardada en la configuración local.
- El mapa lista **todos los equipos con NPerf abierto**. Inalcanzables **en gris**.
- Versiones: compatibles si coincide la **versión mayor** (1.x con 1.x); si no, aviso.

## 4. Protocolo de medición
- Protocolo **propio** (opción A). Compatibilidad con iperf3 fuera de v1.
- Validar resultados contra iperf3 en la misma red.
- Control TCP autenticado. Negociación: `sentido`, `duracion`, `flujos`, `tasa_udp`.
- Por defecto TEST! corre **TCP y luego UDP**.
- TCP: **4 flujos paralelos** por defecto, configurable.
- UDP: tasa = **90 % del throughput TCP** medido.
- Métricas: throughput, jitter, pérdida, retransmisiones, RTT (ping antes de la carga).
- Concurrencia: si un equipo ya está en prueba, la nueva va a **cola con aviso**.

## 5. Sentido
- El usuario elige: **Origen → Destino**, **Destino → Origen** o **Ambos** (default).
- Etiquetas con nombres de equipos. Sentido no medido = **"no medido"**.

## 6. Duración
- **Combo box** editable: 5, 10 (default), 30, 60, 120, 300 s. Rango **3–300 s**.
- En "Ambos" aplica **por sentido**; se muestra el total.
- **Recuerda** la última elección. Aplica a pruebas **manuales y programadas**.

## 7. Interfaz
- ~~Tema tipo **MikroTik / WinBox**: grises con acento azul, colores suaves.~~ Desde la spec 008: paleta "NOC Telemetry + Precision Slate" (§23).
- Claro/oscuro **automático** según el sistema. Idioma **español**.
- **Compacta** por defecto, con modo **detallado**.
- ~~Barra superior fija: **[Sentido ▾] [Duración ▾] [TEST!]**.~~ Desde la spec 008: barra de TEST! fija solo en la vista Panel (§23).
- Selección: **clic en dos nodos** del mapa, luego TEST!.
- Resultado: en el **panel de detalle** y el enlace cambia de color en el mapa.

## 8. Mapa y visualización
- Nodos: equipos, router, AP, gateway; enlaces coloreados.
- Cada nodo: **nombre, IP, tipo de conexión, estado, versión de NPerf**.
- Velocidad de enlace: **1000/1000, 100/100, 10/10** Mbps, cable y WiFi.
- Gráficas en vivo de los **últimos 5 min**. Tablas con semáforo. Panel de detalle a la derecha.

## 9. Semáforo y diagnóstico
- % del enlace nominal: verde > 80 %, amarillo 50–80 %, rojo < 50 %.
- Cuello: cada enlace contra el **más lento de su tramo**.
- Subida y bajada se comparan y reportan por separado.

## 10. Tarjetas de red
- **Identidad:** nombre, fabricante, MAC, IP, driver.
- **Enlace:** Ethernet/WiFi/USB/celular, velocidad, duplex.
- **Estado:** conectada, errores, descartes, uso.
- **Capacidades:** estándar WiFi, bandas, Wake-on-LAN, ahorro de energía.
- **WiFi:** RSSI, banda, canal, ancho de canal, 802.11, BSSID, ruido.

## 11. Sin agente y datos técnicos
- Routers y APs: **ping y traceroute**. Sin SNMP ni agente en v1.
- Topología: **tabla ARP y gateway**. Sin LLDP ni dibujo manual.
- Técnicos: traceroute por salto, MTU, puertos y conexiones TCP/UDP.

## 12. Ejecución
- Manual (TEST!) y programada con **intervalo fijo**, mayor que la duración total.
- Prueba hacia internet: **opcional**.

## 13. Histórico y alertas
- ~~**Solo en vivo**~~ Cambiado por el usuario en la spec 006: **historial automático en disco** (ver §21). Exportar en CSV, JSON e informe HTML.
- Alertas: color en pantalla, sonido opcional.
- Notificación de Windows **solo si hay cuello** en prueba programada.
- Registro de eventos en **archivo de texto** con fecha y hora.

## 14. Proyecto
- Nombre **NPerf**. Repo nuevo en GitHub, propuesta `scouter190/nperf` (pendiente).
- Si se acaba el crédito en la nube: clonar el repo y seguir con Claude Code local.

## 15. Pendientes
- [ ] Confirmar dueño y nombre del repositorio.
- [x] ~~Confirmar Walk como librería de interfaz.~~ Reemplazado por WebView2 (§17).
- [ ] Escribir la especificación en el repositorio.
- [ ] Validar resultados contra iperf3.

## 16. Medidor por CLI (spec 001)
- Un solo `.exe` con modos `nperf server` y `nperf client <host>`; salida legible y `-json`.
- Comandos y flags en inglés (`-port`, `-key`, `-proto tcp|udp|both`, `-direction fwd|rev|both`, `-duration`, `-streams`, `-udp-rate`, `-json`); mensajes al usuario en español.
- Sentido en CLI: `fwd` (A → B), `rev` (B → A), `both` (default).
- La clave nunca viaja en claro por el canal de control.
- Validación: throughput TCP dentro de ±10 % de iperf3 en la misma red cableada.
- Puerto por defecto 5300 (no 5201, para convivir con iperf3).
- UDP sin TCP previo (`-proto udp` sin `-udp-rate`): 100 Mbps.
- Cola: máximo 5 en espera; el siguiente se rechaza con "cola llena"; el cliente cancela con Ctrl+C.
- Control: JSON por línea; TCP y UDP en el mismo puerto; el cliente abre todas las conexiones (también en `rev`).
- Autenticación por desafío y respuesta HMAC-SHA256.
- Módulo Go `nperf` hasta confirmar el repositorio.
- Desarrollo y tests en Linux; lo específico de Windows con build tags; CI en `windows-latest`.
- Secuencia por defecto (`-proto both`): RTT (10 ping/pong por el canal de control, antes de cualquier carga; mín/media/máx), TCP fwd, TCP rev, UDP fwd, UDP rev. UDP = 90 % del TCP del mismo sentido, con tope de 10 Gbps (en loopback el TCP da decenas de Gbps); `-udp-rate` manda sobre la derivación. Si el TCP de un sentido da una tasa inutilizable (< 1 datagrama/s), ese UDP queda "no medido" con una nota; un error TCP/UDP aborta toda la medición.
- Streams: el cliente envía siempre el número explícito (default 4, rango 1–64); el server trata 0 como 1 solo por compatibilidad.
- Retransmisiones: segmentos exactos en Linux (tcpi_total_retrans); en Windows estimación BytesRetrans/MSS, mostrada con ~. Las ve el emisor; en rev el server las envía en el resultado. Sin soporte: "no disponible".
- Keepalive TCP de 15 s en el server: un cliente en cola con la conexión caída se detecta en ~15 s + sondas.
- Cola: 1 en curso + 5 en espera, sin límite por IP por defecto (configurable con WithMaxPerIP); la clave compartida evita que extraños usen la cola.
- Estado del equipo: consulta `status` (disponible / ocupado + cola) en el protocolo desde la spec 001; el indicador visual va en la spec de interfaz.
- Consulta de estado pública en la LAN (decisión del usuario): solo expone state y queue. Sin ids de sesión, IPs, nombres, versión exacta ni detalles de la prueba; mensaje propio antes del handshake que no habilita ningún otro paso sin autenticar; no ocupa turno ni entra en la cola; el server responde en < 1 s y cierra la conexión.
- Formato JSON v1 (`nperf client ... -json`): stdout lleva solo un objeto; avisos y cola van por stderr. Campos en inglés, snake_case, estables, con `"format_version": 1`. Raíz: `format_version`, `params` (`host`, `port`, `proto`, `direction`, `duration_s`, `streams`, `udp_rate_bps` o null si es automática, `version`), `rtt` (`status`, `min_ms`, `avg_ms`, `max_ms`, `samples`), `tcp` y `udp` (cada uno con `fwd` y `rev`), `notes` (lista). Cada sentido: `status` (`measured` | `not_measured`), `throughput_mbps` y `bytes` (medidos por el receptor), `duration_s`, `retransmissions` (solo TCP: `status` `available` | `unavailable`, `count` o null, `estimated`) y `udp` (solo UDP: `sent_mbps`, `sent_bytes`, `target_rate_bps` o null, `jitter_ms`, `loss_pct`, `datagrams_sent`, `datagrams_expected`, `datagrams_lost`). Lo "no medido" / "no disponible" se expresa con `status` y valores null, nunca con 0. Error: `{"error": "..."}` por stdout con el mismo código de salida que sin `-json`. Cambios incompatibles suben `format_version`.

## 17. Spec 002 (interfaz y descubrimiento)
- Alcance: ventana + descubrimiento + TEST!. Fuera: mapa gráfico, adaptadores/WiFi, semáforo, gráficas en vivo, pruebas programadas, alertas.
- Interfaz: **WebView2** (incluido en Windows 10/11) con HTML/CSS/JS embebido en el `.exe`; **reemplaza a Walk** (§2 y pendiente de §15 quedan superados). Se admite **una** dependencia Go pura (sin cgo) para el puente, previa revisión de licencia y mantenimiento. Cada `.exe` es un solo archivo.
- **Dos ejecutables** con el mismo código: `nperf.exe` (CLI de consola, sin cambios; sin argumentos muestra la ayuda) y `nperf-gui.exe` (subsistema Windows GUI, doble clic sin consola). El CI compila y publica ambos como artefactos; `scripts/build-windows.*` generan ambos.
- Descubrimiento: anuncio UDP broadcast propio **cada 3 s** en cada interfaz IPv4, puerto propio **5301/UDP** (configurable). Público en la LAN (como `nperf status`): alias, IP, puerto, estado (disponible/ocupado) y cola. Equipo sin anunciarse **10 s** pasa a gris (no seleccionable).
- Subredes extra a mano: IP o CIDR de **máx. /24**, consultadas por unicast en 5301/UDP.
- Consulta unicast de descubrimiento: `NPDISC1 ?` rellenada hasta 128 bytes (anti-amplificación, factor ≤ 4x) y como máximo 1 respuesta por IP y segundo.
- Origen de la prueba: **siempre "Este equipo"**; el usuario elige solo el destino. Control remoto A → B desde un tercer equipo: spec futura, junto al mapa.
- TEST! pasa a "Cancelar" durante la prueba o la cola. **Una sola instancia** de la app por usuario. Sin equipos en **15 s**: sugerencia de revisar el firewall.
- Nombre mostrado: alias configurable por técnico + IP; sin alias, el nombre del equipo de Windows.
- La app abierta actúa **siempre como server** (clave de `config.json` o la fijada en la app).
- Bandeja incluida: cerrar la ventana deja NPerf en la bandeja (server y anuncio activos); se sale desde el menú de la bandeja.
- Selección en la 002 desde una **lista** (el mapa vendrá después).
- Puente WebView2 (T1): **`github.com/jchv/go-webview2` v0.0.0-20260205173254-56598839c808** (2026-02-05, sin tags; MIT; sin cgo; embebe `WebView2Loader.dll` y lo carga desde memoria con `jchv/go-winloader` (ISC, sin cgo, pura Go) => un solo `.exe`). Dependencias transitivas: go-winloader (la que carga el DLL embebido) y `golang.org/x/sys`. Descartada `wailsapp/go-webview2` v1.0.23 (2025-11-13, MIT): solo trae bajo nivel (`pkg/edge`), sin API de ventana/Bind lista, y arrastra testify/yaml en go.mod. Ejemplo mínimo (ventana "NPerf", HTML, `Bind`) compilado con `GOOS=windows CGO_ENABLED=0 -ldflags -H=windowsgui`: un único .exe de 3,7 MB. Actividad del repo no verificable por GitHub (sin acceso); fechas del proxy de Go. Aceptado por el usuario: dependencias github.com/jchv/go-webview2 (MIT) y github.com/jchv/go-winloader (ISC), versiones fijadas por pseudo-versión exacta. Riesgo: go-winloader carga WebView2Loader.dll desde memoria, técnica que algunos antivirus vigilan. Validación: el usuario prueba nperf-gui.exe del CI con Windows Defender tras T3; si lo marca, se pasa a distribuir WebView2Loader.dll junto al .exe.
- Puente WebView2 (T15): go-webview2 no expone NavigationStarting ni NewWindowRequested, así que no se puede cancelar la navegación. Mitigación aceptada (sin fork ni COM manual): secreto aleatorio de 32 bytes por ejecución, entregado solo en el `index.html` servido (meta `nperf-session`) y exigido como primer argumento de cada `nperfCall` (comparación en tiempo constante, si falta o no coincide: "no autorizado"); CSP sin enlaces externos. Riesgo residual: si la UI navegara por error, una página externa podría mostrarse en la ventana, pero no puede invocar el puente.

## 18. Spec 003 (detección de red y cuello de botella)
- Anuncio público: tipo de enlace (`eth`/`wifi`/`other`), velocidad tx/rx y SSID (≤ 32 bytes, sin caracteres de control, solo como texto en la UI); sigue ≤ 512 B. BSSID y MAC nunca se comparten. Riesgo aceptado por el usuario: esos datos los ve cualquiera en la LAN.
- Semáforo WiFi con umbrales propios: verde > 50 %, ámbar 30–50 %, rojo < 30 % (sobre la velocidad PHY). En cable siguen los de §9.
- CLI: nuevo `nperf adapters [-json]`; `nperf client` sin semáforo ni datos de enlace (JSON v1 sin cambios).
- Sin permiso de ubicación (Windows 11 24H2+): aviso con botón a Configuración › Privacidad › Ubicación.
- Dúplex Ethernet fuera de la 003.
- Compatibilidad (plan 003): el anuncio añade claves opcionales `link`, `tx`, `rx`, `ssid` (las versiones previas ya ignoran claves desconocidas). El resumen de enlace viaja como campo opcional `link` en `turn` (cliente) y `ready` (servidor), tras el handshake; sin campos BSSID/MAC por construcción. Par sin `link` → diagnóstico "parcial".
- Lógica de red portable en `internal/netinfo` (fuente inyectable) y `internal/diag`; Win32 (GetAdaptersAddresses, wlanapi.dll por LazyDLL) solo en `_windows.go`.
- Correcciones tras validación en Windows 11 (usuario): canal WiFi por `WlanQueryInterface` (channel_number) y banda deducida de canal+PHY (BSS preferente para la frecuencia); Wi-Fi Direct (tipo 71 virtual) es "otro", no WiFi; adaptadores virtuales (Hyper-V/vEthernet, Tailscale, WireGuard, TAP, VPN, VirtualBox, VMware, Loopback, Wi-Fi Direct) se muestran como "virtual" con velocidad nominal entre paréntesis y nunca cuentan para el resumen de enlace ni el semáforo (IP local en virtual → desconocido/parcial); Bluetooth PAN sigue como "otro" físico.

## 19. Spec 004 (equipos sin NPerf)
- ICMP sin administrador con `IcmpSendEcho2` (iphlpapi.dll); TTL y DF con `IP_OPTION_INFORMATION`; sin sockets raw ni dependencias nuevas. **Solo IPv4**.
- Puerta de enlace IPv4 por defecto: se añade **siempre** a la lista como "sin NPerf".
- Destinos sin NPerf **solo a mano** en la 004 (guardados en `config.json`, máx. 64). Propuesta desde la tabla ARP: spec posterior. **Sin barridos** de subred.
- Puertos TCP por defecto: 21, 22, 23, 53, 80, 443, 445, 3389, 8080, 8291; máx. 32 por ejecución, 4 conexiones simultáneas.
- Interfaz: grupo **"Sin NPerf"** en la misma lista, icono propio; los diagnósticos también se ofrecen para equipos con NPerf.
- Análisis de traceroute: pérdida ≥ 20 % o subida de latencia ≥ 20 ms solo cuentan si persisten en los saltos siguientes.
- CLI `ping|trace|mtu|ports`: código 0 completado, 1 no se pudo hacer (incluye ping con 100 % de pérdida), 2 uso; `-json` con `format_version: 1` y `command`.
- Plan 004: lógica portable en `internal/probe` con entorno inyectable (`Echoer`, `Router`, `Resolver`, `Clock`, `Dialer`); Win32 solo en `icmp_windows.go` y `route_windows.go` (LazyDLL de iphlpapi, structs a mano con test de tamaño).
- Plan 004: `IcmpSendEcho2` síncrono en una goroutine por eco; al cancelar, el runner vuelve en ≤ 1 s y no envía más; un handle ICMP por diagnóstico.
- Plan 004: puerta de enlace = siguiente salto de `GetBestRoute2` hacia una IP pública de referencia (solo consulta la tabla de rutas, no envía paquetes). MTU de interfaz fuera de 576–9216 (p. ej. loopback) se trata como desconocido → 1500.
- Plan 004: la interfaz lanza los diagnósticos con los valores por defecto de la spec (sin opciones en la ventana).
- Usuario: mientras corre un TEST! los diagnósticos (ping, traceroute, MTU, puertos) quedan desactivados (el TEST! satura la red y falsearía latencia y pérdida); y viceversa, no se inicia un TEST! con un diagnóstico en curso. "Enlace sano" = pérdida 0 y RTT medio ≤ 20 ms. `-timeout` de `ports`: 0.2–5 s.

## 20. Spec 005 (gráficas en vivo)
- Muestras del TEST! cada **1 s**, generadas por el lado que mide; se piden con un campo opcional en `params`. Un par antiguo no da muestras, ni RTT bajo carga, ni gráfica, y el resultado no cambia. Los resultados finales se calculan igual que antes.
- **RTT bajo carga**: el cliente envía 1 sonda ping/pong por segundo por el canal de control durante cada fase (pequeña, sin conexiones nuevas, una en vuelo, 1 s de timeout). Nota "bufferbloat" si la mediana bajo carga ≥ RTT medio inicial + 100 ms (**supuesto**, se revisa tras la validación).
- Monitor de tarjetas: ventana de **5 min** (§8), solo adaptadores físicos, `GetIfTable2` sin admin, muestreo solo con el panel visible.
- CLI: `-json` añade `intervals` y `loaded_rtt` por sentido (cambio aditivo, `format_version` 1); la salida legible imprime una línea por intervalo. Sin `nperf monitor`.
- Fuera de la 005: ping continuo con gráfica (spec posterior) y exportación de sesión/muestras (spec propia, §13). La gráfica del resultado vive en memoria hasta el siguiente TEST!.
- Gráficas en SVG/canvas propio sin librerías externas, accesibles (valores en texto, no solo color), con temas claro y oscuro.
- Usuario (aprobación de la spec): criterio nuevo, el mismo TEST! con y sin sondas de RTT bajo carga da velocidades con diferencia < 1 % (CI y manual). Flag oculto `-no-load-rtt` en `nperf client` (no sale en la ayuda) que desactiva solo las sondas.
- Plan 005, compatibilidad: el server viejo ignora `live` en `params` (`json.Unmarshal` sin campos estrictos), pero durante la fase toma cualquier mensaje del control como abandono. Por eso el cliente solo sondea si `start` trae `live: true`. Confirmación por fase, sin cambios en el handshake.
- Plan 005, protocolo: `sample` por el control, vaciadas antes de `result`. `ping`/`pong` con `seq` durante la fase (tope duración + 10 por fase). En sesiones `live`, el server también contesta pings entre fases. El cliente descarta `pong` sueltos y espera ≤ 1 s la sonda en vuelo tras `result`. `Codec.Write` con mutex.
- Plan 005, intervalos: ⌈D⌉ ventanas de 1 s desde el primer byte o datagrama; la última queda abierta hasta el final real. UDP: pérdida del intervalo = máx(0, esperados − recibidos), los desordenados cuentan donde llegan, y el jitter es el estimador RFC 3550 al cerrar. La sonda k (enviada a k s del `start` del cliente) da el RTT de la muestra k. Las estadísticas usan solo las sondas respondidas.
- Plan 005, JSON: sentido `not_measured` → `intervals` y `loaded_rtt` null; medido sin muestras → `intervals` null y `loaded_rtt.status` `unavailable`. Las líneas por intervalo de la consola van en minúsculas ("tcp ida 3-4 s: …, rtt …"), sin "TCP fwd" ni "RTT:", para no alterar el resumen.
- Plan 005, monitor: emparejado por GUID con el inventario de §18. Se activa con un permiso de 3 s que renueva la UI (`monitor(true)` cada 1 s con el panel visible); sin renovar, no se consulta. Ocultar a la bandeja lo para. `MIB_IF_ROW2` a mano con test de tamaño. Hueco > 3 s → el punto solo fija la base.
- Plan 005, prueba de "sondas sin efecto" en CI: conexiones con tasa limitada en el test (token bucket), 3 + 3 TEST! alternos, mediana y bytes por intervalo < 1 %.

## 21. Spec 006 (historial, exportar e internet)
- Usuario: historial automático **activado por defecto**, desactivable en Ajustes; retención **90 días o 50 MB** (se borra lo más antiguo). Reemplaza "solo en vivo" de §13. En `%LocalAppData%\NPerf\historial`, un JSON por resultado.
- Usuario: informe para el cliente en **HTML autocontenido** (SVG embebido, sin recursos externos), PDF desde el navegador. Incluye **siempre IPs y SSID** (sin casilla); clave, MAC y BSSID **nunca** (tampoco en el historial).
- CSV UTF-8 con BOM: ventana en formato Excel español (`;`, coma decimal); CLI estándar (`,`, punto) con `-csv excel-es` opcional. Fórmulas neutralizadas.
- Latencia a internet: destinos por defecto 1.1.1.1 y 8.8.8.8, editables (máx. 4, IPv4); TCP 443 si no hay ICMP; traceroute solo con veredicto "proveedor". Internet degradado: pérdida ≥ 2 %, RTT medio > 100 ms o jitter > 30 ms (**supuesto**, se revisa tras la validación).
- Usuario: velocidad de descarga/subida con **M-Lab NDT7** (locate API, WebSocket sobre TLS), solo con botón explícito o `nperf internet -speed`; ~10 s por sentido, tope 15 s; fallos → "no disponible" con motivo; sin proxy. Cliente WebSocket/NDT7 propio con la librería estándar (sin dependencias nuevas). Riesgo aceptado por el usuario: M-Lab publica la IP pública de quien mide; aviso visible en UI, CLI e informe. Términos de uso de M-Lab a confirmar en el plan.
- CLI: `nperf history list|show|export`, `nperf internet [-speed]`; la CLI solo guarda en el historial con `-save`.
- Exclusión de §19 ampliada a las pruebas hacia internet.
- Usuario (aprobación de la spec), condiciones de M-Lab verificadas en sus páginas oficiales:
  - **Consentimiento informado antes de cada prueba** de velocidad (https://www.measurementlab.net/develop/: "prior to installation of the client or initiation of the test"): diálogo en la UI; en la CLI confirmación por stdin salvo `-accept-mlab`. Enlaces a la política de M-Lab (https://www.measurementlab.net/privacy/) y a la de NPerf.
  - **Política de privacidad propia** publicada (`docs/privacidad.md`), compatible con la AUP (https://www.measurementlab.net/aup/): NPerf no envía datos fuera del equipo salvo la prueba a M-Lab; M-Lab publica IP pública, fecha/hora y metadatos como CC0 con retención indefinida.
  - **Máx. 40 pruebas interactivas al día por usuario de Windows en este equipo** (AUP): contador local persistente; sin pruebas M-Lab automáticas ni programadas.
  - **Locate v2** (https://www.measurementlab.net/develop/locate-v2) con las URLs devueltas sin tocar (`access_token`); identificación del cliente (`NPerf/<versión>`) según ndt7 (https://github.com/m-lab/ndt-server/blob/main/spec/ndt7-protocol.md); hasta ~10 s por sentido (el servidor puede cerrar a los 13 s).
  - Prohibido alterar resultados, compensar por medir o interferir con la plataforma.
  - **Pendiente de producto (no bloquea)**: si NPerf se comercializa, la AUP pide a integradores comerciales una contribución a M-Lab (patrocinio o en especie).
- Plan 006, paquetes portables `history`, `export`, `inet`, `ws`, `ndt7` (+ `ndt7test`, servidor TLS local para CI); Win32 solo en el diálogo de guardar (`GetSaveFileNameW`).
- Plan 006, historial: id `AAAAMMDD-HHMMSS-<8 hex>` (UTC) = nombre de archivo; tope 2 MB por registro; sin índice (caché en la app); config `history_enabled` (ausente = true) e `internet_targets`.
- Plan 006, M-Lab: una prueba = descarga + subida; se cuenta al iniciarse aunque falle; contador en `%LocalAppData%\NPerf\mlab-diario.json` (por perfil de Windows), día local. Subida medida con lo que informa el servidor (`BytesReceived`/`ElapsedTime`); sin él, estimación del cliente con nota. Un servidor, una conexión por sentido, sin reintentos; sin proxy (`Transport.Proxy = nil`).
- Plan 006, app: latencia (`internet`) y velocidad (`speed`) usan el mismo hueco que los diagnósticos; `StartSpeed(consent)` exige `true` en cada llamada.
- Plan 006, CLI `internet`: con `-speed` el consentimiento se pide al principio; 0 si la latencia se completó (velocidad "no disponible" va en la salida), 1 si no se midió nada o no se acepta, 2 uso (incluye sin terminal y sin `-accept-mlab`, o `-accept-mlab` sin `-speed`).
- Usuario (plan 006): si la puerta de enlace no responde al ping o no se detecta, se juzga solo internet con la nota correspondiente; `-save` con el historial desactivado avisa y no guarda. El repositorio es público: la política de privacidad se enlaza a docs/privacidad.md en GitHub.

## 22. Spec 007 (empaquetado v1)
- Usuario: alcance = saludo UDP estable, icono propio, versión y VERSIONINFO, publicación en borrador. Fuera: firma de código, instalador, auto-actualización, manual, rediseño de UI. Ningún tag ni Release lo crea un agente: lo decide el usuario.
- Versión: `version.Current` (var, "1.0.0") sigue siendo la versión de **protocolo** y de compatibilidad; `version.Build` (por defecto "dev", `-ldflags -X` desde el tag `vX.Y.Z`) solo se muestra. Un tag solo se publica si es `v` + `Current`.
- Icono: generador propio `tools/icongen` (solo biblioteca estándar); la geometría en Go es la fuente y produce SVG, ICO (16-256 px, entradas PNG) y vista previa, commiteados y comprobados por test. Propuesta: cuadrado redondeado azul #1E6FD9 con dos flechas blancas opuestas; pendiente de aprobación del usuario.
- `.syso`: `go run github.com/tc-hib/go-winres@<versión fija>` (candidata v0.3.3, MIT; se confirma en T10) desde `scripts/gen-resources.sh`, sin tocar `go.mod`. Los `.syso` (solo amd64, con versión dev) se commitean; la publicación los regenera con la versión del tag. Icono como grupo ID 1 (bandeja y `WindowOptions.IconId`). Sin manifiesto. Descartados: generador COFF propio (complejo y frágil) y dependencia en `go.mod`.
- VERSIONINFO: ProductName "NPerf", CompanyName vacío, FileDescription "NPerf (consola)" / "NPerf (interfaz)", OriginalFilename, FileVersion X.Y.Z.0 (dev = 0.0.0.0), LegalCopyright con titular pendiente del usuario (marcador `TITULAR-PENDIENTE` que bloquea la publicación).
- Publicación: tag `vX.Y.Z` → windows-latest, vet + tests, zip `NPerf-X.Y.Z-windows-amd64.zip` (exes, README.md, privacidad.md, licencias de terceros, LICENSE si existe) + `SHA256SUMS`, Release **borrador** con la sección del CHANGELOG. Empaquetado en `tools/release` (Go estándar), probado en cada CI con un paquete de prueba.
- Saludo UDP (T1/T2): causa = `greetUDP` medía el presupuesto en intentos (10 × 300 ms = 3 s), no en tiempo, y cada datagrama ajeno (restos de la fase anterior) gastaba un intento y reenviaba al instante; con CPU saturada (-race, UDP de Gbps en loopback) el servidor tardaba > 3 s o los intentos se agotaban en ms. Corrección: plazo total 5 s, reenvío cada 300 ms (tope 16), lecturas ajenas sin gastar reenvíos, mismo error, respeta ctx. Efecto aceptado: un error de lectura no-timeout (ICMP inalcanzable) espera hasta 5 s.
- Usuario (spec 007): licencia propietaria, "todos los derechos reservados" (el código es visible en GitHub pero sin permiso de reutilizar ni redistribuir); titular del copyright: scouter190. Las dependencias de terceros mantienen sus licencias (docs/licencias-terceros.txt).

## 23. Spec 008 (rediseño de la interfaz)
- Usuario: rediseño según `docs/nperf_gui_parameters.md` (app shell, drawers, modales). Se aprueban la paleta "NOC Telemetry + Precision Slate" y la barra de TEST! solo en la vista Panel (sustituyen dos puntos de §7).
- Usuario: sin estado de reinicio del servidor en State; el indicador de la cabecera muestra solo "Activo" o "con error" (`config_error`), y el resultado de guardar se ve en el banner del drawer Ajustes.
- Usuario: `test.history_id` y `probe.history_id` (aditivos, vacíos si no se guardó o se borró). Exportar del Panel usa ese registro; con el historial desactivado, el botón queda deshabilitado con el motivo.
- Usuario: en el modal M-Lab, texto fijo "límite de 40 pruebas al día" (la cuota restante no se expone).
- Usuario: solo fuentes de Windows (Segoe UI Variable / Cascadia Mono con fallbacks); sin fuentes embebidas, sin cambios de CSP, sin selector manual de tema (solo `prefers-color-scheme`).
- Usuario: se ajustan solo los tokens del documento que no alcanzan el contraste AA (4,5:1), con un test de contraste en CI.
- Usuario: BSSID y MAC solo en el drawer local de Tarjetas de red; nunca en Copiar resumen ni en exportaciones (coherente con §18 y §21).
- Puente sin métodos nuevos. Copiar resumen y Repetir son solo frontend; la velocidad a internet vuelve a pedir el consentimiento en cada Repetir.
- Spec 008 T2, tokens ajustados por AA (4,5:1; el test `TestTokenContrastAA` mide cada fg sobre los 6 fondos y sobre su tinte): claro `--text-muted` #64748b → #5b6a80 (4,08 → ≥4,5 sobre bg-subtle), `--accent-primary` #0284c7 → #0270a8 (3,51), `--accent-primary-hover` #0369a1 → #025a87, `--ok-fg` #059669 → #047857 (3,23), `--warn-fg` #b45309 → #a94d08 (4,30), `--err-fg` #dc2626 → #cc2222 (4,14); oscuro `--err-fg` #ef4444 → #f26060 (3,99). Añadido `--text-on-accent` (claro #fff, oscuro #06141f: el blanco sobre #0ea5e9 daba 2,77). Resto de tokens sin cambios. Series de gráfica no son texto y no se ajustan. Fuentes: solo Segoe UI Variable / Cascadia Mono (sin Inter/JetBrains).
- Spec 008 T14: el modal de borrado también bloquea Escape mientras el borrado está en curso (como la exportación), para no cerrar el modal con la operación a medias.
- Flakies CI (corregidos, sin relación con la spec 008): (1) `TestCappedShortIsIncomplete/upload`, fallo del servidor de test: el plazo de red saltaba antes que `rctx.Err()`, el servidor NoClose se iba y cerraba la conexión (EOF/reset); ahora detecta el plazo también por `DeadlineExceeded`/hora. (2) `TestGiantAndInvalidJSON/upload#01`, cliente sin CPU que no llega a enviar nada en el plazo: el producto daba una subida completa de 0 Mbps; ahora es incompleta (`ReasonNoData`) y el test lo acepta con `t.Logf`. La causa NO se reprodujo (25 repeticiones con estrés); el "bad certificate" del log es de `TestRunTLSRejected` (intencionado), no de este test. Hipótesis sin demostrar: escritura que no llegó a enviar nada (bloqueo/plazo) con 2,155 s de reloj; si reaparece, el `t.Logf` da el resultado completo. (3) `TestHistoryConcurrentStateAndToggle`, fallo del producto (no de history_id): `App.cfg()` leía config.json sin `cfgMu` mientras `update` lo reemplazaba (en Windows la lectura falla), devolvía la config vacía y el host manual desaparecía; ahora `cfg()` lee bajo `cfgMu`.

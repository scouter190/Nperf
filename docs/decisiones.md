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
- Tema tipo **MikroTik / WinBox**: grises con acento azul, colores suaves.
- Claro/oscuro **automático** según el sistema. Idioma **español**.
- **Compacta** por defecto, con modo **detallado**.
- Barra superior fija: **[Sentido ▾] [Duración ▾] [TEST!]**.
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
- **Solo en vivo**; botón para **exportar la sesión** en CSV o JSON.
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

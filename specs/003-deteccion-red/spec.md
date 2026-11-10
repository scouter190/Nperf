# 003 - Detección de red y cuello de botella

> Aprobada por el usuario.

## Objetivo
Saber por qué una prueba da lo que da: detectar las tarjetas de red de cada equipo (cable, WiFi y sus datos), mostrar el tipo de conexión de cada equipo en la lista y, tras TEST!, comparar el throughput medido con las velocidades de enlace de ambos extremos para señalar con un semáforo el tramo que limita.

## Alcance

### Incluye
- **Inventario local de tarjetas** (sin administrador, `GetAdaptersAddresses`): nombre, descripción (fabricante/modelo), MAC, IPv4, estado (conectada / desconectada), tipo **Ethernet / WiFi / Otro** (virtual, VPN, túnel, celular; el loopback no se lista) y velocidad de enlace negociada de transmisión y recepción.
- **Datos WiFi** (`wlanapi.dll`: `WlanQueryInterface`, `WlanGetNetworkBssList`, sin privilegios): SSID, BSSID, banda 2.4 / 5 / 6 GHz (derivada de la frecuencia), canal, señal en % y RSSI en dBm, velocidad negociada (tx/rx) y estándar 802.11 (n/ac/ax/be, con nombre WiFi 4/5/6/6E/7). Lo que Windows no entregue se muestra como **"no disponible"** con el motivo (servicio WLAN detenido, permiso de ubicación denegado, sin conexión), nunca como 0.
- **Permiso de ubicación** (Windows 11 24H2+): si Windows niega los datos WiFi por falta de permiso, el panel muestra un aviso con un botón que abre Configuración › Privacidad › Ubicación (`ms-settings:privacy-location`).
- **Ethernet**: velocidad negociada (10M / 100M / 1G / 2.5G / 5G / 10G…).
- **Interfaz de la prueba**: cada extremo identifica la tarjeta usada por la dirección local de la conexión de control.
- **Enlace en el anuncio (público)**: el beacon de cada interfaz incluye su tipo (`eth` / `wifi` / `other`), su velocidad de enlace tx/rx y, si es WiFi conectada, el SSID (≤ 32 bytes; se descarta si tiene caracteres de control o excede el tamaño). Anuncios sin esos campos (versiones previas) → "desconocido". El anuncio sigue ≤ 512 B. BSSID, MAC, banda, canal, señal y estándar no son públicos.
- **Resumen de enlace por canal autenticado**: tras el handshake HMAC, cliente y servidor intercambian el resumen de su interfaz de la prueba: tipo, velocidades tx/rx, y en WiFi SSID, banda, estándar y señal %. **Nunca** BSSID ni MAC. Un par que no lo envíe no rompe la prueba: el diagnóstico queda "parcial".
- **Semáforo y cuello de botella** (TCP, por sentido, subida y bajada por separado):
  - Enlace nominal del sentido = mín(velocidad tx del emisor, velocidad rx del receptor).
  - % = throughput TCP / nominal. Si el tramo limitante es cable: verde > 80 %, ámbar 50–80 %, rojo < 50 %. Si es WiFi: verde > 50 %, ámbar 30–50 %, rojo < 30 % (siempre se muestra el % sobre la velocidad PHY). Throughput > nominal → 100 % con nota.
  - Tramo limitante: el extremo de menor velocidad (empate → "ambos extremos"). Texto, p. ej.: "Limitado por el WiFi de B (enlace 144 Mbps, señal 42 %)"; en ámbar/rojo añade "por debajo del enlace más lento: posible cuello en la red intermedia, la señal WiFi o el equipo".
  - Sin datos de un extremo: se compara solo con el conocido y se marca "parcial"; sin datos de ninguno: sin semáforo.
- **Interfaz**: en la lista de equipos, icono cable / WiFi / otro / desconocido, velocidad de enlace y SSID (siempre como texto); panel **"Tarjetas de red"** de Este equipo (compacto: tipo, velocidad, y en WiFi SSID, banda, canal, señal; detallado: todo lo anterior), refrescado cada 5 s mientras está visible; semáforo y texto del cuello en el panel de detalle del resultado, con el resumen de enlace de ambos extremos.
- **CLI**: nuevo comando `nperf adapters [-json]` que lista las tarjetas locales con los mismos datos del panel (ayuda y mensajes en español, códigos de salida de la spec 001; `-json` con `format_version`, null + `status` para "no disponible").

### Excluye (specs posteriores)
Semáforo o datos de enlace en `nperf client` (salida y JSON v1 sin cambios), dúplex Ethernet, routers/APs sin agente (ping, traceroute, ARP, gateway), mapa gráfico, gráficas en vivo, pruebas programadas, alertas y notificaciones, exportación, contadores de errores/descartes/uso, Wake-on-LAN, ahorro de energía, ancho de canal, ruido, versión de driver, IPv6, cambiar la configuración de las tarjetas, inventario completo de tarjetas de equipos remotos.

## Criterios de aceptación

### Verificables en CI
1. Con datos de adaptadores simulados (inyectados), se clasifica Ethernet / WiFi / Otro por tipo de interfaz; el loopback se omite; desconectadas se marcan como tales; velocidades en bps se formatean (100 Mbps, 1 Gbps, 2.5 Gbps).
2. Tablas de conversión: frecuencia → banda y canal (2412–2484 MHz → 2.4 GHz, 5160–5885 → 5 GHz, 5925–7125 → 6 GHz) y tipo PHY → estándar (HT → 802.11n WiFi 4, VHT → ac WiFi 5, HE → ax WiFi 6/6E según banda, EHT → be WiFi 7); valores desconocidos → "no disponible".
3. Errores simulados de WlanAPI (servicio detenido, acceso denegado por ubicación, sin conexión) producen "no disponible" con el motivo correcto, sin error fatal; el acceso denegado activa el aviso de ubicación; el tipo y la velocidad de `GetAdaptersAddresses` se mantienen.
4. En windows-latest, sin administrador: el inventario real devuelve ≥ 1 tarjeta sin error y la consulta WiFi termina sin error aunque no haya WLAN.
5. Beacon: ida y vuelta con tipo, velocidad tx/rx y SSID; un beacon sin esos campos se parsea como "desconocido"; un SSID > 32 bytes o con caracteres de control se descarta (al emitir y al recibir); con alias y SSID de longitud máxima y velocidades máximas el tamaño es ≤ 512 B; test sobre los bytes emitidos: sin BSSID, MAC, banda, canal, señal ni estándar.
6. Resumen de enlace: se intercambia tras el handshake en ambos sentidos; no contiene BSSID ni MAC (test sobre los bytes); antes del handshake no se envía; un par que no lo envía completa la prueba con diagnóstico "parcial".
7. Selección de interfaz: dada la dirección local de una conexión, se elige la tarjeta que la tiene; sin coincidencia → "desconocida".
8. Semáforo (tabla de casos): límites exactos de cable (50 %, 80 %) y de WiFi (30 %, 50 %), nominal = mín(tx emisor, rx receptor), empate, throughput > nominal, un extremo sin datos ("parcial"), ninguno (sin semáforo), "no medido" en un sentido (sin semáforo en ese sentido); textos en español.
9. Backend de la interfaz: `State()` incluye tipo, velocidad y SSID de cada equipo, y el resultado incluye semáforo, % y texto por sentido; la lista de tarjetas locales nunca expone la clave; el test existente que prohíbe innerHTML cubre el SSID (se muestra solo como texto).
10. `nperf adapters` con datos simulados: salida legible en español y `-json` con `format_version` y null + `status` para "no disponible"; código 0 si hay inventario, 1 si falla; flag desconocido → 2.
11. Los tests de CLI y e2e de las specs 001 y 002 pasan sin cambios (salida de `nperf client` igual); `go vet` y `go test ./...` pasan en windows-latest.

### Validación manual en Windows 10/11
12. En un portátil por WiFi, el panel y `nperf adapters` muestran SSID, banda, canal, señal % (±5 puntos) y velocidad iguales a `netsh wlan show interfaces`; en un equipo por cable, la velocidad coincide con "Estado de Ethernet" de Windows.
13. En Windows 11 24H2+ con la ubicación desactivada para apps, el panel muestra el aviso y su botón abre la página de Ubicación de Configuración; el resto de datos (tipo, velocidad) sigue visible.
14. La lista de otro PC muestra en ≤ 10 s el icono (cable / WiFi), la velocidad de enlace y el SSID correctos de cada equipo.
15. TEST! entre un PC por cable a 1 Gbps y un portátil por WiFi muestra por sentido el semáforo y "Limitado por el WiFi de <alias>" con su velocidad de enlace; entre dos PCs a 1 Gbps por cable sale verde.
16. Todo funciona sin administrador; la app sigue arrancando y midiendo con el servicio WLAN detenido.

## Riesgos
- **Windows 11 24H2+**: la WlanAPI exige permiso de ubicación para SSID/BSSID (y puede negar toda la consulta): se degrada a "no disponible" con aviso (criterio 13). Sin permiso, el anuncio va sin SSID.
- **Velocidad WiFi variable**: la velocidad PHY cambia segundo a segundo (también la del anuncio, que se actualiza cada 3 s); por eso hay umbrales WiFi propios. Para el semáforo se toma la velocidad al inicio de la prueba.
- **Datos públicos (aceptado por el usuario)**: cualquiera en la LAN ve el tipo, la velocidad de enlace y el SSID de cada equipo con NPerf abierto. BSSID y MAC no se comparten.
- **SSID hostil**: un SSID ajeno puede traer texto malicioso; se valida (≤ 32 bytes, sin control) y se muestra solo como texto.
- **Velocidades engañosas**: adaptadores virtuales (Hyper-V, VPN, WSL) informan velocidades ficticias; la interfaz de la prueba puede ser virtual → "Otro" sin semáforo fiable.
- **Cuello invisible**: switches, AP y cables intermedios no se ven; el texto lo indica como "posible cuello en la red intermedia".
- **Compatibilidad**: el nuevo mensaje del canal de control debe ser opcional para no romper la versión mayor 1.

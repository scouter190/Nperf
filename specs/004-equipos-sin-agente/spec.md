# 004 - Equipos sin NPerf (ping, traceroute, MTU, puertos)

> Aprobada por el usuario. Decisiones en `docs/decisiones.md` §19.

## Objetivo
Diagnosticar equipos de la red que no tienen NPerf instalado (router, AP, switch gestionable, impresora, servidor): saber si responden, con qué latencia, jitter y pérdida, por qué saltos pasa el tráfico y dónde se degrada, qué MTU admite la ruta y qué servicios TCP exponen; y relacionarlo con el cuello de botella cuando tenga sentido (p. ej. pérdida en el tramo WiFi). Desarrolla §11 de `docs/decisiones.md`.

## Alcance

### Incluye
- **Motor ICMP sin administrador**: `IcmpCreateFile` + `IcmpSendEcho2` de `iphlpapi.dll` (LazyDLL, solo `_windows.go`); TTL y DF con `IP_OPTION_INFORMATION`. Nada de sockets raw. Lógica (estadísticas, búsqueda, análisis) portable con fuente inyectable, como `internal/netinfo`. Solo IPv4.
- **Destino**: IPv4 o nombre (≤ 253 caracteres, sin esquema ni puerto); un nombre se resuelve a la primera IPv4 y se muestra la IP usada. Rechazados: direcciones de red/broadcast de la subred local, multicast, 0.0.0.0, CIDR o rangos.
- **Ping**: N ecos (default 10, rango 1–100), intervalo ≥ 200 ms (default 500 ms), timeout por eco 1 s (rango 0.2–5 s), carga 32 B. Resultado: enviados, recibidos, % pérdida, RTT mín/media/máx y jitter (media de |RTT_i − RTT_i−1| entre respuestas consecutivas). Respuestas de error (destino inalcanzable, TTL agotado) cuentan como pérdida con el motivo. Windows mide en ms enteros: "< 1 ms" se muestra como tal.
- **Traceroute**: TTL 1..máx. (default 30, rango 1–64), 3 sondas por salto, timeout 1 s, un salto a la vez; se detiene al llegar al destino. Por salto: IP que responde (o "*"), latencia de cada sonda, media y % pérdida. Sin DNS inverso. **Análisis**:
  - Pérdida en un salto intermedio que no se mantiene en los siguientes → "el equipo limita respuestas ICMP (no es pérdida real)".
  - Pérdida ≥ 20 % que se mantiene hasta el destino → "pérdida desde el salto N".
  - Latencia media ≥ la del salto anterior + 20 ms y que se mantiene en los siguientes → "la latencia sube en el salto N".
  - Sin llegar al destino → "el destino no responde o filtra ICMP a partir del salto N".
- **MTU de la ruta**: ecos con DF y búsqueda binaria del mayor paquete IP que pasa sin fragmentar, entre 576 B y el MTU de la interfaz local de salida (o 1500 si no se conoce); 2 intentos por tamaño, tope 20 sondas. Se informa el MTU de la ruta (bytes IP) y la carga ICMP máxima (MTU − 28). Si el destino no responde a ping de 576 B → "no disponible: el destino no responde a ICMP". Si solo hay timeouts sin "fragmentación necesaria" se anota "posible agujero negro de MTU".
- **Puertos TCP**: conexión TCP normal (sin SYN scan), timeout 1.5 s, máx. 4 conexiones simultáneas, cada conexión se cierra al conectar sin enviar datos. Estados: **abierto** (conecta), **cerrado** (rechazo/RST), **filtrado** (timeout). Lista por defecto 21, 22, 23, 53, 80, 443, 445, 3389, 8080, 8291 (Winbox); lista propia por `-p 22,80,8000-8010`, máx. 32 puertos por ejecución.
- **No agresivo**: un solo diagnóstico en curso por instancia (GUI o comando CLI), cancelable; sin barridos de subred ni equipos propuestos desde la tabla ARP (spec posterior); todo límite se valida antes de enviar nada (error de uso).
- **No disponible con motivo**: si iphlpapi falla, el host no resuelve o la respuesta no es medible, se informa "no disponible" + motivo, nunca 0.
- **Interfaz**:
  - Añadir destino manual (IP o nombre, alias opcional) a la lista de equipos, marcado **"sin NPerf"**; quitarlo. Se guardan en `config.json` (máx. 64; campo nuevo, los desconocidos se conservan) y se recuperan al reiniciar.
  - **Puerta de enlace** de la ruta por defecto IPv4, detectada sin administrador, añadida siempre como "Puerta de enlace (sin NPerf)" (no se guarda ni se puede quitar; se actualiza si cambia la red).
  - Se muestran en la misma lista, en un grupo **"Sin NPerf"** debajo de los equipos con NPerf, con icono propio.
  - Los equipos "sin NPerf" no permiten TEST!; su panel tiene botones **Ping**, **Traceroute**, **MTU** y **Puertos** con resultado en el panel de detalle (también disponibles para equipos con NPerf). Durante el diagnóstico el botón pasa a "Cancelar".
  - **Diagnóstico combinado** (texto): si el salto 1 (o el destino, si es el gateway/AP) tiene pérdida ≥ 5 % o latencia media > 20 ms y la interfaz local de salida es WiFi → "pérdida/latencia en el tramo WiFi de este equipo (señal X %)"; si es cable → "problema en el primer tramo (cable, switch o gateway)"; MTU < 1500 → "MTU reducido en la ruta (VPN/PPPoE/túnel): posibles cortes con paquetes grandes"; ping con pérdida 0 y RTT bajo → "enlace sano hasta <destino>".
  - Puente: funciones nuevas en la lista blanca, con tipos y límites validados en Go; datos de la red (nombres, IPs) solo como texto.
- **CLI**: `nperf ping <host>` (`-count`, `-interval`, `-timeout`), `nperf trace <host>` (`-max-hops`, `-timeout`), `nperf mtu <host>`, `nperf ports <host>` (`-p`, `-timeout`); todos con `-json`. Ayuda y mensajes en español; códigos: 0 diagnóstico completado, 1 no se pudo hacer (no resuelve, API ICMP falla, ping con 100 % de pérdida), 2 error de uso. `-json`: un objeto por stdout con `format_version: 1` y `command`, null + `status` para "no disponible", unidades en nombres de campo (`_ms`, `_pct`, `_bytes`).

### Excluye
SNMP, LLDP, mapa de red, gráficas en vivo, pruebas programadas, alertas y notificaciones, exportación, IPv6, DNS inverso en traceroute, puertos UDP, detección de servicios/banners, listado de conexiones TCP/UDP locales, prueba hacia internet, barridos de subred o rangos de IPs, equipos propuestos desde la tabla ARP (spec posterior), cambios en el protocolo NPerf o en el anuncio.

## Criterios de aceptación

### Verificables en CI
1. Ping con respuestas simuladas: pérdida, mín/media/máx y jitter correctos (tabla de casos: todo responde, pérdida parcial, 100 % pérdida, una sola respuesta → jitter "no disponible", RTT 0 → "< 1 ms"); errores ICMP (inalcanzable, TTL agotado) cuentan como pérdida con motivo.
2. Traceroute simulado: se detiene al llegar al destino o al máx. de saltos; "*" por sonda sin respuesta; los cuatro textos del análisis se generan en sus casos y no en sus contraejemplos (pérdida intermedia que no persiste, subida de latencia que no persiste, límite exacto de 20 % y 20 ms).
3. MTU simulado: la búsqueda binaria halla el MTU exacto (p. ej. 1500, 1492, 1400, 576) en ≤ 20 sondas; destino mudo → "no disponible" con motivo; solo timeouts → nota de agujero negro.
4. Puertos contra servidores locales en loopback: abierto, cerrado y filtrado (simulado con un dialer inyectado) se clasifican bien; nunca más de 4 conexiones simultáneas (test con contador); no se envían datos; la lista `-p` acepta comas y rangos y rechaza > 32 puertos, 0 y > 65535.
5. Validación de destino y límites: CIDR, rangos, broadcast, multicast, 0.0.0.0, esquemas/URLs y valores fuera de rango se rechazan antes de enviar ningún paquete (fuente simulada sin llamadas).
6. Ritmo: con reloj inyectado, el ping respeta el intervalo mínimo, el traceroute envía una sonda a la vez y la cancelación detiene el envío en ≤ 1 s.
7. En windows-latest, sin administrador: ping, trace (máx. 3 saltos) y mtu contra 127.0.0.1 terminan sin error usando `IcmpSendEcho2`.
8. CLI con fuente simulada: salida legible en español y `-json` con `format_version`, `command` y null + `status`; códigos 0/1/2 según la regla; flag desconocido → 2.
9. Backend de la interfaz: añadir/quitar destino manual (validado, alias saneado, el 65.º se rechaza), se guarda en `config.json` y se recupera al reiniciar; con una ruta por defecto simulada aparece la puerta de enlace (no se puede quitar); `State()` los pone en el grupo "sin NPerf" con `can_test` falso; solo un diagnóstico a la vez (el segundo se rechaza con mensaje); diagnóstico combinado por tabla de casos (WiFi/cable/MTU/sano); la clave nunca aparece; el test que prohíbe innerHTML sigue pasando.
10. Los tests de CLI, e2e y GUI de las specs 001–003 pasan sin cambios; `go vet` y `go test ./...` pasan en windows-latest; `go.mod` sin dependencias nuevas.

### Validación manual en Windows 10/11
11. `nperf ping <router>` da pérdida y RTT coherentes con `ping -n 10 <router>` (media ±2 ms).
12. `nperf trace 8.8.8.8` muestra los mismos saltos que `tracert -d 8.8.8.8`.
13. `nperf mtu` da 1500 en una LAN Ethernet y el valor reducido tras una VPN/PPPoE, coincidiendo con `ping -f -l <tamaño>`.
14. `nperf ports <router>` marca abiertos los puertos de administración del router (p. ej. 80/443 o 8291 en MikroTik) y el resto cerrado o filtrado, coherente con lo que se ve desde un navegador o Winbox.
15. En la ventana: añadir un destino por IP y por nombre, verlo como "sin NPerf", lanzar los cuatro diagnósticos y cancelar uno a mitad; con el portátil por WiFi lejos del AP, el diagnóstico combinado señala el tramo WiFi.
16. Todo funciona sin administrador y con el firewall de Windows por defecto.

## Riesgos
- **ICMP filtrado**: muchos equipos (firewall de Windows, impresoras) no responden a ping o limitan ICMP; se informa como "no responde a ICMP", no como caída.
- **Resolución de 1 ms** de `IcmpSendEcho2`: jitter y RTT en LAN pueden dar 0; se muestra "< 1 ms".
- **Pérdida aparente en saltos intermedios** por rate-limit ICMP de los routers: el análisis solo cuenta la pérdida que persiste hasta el destino.
- **Puertos y antivirus/IDS**: aunque es una conexión normal y limitada, un IDS podría marcarlo como escaneo; por eso lista corta, tope 32 y sin rangos de IPs.
- **MTU**: con agujeros negros (ICMP "fragmentation needed" filtrado) el resultado es una cota inferior.

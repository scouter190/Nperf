# 002 - Interfaz y descubrimiento

## Objetivo
Llevar el medidor de la spec 001 a una ventana de escritorio: ver los equipos con NPerf abierto en la LAN, elegir uno como destino y medir con TEST! desde este equipo, sin usar la consola. Es la base para el mapa, el diagnóstico y las pruebas programadas.

## Alcance

### Incluye
- **Dos ejecutables con el mismo código**: `nperf.exe` (CLI de consola, igual que en la spec 001: sin argumentos muestra la ayuda) y `nperf-gui.exe` (subsistema Windows GUI: se abre con doble clic, sin consola). Cada uno es un único archivo.
- **Ventana WebView2** con la interfaz HTML/CSS/JS embebida en `nperf-gui.exe`. Puente Go ↔ WebView2 con una única dependencia Go pura (sin cgo), elegida en el plan tras revisar licencia y mantenimiento. Reemplaza a Walk.
- **Server siempre activo**: con la app abierta (ventana o bandeja), el equipo es server de medición en el puerto configurado (5300 por defecto), con la clave de `config.json` o la que se fije en la app (se guarda en `config.json`). Una sola instancia de la app por usuario; una segunda apertura avisa y sale.
- **Bandeja del sistema**: cerrar la ventana deja NPerf en la bandeja (sigue como server y anunciándose). Menú: Abrir, Salir. Solo "Salir" termina el proceso.
- **Anuncio UDP broadcast** cada 3 s en cada interfaz IPv4 activa (a su dirección de broadcast), con: alias, IP de esa interfaz, puerto de medición, estado (disponible/ocupado) y cola. Público en la LAN, como `nperf status`: sin clave, versión exacta, ids de sesión ni detalles de la prueba.
- **Puerto de descubrimiento propio: 5301/UDP** (configurable), separado de los datagramas de medición del 5300/UDP.
- **Subredes extra a mano**: el usuario agrega una IP o un rango CIDR (máx. /24). La app consulta esas direcciones por unicast en 5301/UDP y los NPerf que estén ahí responden con su anuncio.
- **Lista de equipos**: alias + IP, estado (disponible / ocupado / en cola N) y, aparte, "Este equipo". Un equipo sin anunciarse durante 10 s pasa a **gris (no seleccionable)**; vuelve a la normalidad si reaparece.
- **Origen fijo**: la prueba sale siempre de "Este equipo"; el usuario elige solo el **destino** en la lista. El selector de origen se muestra fijo para habilitarlo en una spec futura.
- **Alias**: configurable en la app y guardado en `config.json`; si está vacío se usa el nombre del equipo de Windows.
- **Barra superior fija**: [Sentido ▾] [Duración ▾] [TEST!].
  - Sentido: "Este equipo → Destino", "Destino → Este equipo", "Ambos" (default), con los nombres de los equipos.
  - Duración: combo editable 5, 10 (default), 30, 60, 120, 300 s; rango 3–300; recuerda la última (en `config.json`). En "Ambos" muestra el total.
  - TEST!: habilitado solo con un destino válido (no gris, no "Este equipo"); durante la prueba o la cola pasa a "Cancelar".
- **Prueba**: usa el cliente de la spec 001 (`-proto both`, 4 flujos, UDP derivado). Progreso por fase (RTT, TCP, UDP) y "en cola, posición N".
- **Panel de detalle**: RTT (mín/media/máx), TCP y UDP por sentido (throughput, jitter, pérdida, retransmisiones), con "no medido" / "no disponible"; errores con el mismo texto que la CLI (clave incorrecta, cola llena, versión incompatible, tiempo agotado).
- **Tema** tipo MikroTik/WinBox (grises, acento azul, colores suaves), claro/oscuro automático según Windows, español, densidad compacta con modo detallado (el detallado muestra además bytes, datagramas y notas).
- **Sin equipos**: si en 15 s no aparece ninguno, sugerencia de revisar el firewall de Windows para 5300 TCP/UDP y 5301 UDP.
- **Compilación**: CI y `scripts/build-windows.*` generan `nperf.exe` y `nperf-gui.exe`; el CI publica ambos como artefactos.

### Excluye (specs posteriores)
Control remoto A → B desde un tercer equipo (spec futura, junto al mapa), mapa de topología gráfico, detección de adaptadores/WiFi, tipo de conexión y versión por nodo, semáforo y cuellos de botella, gráficas en vivo, pruebas programadas, alertas y notificaciones, exportación CSV/JSON desde la app, aviso guiado de firewall (más allá de la sugerencia), arranque con Windows, IPv6, routers/APs sin agente, cambios en la CLI.

## Criterios de aceptación

### Verificables en CI (sin ventana)
1. El anuncio se serializa y parsea en ida y vuelta con alias, IP, puerto, estado y cola; mensajes con otra firma, truncados o de más de 512 bytes se descartan sin error fatal.
2. El anuncio no contiene clave, versión exacta, ids de sesión ni detalles de la prueba (test sobre los bytes emitidos).
3. Dos instancias en loopback/puertos de test se descubren en ≤ 2 ciclos; con el server ocupado, el anuncio refleja "ocupado" y la cola en ≤ 1 ciclo.
4. Con reloj inyectado: un equipo sin anunciarse 10 s pasa a "inalcanzable"; si reaparece vuelve a "disponible"/"ocupado".
5. Con varias interfaces simuladas, se emite un anuncio por interfaz con la IP propia de cada una; el mismo equipo visto por dos interfaces aparece una sola vez.
6. Subredes extra: un CIDR > /24 o inválido se rechaza con mensaje; una consulta unicast a un NPerf de test recibe su anuncio.
7. El backend de la interfaz (API Go que llama la UI) se prueba sin WebView2: lista de equipos con "Este equipo" aparte, validación de duración 3–300, sentido, alias por defecto = nombre del equipo, persistencia de alias/duración/clave en `config.json` (directorio inyectado).
8. Selección de destino: el backend rechaza iniciar TEST! sin destino, con un destino inalcanzable o con "Este equipo" como destino; el origen es siempre el equipo local.
9. TEST! desde el backend contra un server de test produce el mismo resultado que `client.Run` (mismos campos y "no medido"), informa la posición en cola y "Cancelar" saca de la cola.
10. Con la app en marcha, `nperf client 127.0.0.1` con la misma clave completa una medición (el server está activo); con el puerto ocupado la app informa el error.
11. Una segunda instancia de la app para el mismo usuario detecta la primera y sale (test con nombre de bloqueo inyectado).
12. `go vet` y `go test ./...` pasan en windows-latest; el CI compila y publica como artefactos `nperf.exe` (subsistema consola) y `nperf-gui.exe` (subsistema Windows GUI, verificado en la cabecera PE), cada uno un único archivo con lo necesario embebido.
13. `nperf.exe` conserva el comportamiento de la spec 001: los tests de CLI y e2e existentes pasan sin cambios.

### Validación manual en Windows 10/11
14. Doble clic en `nperf-gui.exe` abre la ventana en < 2 s, sin consola, en español, con el tema claro u oscuro de Windows; al cambiar el tema de Windows la ventana lo sigue.
15. Dos PCs en la misma LAN se ven entre sí en ≤ 10 s; al cerrar NPerf en una, en la otra pasa a gris en ≤ 15 s.
16. TEST! desde una hacia la otra muestra RTT, TCP y UDP por sentido en el panel de detalle; con un tercer equipo midiendo al mismo destino, se ve "en cola, posición 1" y "Cancelar" sale de la cola.
17. Cerrar la ventana deja el icono en la bandeja y el equipo sigue visible y medible desde otro; "Salir" lo quita de la lista de los demás en ≤ 15 s. Abrir `nperf-gui.exe` otra vez avisa y no crea una segunda instancia.
18. Sin WebView2 (o deshabilitado por política) `nperf-gui.exe` muestra un mensaje nativo con el enlace de descarga del runtime y `nperf.exe` sigue funcionando.
19. Se ejecuta sin administrador; con el firewall bloqueando 5301 aparece en 15 s la sugerencia de revisar el firewall.

## Riesgos
- **WebView2 ausente o bloqueado por política** (Windows 10 LTSC, equipos corporativos): mensaje claro y CLI como alternativa (criterio 18).
- **Firewall que bloquea broadcast o 5301/UDP**; un usuario sin admin quizá no pueda aceptar el aviso de Windows: sugerencia en la app; el aviso guiado completo queda para después.
- **Varias interfaces** (VPN, Hyper-V, WSL, WiFi + cable): anuncio por interfaz y deduplicación; las interfaces virtuales pueden mostrar IPs inalcanzables.
- **Anuncio sin autenticar**: cualquiera en la LAN puede falsificar uno; el daño se limita a una entrada falsa, porque la prueba exige la autenticación HMAC de la spec 001.
- **Dependencia del puente WebView2**: pocas opciones puras en Go y con mantenimiento irregular; si ninguna pasa la revisión, se consulta al usuario antes de seguir.
- **Binario GUI sin consola**: los errores de arranque no se ven en una terminal; deben mostrarse en un mensaje nativo.

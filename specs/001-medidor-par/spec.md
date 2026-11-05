# 001 - Medidor en par

## Objetivo
Medir el enlace entre dos equipos con NPerf (A ↔ B) usando el protocolo propio, por CLI, con resultados comparables a iperf3. Es el núcleo que después usarán la GUI, el mapa y las pruebas programadas.

## Alcance

### Incluye
- Un solo `.exe` con dos modos CLI: `nperf server` (lado B, queda esperando) y `nperf client <host>` (lado A, inicia la prueba). Comandos y flags en inglés; mensajes al usuario en español.
- Canal de control TCP autenticado con clave compartida; la clave nunca viaja en claro.
- Compatibilidad por versión mayor: si no coincide, se rechaza con mensaje claro en ambos lados.
- Negociación de sentido, duración, flujos, tasa UDP y protocolo.
- Protocolo `-proto tcp|udp|both` (default `both`: TCP y luego UDP).
- Sentido `-direction fwd|rev|both`: `fwd` (A → B), `rev` (B → A), `both` (default). El sentido no pedido se reporta como "no medido".
- Duración `-duration` 3–300 s, default 10; en `both` aplica por sentido.
- TCP con `-streams` N flujos paralelos (default 4).
- UDP a tasa fija `-udp-rate`; por defecto 90 % del throughput TCP medido en ese sentido.
- RTT (ping por el canal de control) antes de la carga.
- Métricas: throughput (Mbps), jitter y pérdida (UDP), retransmisiones (TCP, si Windows las entrega sin admin; si no, "no disponible").
- Cola por concurrencia: si B ya está en prueba, la nueva espera en cola (máximo 5 en espera) y A recibe aviso con su posición; A puede cancelar con Ctrl+C.
- Consulta de estado pública en la LAN `nperf status <host>`: "disponible" u "ocupado" con el número en cola, sin clave y sin lanzar prueba (base para el indicador visual de la spec de interfaz).
- Salida legible por defecto y `-json` para comparar y automatizar.
- Puerto 5300 por defecto, configurable con `-port`. Clave por `-key` o configuración local.

### Excluye (specs posteriores)
GUI y bandeja, mapa, descubrimiento automático, routers/APs sin agente, detección de adaptadores y WiFi, ejecución programada, alertas, exportación CSV, aviso guiado de firewall, compatibilidad de protocolo con iperf3.

## Criterios de aceptación
1. `nperf server` y `nperf client 127.0.0.1` completan una prueba con valores por defecto y terminan con código 0.
2. Con clave distinta, la prueba se rechaza antes de la carga, con código ≠ 0 y mensaje "clave incorrecta"; una captura del canal de control no contiene la clave en claro.
3. Con versión mayor distinta (simulada en test), ambos lados muestran aviso de incompatibilidad y no se mide.
4. Por defecto el resultado trae, en este orden: RTT, TCP fwd, TCP rev, UDP fwd, UDP rev.
5. `-direction fwd` reporta el sentido rev como "no medido" (y viceversa).
6. `-duration 5` dura 5 s ±0,5 s por sentido y protocolo; valores fuera de 3–300 se rechazan antes de conectar.
7. `-streams 8` abre 8 conexiones TCP de datos (verificable en test o con `netstat`); el resultado suma los flujos.
8. Sin `-udp-rate`, la tasa UDP enviada es 90 % ±5 % del throughput TCP del mismo sentido; con `-udp-rate 50M` se envía 50 Mbps ±5 %. Con `-proto udp` sin `-udp-rate` (sin TCP previo) se envía 100 Mbps ±5 %.
9. UDP reporta jitter (ms) y pérdida (%); con pérdida inducida en test, la pérdida reportada coincide ±1 punto.
10. Con B ocupado, los siguientes `client` reciben "en cola (posición N)" (N = 1..5) y se ejecutan en orden al terminar el anterior; el 6.º se rechaza al instante con "cola llena" y código ≠ 0. Ctrl+C en un cliente en cola lo saca de la cola, termina con código ≠ 0 y los demás avanzan de posición.
11. `-json` produce un objeto válido con todas las métricas, sentidos y protocolos (incluidos "no medido"/"no disponible").
12. En la misma red cableada, el throughput TCP (4 flujos, 10 s) está dentro de ±10 % del de `iperf3 -P 4 -t 10`, en ambos sentidos (`-R`).
13. Funciona sin privilegios de administrador. `go vet` y `go test ./...` pasan.
14. `nperf status <host>` (acepta `-port` y `-json`; no usa `-key`) informa sin autenticación y sin lanzar prueba "disponible" u "ocupado (N en cola)" y termina con código 0. Límites: la respuesta solo contiene `state` (`available`/`busy`) y `queue` (N), sin ids de sesión, IPs, nombres, versión exacta ni detalles de la prueba en curso; es un mensaje propio atendido antes del handshake que no da acceso a ningún otro paso del protocolo sin autenticar; no ocupa turno ni entra en la cola; el server responde con plazo corto (< 1 s, aunque haya una prueba en curso) y cierra la conexión. Con `-json` produce un objeto válido con solo `state` y `queue`.

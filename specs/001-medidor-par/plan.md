# 001 - Plan

## Enfoque
- Go estándar + `golang.org/x/sys` (solo para retransmisiones). Sin otras dependencias.
- Todo el código de red es portable (`net`, `time`) y se prueba en Linux con loopback. Lo específico de Windows va en archivos `_windows.go` (build tags) con alternativa `_linux.go` / `_other.go`.
- Un solo puerto (5300) para TCP y UDP. La primera línea de cada conexión TCP dice si es de control o de datos (con id de sesión).
- Control: mensajes JSON por línea (`hello`, `auth`, `params`, `queued`, `start`, `result`, `error`).
- El cliente siempre abre las conexiones, también en `rev`: B no necesita llegar a A. En UDP `rev`, A manda primero un datagrama de saludo para que B conozca su dirección.
- Autenticación: desafío y respuesta HMAC-SHA256 con nonce; la clave no se transmite.
- Quien recibe mide el throughput y lo devuelve por el canal de control.
- UDP: cada datagrama lleva su secuencia y la hora de envío. Jitter según RFC 3550; pérdida por huecos de secuencia. El ritmo de envío se calcula por bytes adeudados según el tiempo transcurrido, no con `sleep` por paquete (el temporizador de Windows es de unos 15 ms).
- El paquete `meter` acepta cualquier `time.Duration`, así los tests corren en menos de 1 s. La CLI exige 3–300 s.
- Clave: `-key`, o si falta, `config.json` en `os.UserConfigDir()/NPerf`.

## Paquetes y archivos
- `cmd/nperf/main.go`: subcomandos `server` y `client`, flags, códigos de salida.
- `internal/version`: versión y regla de versión mayor.
- `internal/proto`: mensajes de control y codec JSON por línea.
- `internal/auth`: HMAC con desafío y respuesta.
- `internal/meter`: `tcp.go` (N flujos, conteo de bytes) y `udp.go` (ritmo de envío, jitter, pérdida).
- `internal/tcpinfo`: retransmisiones; `_windows.go` (SIO_TCP_INFO), `_linux.go` (TCP_INFO), `_other.go` ("no disponible").
- `internal/server`: escucha, cola y sesiones.
- `internal/client`: orquesta RTT → TCP → UDP por sentido.
- `internal/report`: salida en texto y JSON.
- `internal/config`: lectura de la clave local.
- `.github/workflows/ci.yml`, `docs/validacion-iperf3.md`.

## Riesgos
- **Precisión del ritmo UDP (±5 %) en Windows**, por la resolución del temporizador. Se prueba temprano (T2), pero en Windows solo cuando exista CI (T14) o a mano.
- SIO_TCP_INFO requiere Windows 10 1703+ y no se puede probar en el contenedor: si falla, se reporta "no disponible".
- Loopback no reproduce una red real; el ±10 % frente a iperf3 solo se valida a mano (T15).
- Los runners de GitHub corren como administrador: "sin admin" se verifica a mano.
- El módulo se llama `nperf` hasta confirmar el repositorio; si cambia, hay que renombrar los imports (cambio mecánico).

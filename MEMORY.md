# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T6 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T1-T6 y T14 hechas; T7-T13 y T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar).
- Repo en GitHub: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` en segundos enteros, `-proto tcp|udp`, `-udp-rate`, `-direction fwd|rev|both` (default both)), `internal/proto`, `internal/meter` (tcp.go, udp.go), `internal/server` (server.go, udp.go), `internal/client`.
- UDP integrado en la sesión: `-proto udp`, `-udp-rate` acepta k/M/G o bps (base 1000), 100 Mbps por defecto sin TCP previo.
- Saludo UDP `NPUDP1 <session>` en el puerto 5300: el server responde `ok`; solo el primer saludo asocia la dirección, los de otra dirección se ignoran.
- T6: el cliente siempre abre las conexiones; en rev mide el cliente. "No medido" con `Result{Fwd,Rev}` y `Measured`. `both` = dos sesiones (fwd y rev); sesión única pendiente en T10.
- `-proto both` sigue dando error claro (pendiente de T8).
- Tests con `-race`: integración TCP y UDP en loopback, errores, cancelación, saludo UDP, unitarios de proto y meter.
- Auth (T5): HMAC-SHA256 mutua (rol + ambos nonces + id de sesión firmado); la clave nunca viaja; id de sesión solo tras auth; deadline de handshake 10 s; aviso en stderr si no hay clave.
- Versión única en `internal/version` (`Current`), regla de versión mayor; aviso en ambos lados si difiere.
- Flag `-key` en server y client (la config local llega en T11).
- El rango 3-300 s de `-duration` se valida en T11.
- CI (`.github/workflows/ci.yml`) en verde: windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`.
- Finales de línea LF forzados con `.gitattributes` (gofmt en CI).
- Guía de entorno para Windows: `docs/entorno-local.md` (el usuario clona en Documentos).

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Notas
- T8: relajar el error de `-udp-rate` sin `-proto udp` en `cmd/nperf/main.go` (aceptarlo con `both`).
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.

## Próximo paso
`implementer` hace T7 (flujos paralelos `-streams`); luego `reviewer`.

## Pendientes
- T10: `both` en una sola sesión (Direction=both; server hace fwd y luego rev en el mismo turno).
- T11: acotar RateBps máximo del server en UDP rev (`sendUDPRev`, internal/server/udp.go).
- T12: retirar de `client.Result` los campos duplicados Sent/Received/Mbps/UDP (usar Fwd/Rev).
- Endurecimiento opcional en `auth.Sum`: prefijo de longitud por parte en vez de separador \0 (hoy no explotable).
- Confirmar Walk como librería de interfaz.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1, T2, T3, T4 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T5-T13 y T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar).
- Repo en GitHub: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` en segundos enteros, `-proto tcp|udp`, `-udp-rate`, `-direction fwd`), `internal/proto`, `internal/meter` (tcp.go, udp.go), `internal/server` (server.go, udp.go), `internal/client`.
- UDP integrado en la sesión (solo fwd): `-proto udp`, `-udp-rate` acepta k/M/G o bps (base 1000), 100 Mbps por defecto sin TCP previo.
- Saludo UDP `NPUDP1 <session>` en el puerto 5300: el server responde `ok`; solo el primer saludo asocia la dirección, los de otra dirección se ignoran.
- rev/both y `-proto both` dan error claro en español (T6 y T8).
- Tests con `-race`: integración TCP y UDP en loopback, errores, cancelación, saludo UDP, unitarios de proto y meter.
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
`implementer` hace T5 (autenticación HMAC y regla de versión; unifica `Version`); luego `reviewer`.

## Pendientes
- Confirmar Walk como librería de interfaz.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1 hecha y aprobada.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T1 hecha; T2-T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.24, solo librería estándar).
- Esqueleto TCP `fwd` de punta a punta: `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` en segundos enteros), `internal/proto`, `internal/meter/tcp.go`, `internal/server`, `internal/client`.
- Tests con `-race`: integración en loopback, errores, segunda prueba, cancelación, unitarios de proto y meter.
- El rango 3-300 s de `-duration` se valida en T11.
- CI escrito (`.github/workflows/ci.yml`): windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`. Pendiente de verificar en verde tras crear el repo (T14 sin marcar).
- Guía de entorno para Windows: `docs/entorno-local.md`.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Próximo paso
1. El usuario crea el repo vacío `scouter190/Nperf`; se sube el proyecto y se verifica el CI.
2. Clonar en Documentos según `docs/entorno-local.md`.
3. `implementer` hace T2 (medidor UDP, la más riesgosa: ritmo ±5 %); luego `reviewer`.

## Pendientes
- Crear repo `Nperf` en GitHub (lo crea el usuario) y subir el proyecto.
- Confirmar Walk como librería de interfaz.

## Bloqueos
- Esta sesión no puede crear repositorios en GitHub (repo aún no creado).

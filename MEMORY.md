# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 0 - Alcance definido.** Sin código todavía.

## Hecho
- Alcance y decisiones de v1 registrados en `docs/decisiones.md`.
- Estructura SDD creada: `AGENTS.md`, subagentes en `.claude/agents/`, carpeta `specs/`.

## Spec activa
Ninguna.

## Próximo paso
1. `planner` escribe `specs/001-medidor-par/spec.md` (medidor en par TCP/UDP, sentido y duración).
2. El usuario aprueba la spec.
3. `planner` escribe plan y tareas; `implementer` empieza por T1.

## Pendientes
- Confirmar repo en GitHub (propuesta `scouter190/nperf`).
- Confirmar Walk como librería de interfaz.
- Validar resultados contra iperf3 cuando exista el medidor.

## Bloqueos
Ninguno.

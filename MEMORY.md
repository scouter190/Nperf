# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 1 - Spec 001 completa, sin preguntas abiertas; esperando aprobación del usuario.** Sin código todavía.

## Hecho
- Alcance y decisiones de v1 en `docs/decisiones.md` (incluye sección 16 de la spec 001).
- Estructura SDD: `AGENTS.md`, subagentes en `.claude/agents/`, carpeta `specs/`.
- `specs/001-medidor-par/spec.md`: medidor en par por CLI (`escuchar` / `medir`), 13 criterios.

## Spec activa
`specs/001-medidor-par/` - solo `spec.md`. Plan y tareas tras la aprobación.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Próximo paso
1. El usuario aprueba la spec 001.
2. `planner` escribe `plan.md` y `tasks.md`; `implementer` empieza por T1.

## Pendientes
- Crear repo `Nperf` en GitHub (lo crea el usuario) y subir el proyecto.
- Confirmar Walk como librería de interfaz.

## Bloqueos
- Esta sesión no puede crear repositorios en GitHub.

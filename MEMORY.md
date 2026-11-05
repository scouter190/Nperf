# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 2 - Spec 001 aprobada; plan y 15 tareas escritos. Listo para implementar T1.** Sin código todavía.

## Hecho
- Alcance y decisiones de v1 en `docs/decisiones.md` (incluye sección 16 de la spec 001).
- Estructura SDD: `AGENTS.md`, subagentes en `.claude/agents/`, carpeta `specs/`.
- `specs/001-medidor-par/spec.md`: medidor en par por CLI (`escuchar` / `medir`), 13 criterios.

## Spec activa
`specs/001-medidor-par/`: spec aprobada, `plan.md`, `tasks.md` (T1–T15, 0 hechas).

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Próximo paso
1. `implementer` hace T1 (esqueleto de punta a punta); luego `reviewer`.
2. T14 (CI Windows) espera al repositorio en GitHub.

## Pendientes
- Crear repo `Nperf` en GitHub (lo crea el usuario) y subir el proyecto.
- Confirmar Walk como librería de interfaz.

## Bloqueos
- Esta sesión no puede crear repositorios en GitHub.

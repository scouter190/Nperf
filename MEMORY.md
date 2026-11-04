# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 1 - Spec 001 escrita, esperando aprobación del usuario.** Sin código todavía.

## Hecho
- Alcance y decisiones de v1 en `docs/decisiones.md` (incluye sección 16 de la spec 001).
- Estructura SDD: `AGENTS.md`, subagentes en `.claude/agents/`, carpeta `specs/`.
- `specs/001-medidor-par/spec.md`: medidor en par por CLI (`escuchar` / `medir`), 13 criterios.

## Spec activa
`specs/001-medidor-par/` - solo `spec.md`. Plan y tareas tras la aprobación.

## Preguntas abiertas (spec 001)
1. Puerto por defecto: propuesta 5300 (no 5201, para convivir con iperf3).
2. Tasa UDP con `-proto udp` sin TCP previo: propuesta 100 Mbps.
3. Cola: propuesta máximo 5 en espera; A cancela con Ctrl+C.
4. Comandos y flags en español (`escuchar`, `medir`, `-duracion`) o en inglés.
5. Tolerancia frente a iperf3: propuesta ±10 %.

## Próximo paso
1. El usuario responde las preguntas y aprueba la spec.
2. `planner` escribe `plan.md` y `tasks.md`; `implementer` empieza por T1.

## Pendientes
- Crear repo `Nperf` en GitHub (lo crea el usuario) y subir el proyecto.
- Confirmar Walk como librería de interfaz.

## Bloqueos
- Esta sesión no puede crear repositorios en GitHub.

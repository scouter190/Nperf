# AGENTS.md - NPerf

Reglas para todos los agentes. Metodología: **SDD ligero** (Spec-Driven Development) con consumo mínimo de tokens.

## Roles
| Agente | Hace | No hace |
|---|---|---|
| `coordinator` | Recibe el pedido, delega en los subagentes, informa al usuario | No lee código, no escribe archivos |
| `planner` | Escribe `spec.md`, `plan.md`, `tasks.md` | No escribe código |
| `implementer` | Implementa **una** tarea de `tasks.md` y sus tests | No cambia la spec ni el plan |
| `reviewer` | Revisa el diff contra la spec y las tareas | No corrige código; solo reporta |

Los subagentes no se llaman entre sí: todo pasa por el `coordinator`.

## Flujo SDD
1. **Spec** - `planner` crea `specs/NNN-nombre/spec.md`: qué y por qué, criterios de aceptación.
2. **Plan** - `planner` crea `plan.md`: cómo, archivos y paquetes afectados.
3. **Tasks** - `planner` crea `tasks.md`: tareas pequeñas, numeradas, verificables.
4. **Implement** - `implementer` hace una tarea por vez y la marca `[x]`.
5. **Review** - `reviewer` aprueba o devuelve hallazgos concretos.
6. **Memory** - tras cada tarea aprobada se actualiza `MEMORY.md` (ver regla).

El usuario aprueba la spec antes de pasar al plan.

## Plantillas (cortas)
- `spec.md`: Objetivo · Alcance (incluye / excluye) · Criterios de aceptación (lista verificable).
- `plan.md`: Enfoque · Archivos/paquetes · Riesgos.
- `tasks.md`: `- [ ] T1 descripción (archivos) — verificación`.

## Ahorro de tokens
- Leer **solo** lo necesario: `MEMORY.md`, la spec activa y los archivos que nombra la tarea.
- `docs/decisiones.md` es la fuente de decisiones; leerla solo al escribir specs.
- No releer archivos ya leídos en la misma tarea. Buscar con Grep/Glob antes de abrir archivos grandes.
- Respuestas de subagentes al coordinator: **máximo 10 líneas**, con rutas y resultado.
- Sin resúmenes largos, sin repetir el contenido de la spec.

## Regla MEMORY.md (obligatoria)
- Después de **cada implementación aprobada por el reviewer**, actualizar `MEMORY.md`.
- Lo actualiza el `implementer` (el coordinator se lo pide tras la aprobación).
- Contenido: fase actual, spec activa, tareas hechas/pendientes, bloqueos, próximo paso.
- **Máximo 50 líneas.** Reemplazar el estado viejo, no acumular historial.
- Las decisiones nuevas van a `docs/decisiones.md`, no a `MEMORY.md`.

## Regla de subida a GitHub (aprobada por el usuario)
1. Solo se sube código **completo y aprobado por el reviewer**; nunca respaldos a medias, aunque un hook lo pida.
2. Se sube primero a la rama `wip/actual` (sin pedir permiso) para que el CI lo pruebe en Windows.
3. Si el CI falla, se corrige y se repite; `main` no se toca.
4. Si el CI pasa, se sube a `main` (sin pedir permiso) y se confirma que el CI de `main` queda en verde.
5. Al terminar cada tarea, el coordinator envía al usuario un resumen. El usuario revisa por tarea, no por subida.
6. Siempre requieren aprobación del usuario: specs nuevas, cambios de alcance y decisiones de producto.

## Convenciones de código
- Go, Windows únicamente, sin privilegios de administrador.
- `go vet` y `go test ./...` deben pasar antes de pedir revisión.
- Commits pequeños, uno por tarea, mensaje en español.

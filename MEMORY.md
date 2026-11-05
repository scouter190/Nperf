# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T10 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T11-T13 y T15 pendientes; T16 propuesta, sin aprobar).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar). Repo: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` s enteros, `-proto tcp|udp|both` (default both), `-udp-rate`, `-direction fwd|rev|both` (default both), `-streams` 1-64 (default 4), `-key`), `internal/{proto,meter,server,client,auth,version,hrtime,tcpinfo}`.
- Cliente abre siempre las conexiones; en rev mide el cliente. `Result{Fwd,Rev,UDPFwd,UDPRev,RTT,Notes}`; "no medido" con `Measured`. `client.Run` usa fwd por defecto: los consumidores (T12/T13) pasan Direction explícito.
- Sesión única (T10): una sesión autenticada = varias fases (params -> start -> result) con el turno retenido; `both` y `-proto both` son fases fwd/rev en la misma sesión. Protocolo: handshake -> `turn` -> `queue{position}`* -> `ready` -> ping* -> params.
- Cola (`internal/server/queue.go`): FIFO, 1 en curso + 5 en espera; el 6.º recibe "cola llena" (código ≠ 0); quien sale de la cola hace avanzar a los demás; turno liberado siempre; sin plazos mientras espera (`Options.HandshakeWait`, `OnQueue`). Solo entran clientes autenticados. Sin límite por IP por defecto (`WithMaxPerIP` configurable). Keepalive TCP 15 s en el server de la CLI. Hook de test: `WithTurnHook`.
- TCP multiflujo: throughput = suma medida por el receptor en ventana común; un flujo que falla = error, sin suma parcial.
- Secuencia `-proto both`: RTT (10 ping/pong por control, mín/media/máx) -> TCP fwd/rev -> UDP fwd/rev. UDP = 90 % del TCP del mismo sentido, tope 10 Gbps; `-udp-rate` manda; con `-proto tcp` es error de uso. Un fallo aborta todo.
- UDP: saludo `NPUDP1 <session>` en 5300 (server responde `ok`; solo el primer saludo asocia dirección); 100 Mbps por defecto sin TCP previo.
- Auth: HMAC-SHA256 mutua (rol + nonces + id de sesión); la clave nunca viaja; handshake 10 s (`hsWait`); aviso en stderr sin clave. Versión en `internal/version`, regla de versión mayor.
- Retransmisiones (`internal/tcpinfo`): exactas en Linux, estimadas en Windows ("~N (estimado)"), "no disponible" en otros SO. Las ve el emisor; en rev las envía el server en el resultado.
- Reloj: `internal/hrtime` (QPC en Windows) para RTT y jitter UDP.
- Arreglos Windows: TCP fin por reloj entre escrituras + deadline end+2 s; UDP ráfaga acotada con deuda máx. 20 ms.
- Tests con `-race`: integración TCP/UDP en loopback, cola (7 clientes, FIFO, cancelación), errores, unitarios.
- CI (`.github/workflows/ci.yml`): windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`. LF forzado. Guía Windows: `docs/entorno-local.md`.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 · ±10 % vs iperf3 · CLI en inglés.

## Notas
- Por decisión del usuario, no se sube nada a GitHub (ni main ni wip) hasta que él lo revise; los commits quedan locales. Desde T10 no hay CI de Windows hasta la próxima subida.
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.

## Próximo paso
Aprobación de T16 por el usuario; luego T11 (CLI completa).

## Pendientes
- T11: acotar RateBps máximo del server en UDP rev (`sendUDPRev`, internal/server/udp.go); validar `-duration` 3-300 s; config local.
- T12: retirar de `client.Result` los campos duplicados Sent/Received/Mbps/UDP (usar Fwd/Rev).
- T16 (estado del equipo, `nperf status`, criterio 14) escrita por el planner, PENDIENTE de aprobación del usuario (incluida la decisión: consulta protegida con la clave).
- Confirmar Walk como librería de interfaz.
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"): causa no encontrada; no reproducido en 500+ corridas; el test reporta el paso exacto. Si reaparece, analizar ese mensaje.

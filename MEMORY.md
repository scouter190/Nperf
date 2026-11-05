# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T14 hechas y aprobadas, excepto T15 y T16.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T15 pendiente; T16 aprobada como diseño, sin implementar).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar). Repo: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` 3-300 s validado antes de conectar, `-proto tcp|udp|both` (default both), `-udp-rate`, `-direction fwd|rev|both` (default both), `-streams` 1-64 (default 4), `-key`), `internal/{proto,meter,server,client,auth,config,version,hrtime,tcpinfo}`.
- CLI (T11): ayudas en español (`nperf`, `-h`, `help`, `client -h`, `server -h`); comando desconocido = ayuda + código 2. Códigos: 0 ok, 1 error de medición/conexión/auth, 2 error de uso. Salida "Medición completa (RTT, TCP y UDP)" / "Midiendo TCP|UDP"; nota si UDP enviado < 90 % de la tasa objetivo.
- Clave: `-key` o `config.json` en os.UserConfigDir()/NPerf (`{"key":"..."}`, `internal/config`); el flag manda; archivo inválido = error (código 2); aviso "sin clave" solo si no hay ninguna. Tests inyectan `configDir`/`stdout`/`stderr`.
- T12: `internal/report` (texto y JSON). `client -json`: stdout solo JSON v1 (`format_version`, unidades en nombres de campo, null + `status` para no medido/no disponible, notas en `notes`); avisos y cola por stderr; errores (también de flags) como `{\"error\"}` con el mismo código. `client.Result` sin campos duplicados (solo Fwd/Rev/UDPFwd/UDPRev).
- Cliente abre siempre las conexiones; en rev mide el cliente. `Result{Fwd,Rev,UDPFwd,UDPRev,RTT,Notes}`; "no medido" con `Measured`. `client.Run` usa fwd por defecto: los consumidores pasan Direction explícito.
- Sesión única: una sesión autenticada = varias fases (params -> start -> result) con el turno retenido. Protocolo: handshake -> `turn` -> `queue{position}`* -> `ready` -> ping* -> params.
- Cola (`internal/server/queue.go`): FIFO, 1 en curso + 5 en espera; el 6.º recibe "cola llena". Solo entran clientes autenticados. Keepalive TCP 15 s en el server de la CLI.
- Cola: el turno se libera en cuanto el cliente se va, en cualquier fase (watchControl en todas las esperas del server).
- TCP multiflujo: suma medida por el receptor en ventana común; un flujo que falla = error.
- `-proto both`: RTT -> TCP fwd/rev -> UDP fwd/rev. UDP = 90 % del TCP del mismo sentido, tope 10 Gbps; `-udp-rate` manda; con `-proto tcp` es error de uso. Un fallo aborta todo.
- UDP: saludo `NPUDP1 <session>` en 5300; 100 Mbps por defecto sin TCP previo. El server rechaza RateBps > `server.MaxUDPRate` (10 Gbps) al recibir params (UDP rev); el cliente muestra ese error.
- Auth: HMAC-SHA256 mutua; la clave nunca viaja; handshake 10 s. Versión en `internal/version`.
- Retransmisiones (`internal/tcpinfo`): exactas Linux, estimadas Windows, "no disponible" otros SO. Reloj `internal/hrtime` (QPC).
- Cliente: timeout de dial 5 s (`Options.DialTimeout`, "tiempo agotado").
- Tests: `-race`, loopback TCP/UDP, cola, errores; `server.WithUDPConn` e `internal/testutil`.
- T13: cmd/nperf/e2e_test.go prueba el binario real como proceso (criterios 1, 2, 4, 5, 6, 10, 11 y ayuda); config aislada; -short lo salta.
- CI (`.github/workflows/ci.yml`): windows-latest vet/test/build + ubuntu `-race`. Guía Windows: `docs/entorno-local.md`.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 · ±10 % vs iperf3 · CLI: comandos y flags en inglés, mensajes en español.

## Notas
- Subidas: regla en AGENTS.md (wip/actual → CI Windows verde → main, sin pedir permiso; el usuario revisa por tarea).
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.
- CI Windows: tests con -p 1 (paquetes en serie) para evitar contención de CPU en tests de tiempo real.

## Próximo paso
T15 (procedimiento manual iperf3) y T16 (`nperf status`).

## Pendientes
- T16 (`nperf status`, criterio 14): consulta pública en la LAN, sin clave; solo expone `state` y `queue`, antes del handshake. Añadirlo a la ayuda general al implementarlo.
- Confirmar Walk como librería de interfaz.
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"), no reproducido; si reaparece, analizar ese mensaje.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T11 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T12, T13 y T15 pendientes; T16 aprobada como diseño, sin implementar).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar). Repo: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` 3-300 s validado antes de conectar, `-proto tcp|udp|both` (default both), `-udp-rate`, `-direction fwd|rev|both` (default both), `-streams` 1-64 (default 4), `-key`), `internal/{proto,meter,server,client,auth,config,version,hrtime,tcpinfo}`.
- CLI (T11): ayudas en español (`nperf`, `-h`, `help`, `client -h`, `server -h`); comando desconocido = ayuda + código 2. Códigos: 0 ok, 1 error de medición/conexión/auth, 2 error de uso. Salida "Medición completa (RTT, TCP y UDP)" / "Midiendo TCP|UDP"; nota si UDP enviado < 90 % de la tasa objetivo.
- Clave: `-key` o `config.json` en os.UserConfigDir()/NPerf (`{"key":"..."}`, `internal/config`); el flag manda; archivo inválido = error (código 2); aviso "sin clave" solo si no hay ninguna. Tests inyectan `configDir`/`stdout`/`stderr`.
- Cliente abre siempre las conexiones; en rev mide el cliente. `Result{Fwd,Rev,UDPFwd,UDPRev,RTT,Notes}`; "no medido" con `Measured`. `client.Run` usa fwd por defecto: los consumidores pasan Direction explícito.
- Sesión única: una sesión autenticada = varias fases (params -> start -> result) con el turno retenido. Protocolo: handshake -> `turn` -> `queue{position}`* -> `ready` -> ping* -> params.
- Cola (`internal/server/queue.go`): FIFO, 1 en curso + 5 en espera; el 6.º recibe "cola llena". Solo entran clientes autenticados. Keepalive TCP 15 s en el server de la CLI.
- TCP multiflujo: suma medida por el receptor en ventana común; un flujo que falla = error.
- `-proto both`: RTT -> TCP fwd/rev -> UDP fwd/rev. UDP = 90 % del TCP del mismo sentido, tope 10 Gbps; `-udp-rate` manda; con `-proto tcp` es error de uso. Un fallo aborta todo.
- UDP: saludo `NPUDP1 <session>` en 5300; 100 Mbps por defecto sin TCP previo. El server rechaza RateBps > `server.MaxUDPRate` (10 Gbps) en UDP rev; el cliente muestra ese error si falla el saludo.
- Auth: HMAC-SHA256 mutua; la clave nunca viaja; handshake 10 s. Versión en `internal/version`.
- Retransmisiones (`internal/tcpinfo`): exactas Linux, estimadas Windows, "no disponible" otros SO. Reloj `internal/hrtime` (QPC).
- Cliente: timeout de dial 5 s (`Options.DialTimeout`, "tiempo agotado").
- Tests: `-race`, loopback TCP/UDP, cola, errores; `server.WithUDPConn` e `internal/testutil`.
- CI (`.github/workflows/ci.yml`): windows-latest vet/test/build + ubuntu `-race`. Guía Windows: `docs/entorno-local.md`.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 · ±10 % vs iperf3 · CLI en español.

## Notas
- Por decisión del usuario, no se sube nada a GitHub hasta que él lo revise; commits locales. Sin CI de Windows desde T10.
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.

## Próximo paso
T12 (salida `-json`; retirar de `client.Result` los campos duplicados Sent/Received/Mbps/UDP y usar Fwd/Rev). Luego T13, T15, T16.

## Pendientes
- T16 (`nperf status`, criterio 14): consulta pública en la LAN, sin clave; solo expone `state` y `queue`, antes del handshake. Añadirlo a la ayuda general al implementarlo.
- Confirmar Walk como librería de interfaz.
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"), no reproducido; si reaparece, analizar ese mensaje.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T9 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T10-T13 y T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar). Repo: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` s enteros, `-proto tcp|udp|both` (default both), `-udp-rate`, `-direction fwd|rev|both` (default both), `-streams` 1-64 (default 4), `-key`), `internal/{proto,meter,server,client,auth,version,hrtime,tcpinfo}`.
- Cliente abre siempre las conexiones; en rev mide el cliente. `both` = dos sesiones (fwd y rev). `Result{Fwd,Rev,UDPFwd,UDPRev,RTT,Notes}`; "no medido" con `Measured`. `client.Run` usa fwd por defecto: los consumidores (T12/T13) pasan Direction explícito.
- TCP multiflujo: throughput = suma medida por el receptor en ventana común; un flujo que falla = error, sin suma parcial.
- Secuencia `-proto both`: RTT (10 ping/pong por control, mín/media/máx) -> TCP fwd/rev -> UDP fwd/rev. UDP = 90 % del TCP del mismo sentido, tope 10 Gbps; `-udp-rate` manda; con `-proto tcp` es error de uso. Un fallo aborta todo.
- UDP: saludo `NPUDP1 <session>` en 5300 (server responde `ok`; solo el primer saludo asocia dirección); 100 Mbps por defecto sin TCP previo.
- Auth: HMAC-SHA256 mutua (rol + nonces + id de sesión); la clave nunca viaja; handshake 10 s (`hsWait`, único hasta params); aviso en stderr sin clave. Versión en `internal/version`, regla de versión mayor.
- T9 retransmisiones (`internal/tcpinfo`): exactas en Linux (tcpi_total_retrans, amd64/arm64), estimadas en Windows (BytesRetrans/MSS, se muestra "~N (estimado)"), "no disponible" en otros SO. Las ve el emisor; en rev las envía el server en el resultado (`retrans`, `retrans_est`).
- Reloj: `internal/hrtime` (QPC en Windows) para RTT y jitter UDP.
- Arreglos Windows: TCP fin por reloj entre escrituras + deadline end+2 s de protección; UDP ráfaga acotada con deuda máx. 20 ms.
- Tests con `-race`: integración TCP/UDP en loopback, errores, cancelación, saludo UDP, unitarios.
- CI (`.github/workflows/ci.yml`) verde: windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`. LF forzado (`.gitattributes`). Guía Windows: `docs/entorno-local.md`.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Notas
- El CI de Windows detecta fallos que Linux no ve; probar en rama wip antes de subir a main.
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.

## Próximo paso
`implementer` arregla el flaky de abajo y luego T10 (cola y `both` en una sola sesión); luego `reviewer`.

## Pendientes
- Flaky: TestStreamsOutOfRangeRejected (server/streams_test.go:69, connection reset): el server cierra antes de que el cliente lea el error.
- T10: cola máx. 5 y `both` en una sola sesión (server hace fwd y luego rev en el mismo turno).
- T11: acotar RateBps máximo del server en UDP rev (`sendUDPRev`, internal/server/udp.go); validar `-duration` 3-300 s; config local.
- T12: retirar de `client.Result` los campos duplicados Sent/Received/Mbps/UDP (usar Fwd/Rev).
- Confirmar Walk como librería de interfaz.
- Flaky TestStreamsOutOfRangeRejected (1 vez, "connection reset"): causa no encontrada (no hay datos pendientes en ese camino); no reproducido en 500+ corridas; el test ahora reporta el paso exacto. Si reaparece, analizar ese mensaje.

# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1-T8 y T14 hechas y aprobadas (más arreglos Windows de TCP y UDP).

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T1-T8 y T14 hechas; T9-T13 y T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar).
- Repo en GitHub: https://github.com/scouter190/Nperf (rama `main`).
- `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` en segundos enteros, `-proto tcp|udp`, `-udp-rate`, `-direction fwd|rev|both` (default both)), `internal/proto`, `internal/meter` (tcp.go, udp.go), `internal/server` (server.go, udp.go), `internal/client`.
- UDP integrado en la sesión: `-proto udp`, `-udp-rate` acepta k/M/G o bps (base 1000), 100 Mbps por defecto sin TCP previo.
- Saludo UDP `NPUDP1 <session>` en el puerto 5300: el server responde `ok`; solo el primer saludo asocia la dirección, los de otra dirección se ignoran.
- T6: el cliente siempre abre las conexiones; en rev mide el cliente. "No medido" con `Result{Fwd,Rev}` y `Measured`. `both` = dos sesiones (fwd y rev); sesión única pendiente en T10.
- T7: `-streams` 1-64 (default 4) en TCP fwd/rev; throughput = suma medida por el receptor en ventana común; un flujo que falla = error, sin suma parcial; el server limita las conexiones de datos al número negociado (0 = 1).
- T8: `-proto both` es el default de la CLI: RTT (10 ping/pong por el canal de control antes de la carga; mín/media/máx; "<1 µs" si es menor) -> TCP fwd/rev -> UDP fwd/rev. UDP = 90 % del TCP del mismo sentido, tope 10 Gbps; `-udp-rate` manda; con `-proto tcp` es error de uso. Si el TCP da tasa inutilizable, ese UDP queda "no medido" con nota. Un fallo de cualquier fase aborta todo. `Result`: RTT, Fwd/Rev (TCP), UDPFwd/UDPRev.
- Server: plazo (hsWait, 10 s) único desde el fin del handshake hasta recibir params; los ping no lo extienden.
- `client.Run` usa fwd por defecto y la CLI both: los consumidores (T12/T13) deben pasar Direction explícito.
- Tests con `-race`: integración TCP y UDP en loopback, errores, cancelación, saludo UDP, unitarios de proto y meter.
- Auth (T5): HMAC-SHA256 mutua (rol + ambos nonces + id de sesión firmado); la clave nunca viaja; deadline de handshake 10 s; aviso en stderr si no hay clave.
- Versión única en `internal/version` (`Current`), regla de versión mayor; aviso en ambos lados si difiere.
- Flag `-key` en server y client (la config local llega en T11).
- El rango 3-300 s de `-duration` se valida en T11.
- CI (`.github/workflows/ci.yml`) en verde: windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`.
- LF forzado con `.gitattributes` (gofmt en CI). Guía Windows: `docs/entorno-local.md`.

## Arreglos Windows (aprobados, CI verde)
- TCP (`meter/tcp.go`): el fin lo decide el reloj entre escrituras; deadline único end+2 s solo como protección; si salta sin cancelación -> error "envío bloqueado: medición no fiable". Motivo: en Windows una escritura cancelada por deadline devuelve n=0 aunque los datos salieron.
- UDP (`meter/udp.go`): ráfaga por iteración acotada (`burstPackets`) + Gosched; se conserva hasta 20 ms de deuda (`debtWindow`) por atrasos del timer de Windows (~16 ms); el exceso se perdona. Ráfaga máx. ~2,5 MB a 1 Gbps.

- Reloj: RTT (client) y marcas/jitter UDP (meter, clock.now) usan `internal/hrtime` (QPC en Windows, time.Since de origen fijo en otros SO) porque el reloj de Go en Windows tiene resolución gruesa y cuantizaba RTT a 0. La cabecera UDP lleva ns desde el origen del emisor; el jitter solo usa diferencias.

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Notas
- Lección: el CI de Windows detecta fallos que Linux no ve; probar en la rama wip/actual antes de subir a main.
- `greetTries`/`greetWait` (internal/client) son globales de test: no usar `t.Parallel` con `greetUDP`.

## Próximo paso
`implementer` hace T9 (retransmisiones TCP por SO); luego `reviewer`.

## Pendientes
- T10: `both` en una sola sesión (server hace fwd y luego rev en el mismo turno).
- T11: acotar RateBps máximo del server en UDP rev (`sendUDPRev`, internal/server/udp.go).
- T12: retirar de `client.Result` los campos duplicados Sent/Received/Mbps/UDP (usar Fwd/Rev).
- Confirmar Walk como librería de interfaz.

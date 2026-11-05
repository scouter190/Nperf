# MEMORY.md - Estado de NPerf

> Máximo 50 líneas. Se reescribe tras cada implementación aprobada (ver AGENTS.md).

## Fase actual
**Fase 3 - Implementando spec 001.** T1, T2, T3 y T14 hechas y aprobadas.

## Spec activa
`specs/001-medidor-par/`: spec, `plan.md`, `tasks.md` (T1, T2, T3 y T14 hechas; T4-T13 y T15 pendientes).

## Estado del código
- Módulo `nperf` (Go 1.27.3, solo librería estándar).
- Repo en GitHub: https://github.com/scouter190/Nperf (rama `main`).
- Esqueleto TCP `fwd` de punta a punta: `cmd/nperf` (`server`, `client <host>`, `-port` 5300, `-duration` en segundos enteros), `internal/proto`, `internal/meter/tcp.go`, `internal/server`, `internal/client`.
- Medidor UDP (`internal/meter/udp.go`): ritmo por bytes adeudados con catch-up acotado, jitter RFC 3550, pérdida con dedupe de duplicados, FIN con terminación por idle; tests con reloj y lector falsos.
- El ritmo UDP en Windows real se valida en el CI y a mano (T15).
- Nota menor del reviewer: falta test para seq >= maxTrackedSeq.
- Tests con `-race`: integración en loopback, errores, segunda prueba, cancelación, unitarios de proto y meter.
- El rango 3-300 s de `-duration` se valida en T11.
- CI (`.github/workflows/ci.yml`) en verde: windows-latest vet/test/build + artefacto `nperf.exe`; ubuntu `-race`.
- Finales de línea LF forzados con `.gitattributes` (gofmt en CI).
- Guía de entorno para Windows: `docs/entorno-local.md` (el usuario clona en Documentos).
- Compilación: `scripts/build-windows.sh` (cruzada, GOOS=windows GOARCH=amd64) y `scripts/build-windows.ps1` (nativo en Windows); salida `nperf.exe` en la raíz.
- Nota: la constante `Version` está duplicada en `internal/client` e `internal/server`; se unifica en T5 (`internal/version`).

## Decisiones cerradas (spec 001)
Puerto 5300 · UDP 100 Mbps sin TCP previo · cola máx. 5 (6.º rechazado) · ±10 % vs iperf3 · CLI en inglés.

## Próximo paso
`implementer` hace T4 (UDP en la sesión); luego `reviewer`.

## Pendientes
- Confirmar Walk como librería de interfaz.

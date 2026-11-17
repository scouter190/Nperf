# 004 - Plan

## Enfoque
- **Capas**: paquete nuevo `internal/probe` con toda la lógica portable (validación, ritmo, estadísticas, traceroute y su análisis, búsqueda de MTU, puertos, diagnóstico combinado, salida texto/JSON), probada en Linux con un entorno inyectable. Lo Win32 queda en `_windows.go` finos; en otros SO, `_other.go` devuelve "no soportado" (→ "no disponible" + motivo).
- **Entorno inyectable** (`probe.Env`): `Echoer` (`Echo(ctx, EchoRequest{Dst, TTL, DF, Size, Timeout}) (Reply, error)`; `Reply{Status, From, RTTms}`), `Router` (`Route(dst) (Route{IfIndex, Source, NextHop, MTU}, error)`, `Gateway()`, `LocalPrefixes()`), `Resolver` (primera IPv4), `Clock` (`Now`, `Sleep(ctx, d)`) y `Dialer` (TCP). `probe.System()` arma el real.
- **Estados ICMP** portables: tabla código Win32 → `Status` (ok, timeout, ttl_expired, host/net/port unreachable, frag_needed, otro) + motivo en español. `IcmpSendEcho2` también devuelve el estado por `GetLastError` cuando retorna 0: ambos caminos usan la misma tabla.
- **Windows ICMP** (`iphlpapi.dll` por `windows.NewLazySystemDLL`): `IcmpCreateFile` por diagnóstico, `IcmpSendEcho2` síncrono (Event y APC nulos) con `IP_OPTION_INFORMATION{Ttl, Flags=IP_FLAG_DF}`, `IcmpCloseHandle`. Buffer de respuesta = sizeof(`ICMP_ECHO_REPLY`) + carga + 8 + 16 (IO_STATUS_BLOCK) + margen. Structs a mano (64 bits: `IP_OPTION_INFORMATION` 16 B, `ICMP_ECHO_REPLY` 40 B) con test de `unsafe.Sizeof`.
- **Cancelación ≤ 1 s**: cada `Echo` corre en una goroutine; el runner vuelve en cuanto se cancela el contexto y no envía más; el handle se cierra cuando la llamada en vuelo termina (≤ timeout, máx. 5 s).
- **Ruta y puerta de enlace** (`route_windows.go`): `GetBestRoute2` (LazyDLL; `MIB_IPFORWARD_ROW2`, `SOCKADDR_INET` a mano con test de tamaño) da interfaz, IP de origen y siguiente salto; MTU y prefijos locales de `windows.GetAdaptersAddresses` (campos `Mtu`, `OnLinkPrefixLength`). Puerta de enlace = siguiente salto de la mejor ruta hacia una IP de referencia pública (consulta de tabla, no envía paquetes); sin siguiente salto → "sin puerta de enlace". MTU de interfaz fuera de 576–9216 (p. ej. loopback) → desconocido → 1500.
- **Validación previa**: sintaxis (IPv4 o nombre ≤ 253, sin esquema, puerto, `/` ni `-` de rango), resolución y validación de la IP resultante (0.0.0.0, multicast, 255.255.255.255, red/broadcast de `LocalPrefixes`) y límites de opciones, todo antes del primer `Echo`/`Dial`.
- **Runners**: `Ping` (ritmo por `Clock`, errores ICMP = pérdida con motivo), `Trace` (sondas en serie con TTL, para al llegar o en máx. saltos) + `Analyze` (cuatro textos con persistencia), `MTU` (búsqueda binaria 576..MTU interfaz, 2 intentos, tope 20 sondas, carga = tamaño − 28), `Ports` (semáforo de 4, `Dial` con timeout, cierre inmediato; RST → cerrado, timeout → filtrado).
- **Diagnóstico combinado** (`combined.go`, función pura): entrada = estadísticas del tramo 1 (salto 1 del trace, o el ping si el destino es la puerta de enlace o está en enlace directo, `NextHop` vacío), tipo y señal de la interfaz de salida (`netinfo.ForLocalIP(Route.Source)`), MTU de la ruta.
- **Backend**: `config.ManualHosts []{target, alias}` (máx. 64). `App`: `AddHost/RemoveHost`, puerta de enlace por `Deps.Gateway` (caché 5 s), `State.NoAgent []Device` (id `host:<target>` o `gateway`, `can_test` falso); `StartProbe(kind, id)/CancelProbe/WaitProbe` con `State.Probe` (estado, resultado, combinado), uno a la vez, valores por defecto de la spec; `Deps.Probe probe.Env`.
- **Puente**: `addHost(str, str)`, `removeHost(str)`, `startProbe(str, str)` (kind ∈ ping|trace|mtu|ports), `cancelProbe()`; tipos y límites validados en Go.
- **CLI**: `ping|trace|mtu|ports` con `probe.Env` sustituible en tests; texto y JSON de `probe` (`format_version: 1`, `command`, null + `status`).

## Paquetes y archivos
- `internal/probe` (nuevo): `target.go`, `options.go`, `portlist.go`, `icmp.go`, `status.go`, `stats.go`, `clock.go`, `ping.go`, `trace.go`, `analyze.go`, `mtu.go`, `ports.go`, `combined.go`, `route.go`, `report.go`, `system.go`, `icmp_windows.go`/`icmp_other.go`, `route_windows.go`/`route_other.go`.
- `internal/config/config.go`; `internal/app/hosts.go`, `probe.go`, `app.go`; `internal/gui/bridge.go`, `service.go`; `internal/ui/web/*`.
- `cmd/nperf/probe.go`, `main.go`; `docs/validacion-sin-agente.md`; `docs/decisiones.md` §19.

## Riesgos
- **Structs Win32 a mano** (`ICMP_ECHO_REPLY`, `MIB_IPFORWARD_ROW2`): alineación errónea = basura o cuelgue; tests de tamaño en el CI de Windows y pruebas reales contra 127.0.0.1 (criterio 7).
- **TCP en Windows ante RST**: Windows reintenta el SYN tras un RST (~1–2 s), y un puerto cerrado podría salir "filtrado" con timeout 1.5 s. El test `_windows` con puerto cerrado en loopback lo detecta; si falla, el implementer lo reporta al coordinator (no se inventa el arreglo).
- **Loopback en CI**: MTU de loopback no es real (se normaliza a 1500) y trace llega en el salto 1; la coherencia real va al procedimiento manual.
- **Llamada ICMP en vuelo tras cancelar**: el handle vive hasta su timeout; un diagnóstico nuevo crea su propio handle, sin compartir.
- **Puerta de enlace con VPN**: con túnel completo, la "puerta de enlace" es la del túnel; es lo esperado por la regla de la ruta por defecto.

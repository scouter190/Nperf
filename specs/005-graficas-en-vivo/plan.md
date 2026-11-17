# 005 - Plan

## Enfoque
- **Compatibilidad (riesgo de la spec, resuelto)**: `proto.Codec.Read` usa `json.Unmarshal` sin `DisallowUnknownFields` y `server.phase` solo valida `proto`, `direction`, `streams` y `rate_bps`: un server viejo **ignora** `live` en `params`. Pero durante la fase el server viejo trata **cualquier** mensaje del control como abandono (`watchControl` cancela al leer). Por eso el cliente **solo** envía sondas si el server lo confirma con `live: true` en `start`. Es una confirmación por fase, sin cambios en el handshake. El cliente viejo no pide `live` y el server nuevo no envía nada nuevo.
- **Mensajes** (`proto.Message`, campos `omitempty`, así los mensajes de hoy salen idénticos): `live bool` (en `params` y `start`), `seq` (`ping`/`pong` de fase), tipo nuevo `sample` (`index`, `t_ms`, `duration_ms`, `bytes`, `mbps`, `received`, `expected`, `lost`, `loss_pct`, `jitter_ms`). `Codec.Write` con mutex, porque muestras, pongs y `result` se escriben desde goroutines distintas. `SampleFromMessage` valida lo recibido: no negativos, índice creciente, como máximo ⌈D⌉ ≤ 300 por fase y nada después de `result`.
- **Intervalos** (`meter`, lógica pura con reloj inyectado): ⌈D⌉ ventanas de 1 s desde el primer byte o datagrama. La última queda abierta hasta el final real y lleva su duración. La suma de bytes es igual al total. Los resultados finales usan el mismo código y el mismo reloj de hoy; el muestreo solo lee.
  - TCP: contador atómico por flujo (una suma por `Read` de 128 KiB) y una goroutine de muestreo que lee la suma en cada borde.
  - UDP: el bucle del receptor compara `now` (ya calculado) con el siguiente borde. Pérdida del intervalo = máx(0, esperados − recibidos); los desordenados cuentan en el intervalo en que llegan. Jitter = estimador RFC 3550 al cerrar el intervalo. Sin trabajo extra por datagrama salvo contadores.
  - Entrega por callback no bloqueante (canal con capacidad 300): el medidor nunca espera a la red.
- **Server**: con `params.live` responde `start{live}`. En `fwd` (TCP y UDP) emite cada `sample` por el control y vacía la cola de muestras **antes** de `result`. `watchControl(…, live)` contesta `ping{seq}` con `pong{seq}` en lugar de cancelar (tope duración en s + 10 por fase; los que sobran no se contestan). En sesiones `live`, las lecturas entre fases también contestan pings: un ping que llegó tras cerrar la fase no rompe la siguiente.
- **Cliente**: durante una fase `live`, una goroutine lectora del control reparte `sample`, `pong` y `result`/`error`. El *prober* (reloj inyectado) envía la sonda k a k s del `start` (k < ⌈D⌉), con una sola en vuelo y 1 s de plazo ("sin respuesta"). La sonda k da el RTT de la muestra k. La muestra k se entrega (`OnInterval`) cuando llegó y su sonda está resuelta. En `rev` las muestras las genera el propio cliente. Tras `result` espera ≤ 1 s la sonda en vuelo y descarta los `pong` sueltos.
  - Opciones nuevas: `NoLoadRTT`, `OnReady(local, remote *proto.Link)`, `OnRTT(RTT)` y `OnInterval(phase, Interval)`.
  - `Dir` añade `Live bool`, `Intervals []Interval` y `LoadedRTT *LoadedRTT` (mín/media/mediana/máx sobre sondas respondidas, número de sondas). La nota de bufferbloat va a `Result.Notes`.
- **Prueba de "sondas sin efecto"** (criterio 4): server y cliente en proceso, `DialContext` del test envuelve las conexiones con un limitador de tasa (token bucket, p. ej. 200 Mbps). Así la velocidad la fija el limitador y no la CPU del runner. Se alternan 3 TEST! con sondas y 3 sin ellas (`NoLoadRTT`), se compara la mediana de Mbps (< 1 %) y la media de bytes por intervalo sin el primero ni el último (< 1 %).
- **Informe y CLI**:
  - `report`: `JSONDir` añade `intervals` (`t_s`, `duration_s`, `bytes`, `throughput_mbps`, `rtt_ms`, y en UDP `datagrams_received`, `datagrams_lost`, `loss_pct`, `jitter_ms`; null en TCP) y `loaded_rtt` (`status`, `min_ms`, `avg_ms`, `median_ms`, `max_ms`, `samples`). Si el sentido es `not_measured`, ambos van a null. Si se midió sin muestras, `intervals` null y `loaded_rtt.status` `unavailable`.
  - `IntervalLine` en minúsculas, p. ej. `  tcp ida 3-4 s: 941.2 Mbps, rtt 1.3 ms`. No contiene "TCP fwd" ni "RTT:", porque el e2e comprueba el orden de esas cadenas en stdout.
  - `cmd/nperf`: imprime las líneas en vivo solo sin `-json`. `-no-load-rtt` queda fuera de la ayuda (lista de ocultos en `newFlagSet`).
- **App**: `TestState.Chart` se arma en vivo desde los callbacks y se conserva en `done`, `cancelled` (`cancelled: true`) y `error` hasta el siguiente TEST!. Forma compacta por fase: `{phase, label, nominal_mbps|null, rtt_initial{min_ms,avg_ms,max_ms}|null, samples:[[t_s,dur_s,mbps,rtt_ms|null,loss_pct|null,jitter_ms|null]], loaded_rtt|null, note, available}`. La etiqueta usa los alias y el sentido; la nominal sale de `OnReady` con la regla de §18.
- **Monitor de tarjetas** (`netinfo`, portable): `CounterSource.Counters() []IfCounter{GUID, In, Out uint64, Up bool}`. Se empareja por GUID con `Inventory` para excluir los virtuales. Tasa = Δbytes·8/Δt. Se descarta el punto si el contador retrocede, el adaptador falta o el hueco es > 3 s (al reanudar, el primer punto solo fija la base). Ventana de 5 min, tope 300 puntos. Un adaptador ausente o caído corta la serie con el estado "desconectado".
  - Activación por **permiso de 3 s**: `Touch()` lo renueva; sin renovar, la goroutine de 1 s se detiene y no consulta la fuente. `Step(now)` es la lógica pura que se prueba.
  - Puente `monitor(bool)`: `true` renueva y devuelve la vista; `false` para. La UI lo llama cada 1 s solo con el panel visible y `document.visibilityState` visible. Ocultar a la bandeja llama a `App.StopMonitor()`.
- **Win32** (`counters_windows.go`): `GetIfTable2`/`FreeMibTable` por LazyDLL de iphlpapi. `MIB_IF_ROW2` se declara a mano (1352 B en 64 bits) con test de `Sizeof`/`Offsetof` (`InterfaceGuid`, `OperStatus`, `InOctets`, `OutOctets`). `FreeMibTable` va en `defer` justo tras el éxito y se puede espiar en el test.
- **UI**: `chart.js` propio que construye SVG con `createElementNS`, `textContent` y clases CSS (la CSP `style-src 'self'` prohíbe atributos `style`; los atributos de presentación SVG sí valen). Incluye ejes con unidades, líneas de referencia, trazos discontinuos y marcadores distintos por serie, `role="img"` con `aria-label` y una tabla de texto con el último valor y mín/media/máx. Se redibuja ≤ 1/s (solo si cambió el número de muestras).

## Paquetes y archivos
- `internal/proto/proto.go`, `live.go` (nuevo).
- `internal/meter/sample.go` (nuevo), `tcp.go`, `udp.go`.
- `internal/server/server.go`, `udp.go`, `live.go` (nuevo).
- `internal/client/prober.go`, `loadrtt.go`, `live.go` (nuevos), `client.go`; test `loadrtt_effect_test.go`.
- `internal/report/report.go`, `interval.go` (nuevo); `cmd/nperf/main.go`.
- `internal/netinfo/counters.go`, `monitor.go`, `counters_windows.go`, `counters_other.go` (nuevos).
- `internal/app/chart.go`, `monitor.go` (nuevos), `test.go`, `app.go`; `internal/gui/bridge.go`, `service.go`, `tray.go`/`tray_windows.go`.
- `internal/ui/web/chart.js` (nuevo), `app.js`, `index.html`, `app.css`, `mock.js`; `internal/ui/ui_test.go`.
- `docs/validacion-graficas.md`; `docs/decisiones.md` §20.

## Riesgos
- **Carreras en el control**: muestras, pongs y `result` comparten conexión. El mutex de `Codec.Write` y el vaciado de muestras antes de `result` lo cubren; los tests corren con `-race`.
- **Ping tardío entre fases**: si el server no lo contestara, la siguiente fase fallaría con "se esperaban parámetros". Lo cubren la respuesta entre fases y el descarte de `pong` sueltos en el cliente; hay un test con una sonda enviada justo antes de `result`.
- **Tests existentes con server simulado**: el cliente nuevo añade `live` a `params`. Un test que compare el mensaje entero fallaría; si pasa, el implementer lo reporta (la spec prohíbe tocar esos tests).
- **Variación entre ejecuciones (< 1 %)**: en loopback sin limitador la velocidad varía más del 1 % entre repeticiones; por eso el test usa el limitador. En la validación manual, el WiFi puede variar > 1 % por sí solo: el procedimiento lo hace por cable y repite el par si el entorno varía.
- **Tamaño de `State()`**: con 300 s y 4 fases son hasta 1200 muestras por sondeo (cada 500 ms); la forma compacta en arrays lo acota a unos 60 KB.
- **`MIB_IF_ROW2` a mano**: un desplazamiento mal puesto da contadores basura; los tests de tamaño y desplazamientos corren en el CI de Windows.
- **Tick de la goroutine TCP**: con el runner cargado, un borde puede llegar tarde. La muestra mide su duración real (borde efectivo), así que la suma no cambia.

# 005 - Gráficas en vivo

> Aprobada por el usuario. Decisiones en `docs/decisiones.md` §20.

## Objetivo
Ver cómo se comporta el enlace **mientras ocurre**. Durante el TEST! se muestran la velocidad y la latencia bajo carga de cada intervalo frente a la velocidad nominal del enlace (spec 003) y al RTT inicial. Así se ven caídas, rampas lentas, un tope fijo que delata el cuello de botella o colas que se llenan (bufferbloat). Fuera del TEST! se ve el tráfico real de las tarjetas del equipo. Desarrolla §8 ("gráficas en vivo") de `docs/decisiones.md`.

## Alcance

### Incluye
- **Muestras cada 1 s en el medidor**: el lado que mide (receptor: el server en `fwd`, el cliente en `rev`) genera una muestra por segundo con el instante relativo, los bytes y los Mbps del intervalo. En UDP añade datagramas recibidos y perdidos, % de pérdida y jitter del intervalo. El último intervalo parcial lleva su duración real. Los resultados finales del TEST! se calculan igual que hoy: las muestras no los alteran.
- **RTT bajo carga**: durante cada fase TCP/UDP, el cliente envía 1 sonda ping/pong por segundo por el canal de control. Es JSON pequeño (< 100 B), sin datos de carga y sin conexiones nuevas, así que no falsea el throughput. El server responde de inmediato. Cada muestra lleva su RTT (o "sin respuesta" si pasa 1 s sin pong). Por sentido se informa mín/media/máx/mediana y el número de sondas.
  - **Nota de diagnóstico**: se emite si la mediana bajo carga ≥ RTT medio inicial + 100 ms. Texto: "la latencia sube N ms con carga en <sentido>: colas llenas en el enlace (bufferbloat)". El umbral de +100 ms es un **supuesto**, ajustable tras la validación manual.
- **Transporte y compatibilidad**: el cliente pide muestras y sondas con un campo opcional en `params`. Si el server lo admite, envía las muestras por el canal de control durante la fase (en `rev` las genera el propio cliente) y contesta las sondas.
  - Cliente nuevo + server viejo: el server ignora el campo y no hay muestras, RTT bajo carga ni gráfica. El resultado sigue igual.
  - Cliente viejo + server nuevo: el cliente no lo pide y no se envía nada nuevo.
  - Sin cambios en handshake, cola, `status` ni anuncio.
- **Gráfica del TEST! en la ventana**:
  - Una serie de Mbps por fase (TCP fwd/rev, UDP fwd/rev), etiquetada con los nombres de los equipos y el sentido.
  - Línea de referencia con la velocidad nominal del sentido (mín(tx emisor, rx receptor), spec 003) cuando se conoce.
  - En UDP, pérdida y jitter por intervalo.
  - Gráfica de latencia con el RTT bajo carga por intervalo frente al RTT inicial (línea de referencia con mín/media/máx), con la nota de bufferbloat si aplica.
  - Se actualiza en vivo, con ≤ 2 s de retraso por muestra. **Queda en el panel de resultado** al terminar, hasta el siguiente TEST! o hasta cerrar la app (no se guarda en disco).
  - Si se cancela, queda lo medido, marcado "cancelado". Si no hay muestras (par antiguo), se muestra el texto "gráfica no disponible: el otro equipo no envía muestras".
- **Monitor de tarjetas**:
  - Bytes de entrada y salida por segundo de cada adaptador físico, excluidos los virtuales de §18. Se leen con `GetIfTable2`/`GetIfEntry2` (iphlpapi, sin administrador) y contadores de 64 bits.
  - Si un contador se reinicia o desborda, se descarta ese punto.
  - Se ve en el panel "Tarjetas de red", con una ventana deslizante de los **últimos 5 min** (tope de 300 puntos por adaptador).
  - Solo se muestrea mientras el panel está visible y la ventana no está en la bandeja.
  - Si un adaptador se desconecta o desaparece, la serie se corta con un texto y sin error.
- **Gráficas propias**: SVG o canvas generado por la UI, sin librerías externas y compatible con la CSP actual.
  - Todo dato de la red se trata como texto (sigue el test que prohíbe innerHTML).
  - Temas claro y oscuro.
  - Accesibles: ejes con unidades; último valor y mín/media/máx visibles en texto; series que se distinguen por trazo o marcador, no solo por color; texto alternativo.
- **Coste bajo**: redibujado ≤ 1 vez por segundo; máx. 300 muestras por fase; sin muestreo con la ventana oculta.
- **CLI**:
  - `nperf client ... -json` añade por sentido `intervals` (lista de muestras con `rtt_ms` o null) y `loaded_rtt` (`status`, `min_ms`, `avg_ms`, `median_ms`, `max_ms`, `samples`).
  - Sin muestras: valores null y `status` `unavailable`. Es un cambio aditivo: `format_version` sigue en 1.
  - La salida legible imprime una línea por intervalo y fase (Mbps; en UDP pérdida y jitter; RTT bajo carga), además del resumen de hoy, que sigue igual. Con `-json` esas líneas no salen por stdout.
  - Flag oculto `-no-load-rtt` en `nperf client` (no sale en la ayuda): desactiva solo las sondas de RTT bajo carga; las muestras siguen. Sirve para comparar el efecto de las sondas.

### Excluye
- Ping continuo con gráfica hacia un equipo (spec posterior).
- `nperf monitor` en consola.
- Exportación CSV/JSON (spec propia) e histórico en disco.
- Pruebas programadas, alertas y notificaciones, mapa de red.
- Retransmisiones por intervalo.
- Gráficas de los diagnósticos de la spec 004.
- Uso de CPU del sistema, conexiones por proceso, IPv6 en el monitor.
- Cambios en el cálculo de los resultados finales.
- Dependencias Go o JS nuevas.

## Criterios de aceptación

### Verificables en CI
1. Medidor con reloj inyectado:
   - Una fase de D s genera ⌈D⌉ muestras.
   - La suma de bytes de las muestras es igual a los bytes finales del sentido.
   - El último intervalo parcial lleva su duración real.
   - Los resultados finales son idénticos con y sin muestras (tabla de casos: TCP con 1 y 4 flujos, UDP).
2. UDP por intervalo: pérdida y jitter de cada muestra correctos con secuencias simuladas (sin pérdida, pérdida en un intervalo, datagramas desordenados entre intervalos).
3. RTT bajo carga con reloj inyectado:
   - 1 sonda/s como máximo y una sola en vuelo.
   - Sonda sin pong en 1 s → "sin respuesta".
   - mín/media/mediana/máx correctos.
   - La nota de bufferbloat aparece con mediana = inicial + 100 ms y no con + 99 ms.
4. Sondas sin efecto en la velocidad: el mismo TEST! en loopback, con y sin sondas de RTT bajo carga, da velocidades con diferencia < 1 %. Se comprueba con varias repeticiones alternas (mediana de cada grupo) sobre conexiones con tasa limitada en el test, y comparando los bytes por intervalo de ambos grupos.
5. Protocolo en loopback:
   - Cliente nuevo + server nuevo: hay muestras y RTT bajo carga en `fwd`, `rev` y `both`.
   - Cliente nuevo contra un server simulado que ignora el campo: resultado completo, sin muestras ni sondas.
   - Cliente que no las pide: no se envían mensajes de muestra ni sondas (test de los mensajes del canal de control).
6. CLI:
   - `-json` lleva `intervals` y `loaded_rtt` con datos, o null + `status` sin ellos; el resto del JSON v1 sigue igual.
   - La salida legible lleva una línea por intervalo.
   - `-no-load-rtt` no aparece en la ayuda y deja `rtt_ms` en null con `loaded_rtt.status` `unavailable`.
   - Los tests de las specs 001–004 pasan sin modificarlos.
7. Monitor con fuente simulada:
   - Tasa = Δbytes/Δt.
   - Un contador que retrocede o desaparece descarta el punto.
   - Los virtuales quedan excluidos.
   - La ventana guarda 5 min y no más de 300 puntos.
   - No se consulta la fuente mientras el monitor está inactivo.
8. Backend de la interfaz:
   - `State()` expone las muestras en vivo del TEST! en curso y conserva las del último resultado, con la nominal del sentido y el RTT inicial (o `null`).
   - El puente valida las funciones nuevas (lista blanca y tipos).
   - La clave nunca aparece.
   - El test que prohíbe innerHTML sigue pasando.
9. En windows-latest, sin administrador: `GetIfTable2` devuelve al menos un adaptador con contadores y siempre se llama a `FreeMibTable`.
10. `go vet` y `go test ./...` pasan en windows-latest; `go.mod` no tiene dependencias nuevas; la UI no carga recursos externos.

### Validación manual en Windows 10/11
11. TEST! por cable de 30 s en "Ambos":
    - La gráfica se mueve en vivo en cada fase.
    - La media visual coincide con el resultado final.
    - La línea nominal (p. ej. 1000 Mbps) está a la altura correcta.
    - La consola muestra una línea por segundo.
12. TEST! por WiFi: se ven la variación por intervalo y la referencia PHY; en UDP se ven pérdida y jitter por intervalo.
13. RTT bajo carga:
    - En un enlace con colas grandes (WiFi saturado o subida de internet), la gráfica de latencia sube frente al RTT inicial y aparece la nota.
    - En cable LAN sin congestión no aparece.
    - El throughput coincide con un TEST! hecho con un cliente de la spec 004 (±10 %).
14. Sondas sin efecto en la velocidad: dos TEST! seguidos en la misma red y con los mismos parámetros, `nperf client <host>` y `nperf client <host> -no-load-rtt`, dan velocidades con diferencia < 1 %.
15. TEST! contra un equipo con NPerf de la spec 004: resultado completo y mensaje "gráfica no disponible".
16. Monitor:
    - Al descargar un archivo grande, la entrada de la tarjeta sube de forma coherente con el Administrador de tareas (±10 %).
    - Al ocultar el panel o minimizar a la bandeja, el uso de CPU de NPerf vuelve al de reposo.
17. Ambos temas son legibles y los valores numéricos se ven sin depender del color.

## Riesgos
- **Campos nuevos en `params`**: resuelto en `plan.md` (el server viejo ignora el campo; las sondas solo se envían si el server lo confirma en `start`).
- **Sondas por el canal de control (TCP)**: miden la cola del camino compartido, que es lo que se busca. Una pérdida en el control puede añadir una retransmisión TCP y verse como pico. Por eso se usa la mediana en la nota.
- **Umbral +100 ms (supuesto)**: puede ser alto para la LAN o bajo para enlaces satelitales; se revisa tras la validación manual.
- **Precisión por intervalo**: los primeros intervalos de TCP reflejan el arranque lento. En multiflujo, la suma por intervalo exige una ventana común. A 10 Gbps en UDP, el conteo no debe añadir trabajo por datagrama.
- **Contadores de `GetIfTable2`**: incluyen todo el tráfico del adaptador, y algunos drivers WiFi los actualizan a saltos, lo que provoca picos y valles.
- **Coste en WebView2**: redibujar el SVG cada segundo; se acota el número de puntos.

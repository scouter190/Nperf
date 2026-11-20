# 008 - Rediseño de la interfaz

> Fuente: `docs/nperf_gui_parameters.md` ("el documento"). Decisiones del usuario sobre las discrepancias (transmitidas por el coordinator) en la tabla final y en `docs/decisiones.md` §23.

## Objetivo
Rehacer `internal/ui/web/` según el documento: app shell con cabecera fija, barra lateral solo de equipos, drawers para Tarjetas de red y Ajustes, modales para consentimiento M-Lab, exportación y borrado, resultados en pirámide invertida y tokens claros/oscuros. Se conserva todo lo que ya hace la ventana sin perder ninguna garantía de seguridad ni de privacidad.

## Alcance
**Incluye**
1. **Tokens y tipografía**: variables CSS del documento (§2.1-2.2) con tema solo automático (`prefers-color-scheme`); solo cambian los tokens que no llegan a AA. Fuentes del sistema: `Segoe UI Variable`, `Segoe UI`, `system-ui` / `Cascadia Mono`, `Consolas`, `monospace`. Números, IPs, velocidades, fechas y MAC en mono con `tabular-nums`.
2. **App shell** (§3): cabecera de 48 px (Panel, Historial (N), indicador de servidor **Activo / con error**, Red, Ajustes), banner persistente para `config_error` y `firewall_hint` (texto de Go tal cual), barra lateral de 260/220 px y área principal, cada una con su scroll, sin scroll horizontal a 720×480. Escape cierra popover → modal → drawer; `:focus-visible` en todo lo interactivo.
3. **Barra lateral** (§4.2): Este equipo (no seleccionable), En la red, Sin NPerf (Quitar en hover/foco), estado vacío con el aviso de firewall y un mini-formulario "+ Añadir equipo" (`addHost(ip, alias)`). Se quitan los acordeones de Tarjetas de red y Ajustes y la alta manual fija.
4. **Panel** (§4.3): barra de TEST! (sentido, duración, `total_label`, `test_blocked_reason` una sola vez, TEST!/Cancelar/Cancelando…) y fila de diagnósticos (Ping, Traceroute, MTU, Puertos | Internet, Velocidad a internet + popover ⓘ de M-Lab, que sustituye al texto fijo). Franja de progreso con la cola (`queue_position`), las fases del sentido elegido y el tiempo transcurrido; en diagnósticos, `label` y el tiempo transcurrido, sin porcentaje. Resultado: cabecera con Repetir, Exportar y Copiar resumen; semáforo; 4 KPI (`—` + "No medido"/"No disponible", nunca 0); gráfica principal TCP ida+vuelta combinada; tablas, gráficas UDP y RTT bajo carga en "Modo detallado". En Internet, los hallazgos aparecen una sola vez.
5. **Historial** (§4.4): filtros con `flex-wrap`, acciones en lote, badge separado del tipo, registro en solo lectura con Volver y Exportar; sin la casilla de historial y con el aviso "Activar en Ajustes" si está desactivado. El contador de la cabecera usa `history({})` al arrancar, al abrir la vista y al terminar cada prueba.
6. **Drawer Tarjetas de red** (§4.5): activas primero; inactivas y virtuales en un acordeón cerrado; BSSID y MAC solo aquí. `adapters(true)` (5 s) y `monitor(true)` (1 s) solo con el drawer abierto, más una carga inicial. El botón Red muestra "Wi-Fi"/"Ethernet" desde `state.self` y, si hay datos, el estándar traducido (802.11ax → Wi-Fi 6, be → Wi-Fi 7…).
7. **Drawer Ajustes** (§4.6): alias, clave (solo escritura; badge según `has_key`), historial (90 días / 50 MB), destinos de internet (máx. 4; "Restaurar" = `setInternetTargets([])`) y subredes extra, con validación en línea. Banner en el drawer con el resultado de cada llamada; "servidor reiniciado" solo tras alias, clave y subredes.
8. **Modales** (§4.7): consentimiento M-Lab en cada prueba (texto fijo "límite de 40 pruebas al día", políticas como enlaces de texto, foco inicial en Cancelar, `startSpeed(true)` solo desde "Aceptar y medir"), exportación compartida por Panel e Historial (estado ocupado mientras el diálogo nativo está abierto) y confirmación de borrado con el alcance exacto.
9. **Render sin parpadeo** (§6): estado local separado del snapshot; el polling de 500 ms no cierra nada ni pisa un input con foco; cada subsección solo se redibuja si cambia su firma.
10. **G2 (Go)**: `test.history_id` y `probe.history_id` con el id del registro guardado (vacío si no se guardó o si se borró). Exportar del Panel usa ese registro; sin id, el botón queda deshabilitado con el motivo ("Historial desactivado" / "No se guardó en el historial").
11. **Maqueta navegable** con datos simulados, para la aprobación visual del usuario antes de cablear.
12. Copiar resumen (Clipboard API con `execCommand('copy')` de respaldo) y Repetir (`startTest`, `startProbe(kind, target_id)`, `startInternet`; la velocidad abre de nuevo el modal), solo en el frontend.

**Excluye**: mapa y topología; protocolo, CLI y mediciones; métodos nuevos en el puente; estado de reinicio del servidor en State (G1) y cuota M-Lab restante (G3); fuentes embebidas, cambios de CSP y selector manual de tema; origen distinto de "Este equipo"; pruebas programadas y alertas; icono y empaquetado (spec 007).

## Criterios de aceptación

### CI (automáticos)
1. `go vet` y `go test ./...` pasan en Windows y en ubuntu `-race`.
2. Se mantienen los invariantes de `internal/ui/ui_test.go`: CSP idéntica en el meta y en la cabecera, sin `innerHTML` ni equivalentes, sin `setAttribute` (salvo en `chart.js`), sin `.href`/`.src`/`on*=`, sin URLs externas ni `@font-face`, y `mock.js` nunca se sirve. Los tests atados a los IDs antiguos se reescriben sin debilitar esos invariantes.
3. Todo JS nuevo de `internal/ui/web/` está embebido, en `servidos`, cargado en `index.html` y cubierto por el test anti-DOM inseguro.
4. Cada `nperf.<método>(` del JS está en la lista blanca del puente, y cada método de la lista se usa desde el JS.
5. Test de contraste sobre los tokens de `app.css`, en ambos temas: ≥ 4,5:1 para el texto (primario, secundario y apagado sobre canvas, surface y sidebar), para ok/warn/err sobre surface y sus fondos `-bg`, y para el texto de los botones primarios sobre el acento.
6. `startSpeed(true)` solo aparece en el manejador de "Aceptar y medir"; `startSpeed(false` no aparece y `mock.js` no llama a `startSpeed(`.
7. La clave nunca llega al DOM (solo el badge de `has_key`). El resumen copiado no contiene MAC, BSSID ni clave: test con node sobre la función que lo arma, usando un estado que incluye esos datos.
8. G2: tests en `internal/app` de que `history_id` coincide con el registro guardado de un TEST!, de un diagnóstico y de una prueba de internet; queda vacío con el historial desactivado o con un fallo al guardar; se vacía al borrar ese registro o todo el historial. `mock.js` lo simula.
9. Con node: las funciones de render de KPI y de resultados no fallan con informes parciales (cancelado, sin rtt, tcp o udp) y muestran `—` en lugar de 0 si la métrica no se midió.

### Manuales (Windows 10 y 11, `docs/validacion-gui-v2.md`)
10. Maqueta aprobada por el usuario en claro y oscuro, a 1000×640 y a 720×480, antes de cablear.
11. Sin internet: sin errores de CSP en DevTools y sin peticiones externas.
12. Claro y oscuro siguen al sistema al cambiarlo en caliente.
13. A 720×480 no hay scroll horizontal, filtros cortados ni texto superpuesto. A 1000×640, tras un TEST!, los 4 KPI y la gráfica se ven sin scroll.
14. La barra lateral solo contiene equipos y "+ Añadir equipo"; cambiar de un drawer al otro no hace parpadear el fondo.
15. Polling: con un drawer, un modal, un acordeón o el mini-formulario abiertos, nada se cierra solo; al escribir en duración, alias o filtros no se pierde el texto; durante 1 min en reposo, la selección de texto y el scroll se mantienen.
16. Los botones que guardan, borran o exportan pasan a "ocupado"; con dos clics rápidos en Exportar solo se abre un diálogo nativo.
17. Escape cierra en el orden popover → modal → drawer; el teclado llega a todo y el foco siempre se ve.
18. TEST! → Cancelar → Cancelando… → TEST!; con un diagnóstico en curso, TEST! queda deshabilitado con el motivo de Go mostrado una sola vez (y al revés).
19. Velocidad a internet y su Repetir abren siempre el modal; Cancelar no genera tráfico.
20. Copiar resumen pega texto plano con los KPI y el diagnóstico en el Bloc de notas.
21. El indicador muestra "Activo" y pasa a "con error" (clic → Ajustes) con un error de configuración forzado.
22. Con el historial desactivado, Exportar del Panel aparece deshabilitado con el motivo y el Historial muestra "Activar en Ajustes"; con él activado, Exporta el registro de la prueba que se ve.

## Discrepancias resueltas

| # | Punto del documento | Decisión |
|---|---|---|
| 1 | "Reiniciando :5300" (G1) | Rechazado: indicador Activo / con error (`config_error`); el reinicio se ve en el banner de Ajustes |
| 2 | Aviso de reinicio por historial o destinos | Solo "Guardado" (Go no reinicia en esos casos) |
| 3 | `firewall_hint` booleano | Es texto de Go: se muestra tal cual |
| 4 | Cuota M-Lab restante (G3) | Texto fijo "límite de 40 pruebas al día" |
| 5 | Exportar desde el Panel | G2 aprobado (`history_id`) |
| 6 | Copiar resumen | Solo frontend; sin MAC, BSSID ni clave |
| 7 | Repetir | Solo frontend; la velocidad vuelve a pedir el consentimiento |
| 8 | Progreso "reportado por Go" | `label` + tiempo transcurrido |
| 9 | BSSID/MAC | Solo en el drawer local |
| 10 | "Wi-Fi 6" en el botón Red | `state.self` + estándar traducido en JS |
| 11 | Fuentes Inter/JetBrains Mono | Solo fuentes de Windows; CSP sin cambios |
| 12 | Tokens sin AA | Se ajustan solo esos; test de contraste |
| 13 | `.theme-dark/.theme-light` | Sin clases manuales: solo automático |
| 14 | Paleta nueva y TEST! solo en el Panel | Aprobado; §7 actualizado |
| 15 | "Este equipo" con loopback | No seleccionable |
| 16 | `innerHTML` | Prohibido en todo uso (test actual); ARIA por propiedades |
| 17 | Gráfica TCP ida+vuelta | Se combinan en JS; el resto, en Modo detallado |
| 18 | "Historial (N)" | `history({})` en momentos fijos, sin campo nuevo |

# 008 - Plan

## Enfoque
- **Go (G2)**: `saveRecord` pasa a devolver `(id, error)` (`Store.Save` ya devuelve el id). Los tres puntos de guardado (`test.go`, `probe.go`, `internet.go`) escriben el id en `TestState.HistoryID` / `ProbeState.HistoryID` (`json:"history_id,omitempty"`) antes de publicar el estado final. `DeleteHistory` y `DeleteAllHistory` vacían el id si borran ese registro. Sin cambios en el puente ni en la CSP.
- **Maqueta primero** (puerta visual): `tools/uipreview` (Go estándar, solo desarrollo) sirve `internal/ui/web/` desde disco en 127.0.0.1 con la misma CSP: `/maqueta` es `tools/uipreview/maqueta.html`, con contenido estático de ejemplo y las clases reales, que carga el `app.css` y el `shell.js` reales; `/app` es `index.html` con `mock.js` inyectado antes del resto. La maqueta no se embebe. Tras la aprobación, su marcado pasa a `index.html`.
- **Frontend por módulos** (scripts clásicos, sin módulos ES; orden de carga fijo): `chart.js` → `app.js` (núcleo) → `shell.js` → `sidebar.js` → `panel.js` → `drawers.js` → `modals.js` → `internet.js` → `history.js`.
  - `app.js`: puente, polling de 500 ms, estado local (`ui`), `renderIf(clave, datos, fn)` con firma `JSON.stringify`, `setValueIfIdle(input, v)`, toasts, `busy(btn, promesa)` y formateadores (`—` para no medido).
  - `shell.js`: cabecera, vistas Panel/Historial, drawers, popover, pila de Escape y banners.
  - `sidebar.js`: equipos, selección y mini-formulario.
  - `panel.js`: barra de TEST! y diagnósticos, progreso, resultados de TEST!/diagnósticos (KPI, gráfica combinada, detalle), `buildSummary(state)` pura, Repetir y Exportar.
  - `drawers.js`: Tarjetas de red (polling solo abierto) y Ajustes.
  - `modals.js`: modal genérico (foco atrapado, Escape), exportación y confirmación de borrado.
  - `internet.js`: consentimiento M-Lab y resultados de internet/velocidad (sin duplicar hallazgos).
  - `history.js`: vista Historial y registro en solo lectura.
- **Tests**: `internal/ui/ui_test.go` mantiene los invariantes de seguridad y suma el test de contraste (parser de `--token: #hex|rgba()` por tema; rgba se compone sobre su fondo) y tests con node (`vm`, como `TestReportViewPartial`; se saltan sin node, que los runners de CI traen).

## Archivos y paquetes
- Go: `internal/app/{app.go,probe.go,test.go,internet.go,history.go}` + tests; `internal/gui/static.go` (`servidos`), `internal/ui/embed.go`, `internal/ui/ui_test.go`, `tools/uipreview/{main.go,maqueta.html}` + test.
- Web: `internal/ui/web/{index.html,app.css,app.js,shell.js,sidebar.js,panel.js,drawers.js,modals.js,internet.js,history.js,mock.js}`; `chart.js` solo si hace falta una serie extra.
- Docs: `docs/validacion-gui-v2.md`.

## Riesgos
- **Reescritura grande**: se reemplaza la UI completa. Mitigación: la vieja sigue funcionando hasta el cambio de `index.html` (T8); cada módulo tiene su test con node y el mock cubre todos los estados.
- **Tests atados a IDs antiguos** (`TestNetworkDetectionUI`, `TestHistoryUI`, `TestInternetUI`): se reescriben en la tarea que mueve cada parte, nunca se borran sin reemplazo.
- **Contraste**: los tokens rgba dependen del fondo; el test los compone sobre `--bg-surface`. Algún ajuste puede cambiar el aspecto aprobado en la maqueta, así que el test de contraste va antes de la maqueta.
- **1000×640 sin scroll**: la altura útil es justa (~600 px). La maqueta lo comprueba antes de cablear.
- **Clipboard en WebView2**: depende del gesto del usuario; hay respaldo con `execCommand` y validación manual.
- **Carrera de `history_id`**: el id debe estar asignado antes de pasar a `done`; el test lo comprueba en el primer `State()` con el estado final.

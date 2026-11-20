# Validación visual de la interfaz v2 (spec 008, T5: maqueta)

Maqueta estática: `tools/uipreview/maqueta.html` (+ `maqueta.js`), con el `app.css` y el `shell.js` reales. Ver con `go run ./tools/uipreview` y abrir `/maqueta` (selector de escenarios abajo a la izquierda, o `#escenario` en la URL; `?captura` lo oculta). **Pendiente de aprobación del usuario** (criterio 10).

## Comprobación automática

`NODE_PATH=$(npm root -g) node tools/uipreview/capturar.js` genera todas las capturas (claro y oscuro, 1000×640 y 720×480, factor 2) y comprueba:

- a 720×480: `document.documentElement.scrollWidth <= innerWidth` en los 24 escenarios;
- a 1000×640: la caja de `.kpis` y la de la gráfica (`getBoundingClientRect().bottom`) caben en `innerHeight` en resultado, modo detallado, sin historial, popover y modal.

Resultado: **OK** (sin scroll horizontal a 720×480; a 1000×640 KPI terminan en y=444 y la gráfica en y=620 de 640; con el aviso de "historial desactivado" la gráfica termina en 639, justo en el límite). Sin errores de consola.

## Escenarios principales

### Panel con resultado de TEST! (semáforo, 4 KPI, gráfica)

**claro**

![Panel con resultado de TEST! (semáforo, 4 KPI, gráfica), claro, 1000x640](img/gui-v2/resultado-claro-1000x640.png)

![Panel con resultado de TEST! (semáforo, 4 KPI, gráfica), claro, 720x480](img/gui-v2/resultado-claro-720x480.png)

**oscuro**

![Panel con resultado de TEST! (semáforo, 4 KPI, gráfica), oscuro, 1000x640](img/gui-v2/resultado-oscuro-1000x640.png)

![Panel con resultado de TEST! (semáforo, 4 KPI, gráfica), oscuro, 720x480](img/gui-v2/resultado-oscuro-720x480.png)

### Panel en curso (fases, tiempo)

**claro**

![Panel en curso (fases, tiempo), claro, 1000x640](img/gui-v2/curso-claro-1000x640.png)

![Panel en curso (fases, tiempo), claro, 720x480](img/gui-v2/curso-claro-720x480.png)

**oscuro**

![Panel en curso (fases, tiempo), oscuro, 1000x640](img/gui-v2/curso-oscuro-1000x640.png)

![Panel en curso (fases, tiempo), oscuro, 720x480](img/gui-v2/curso-oscuro-720x480.png)

### Historial (filtros, lote, tabla)

**claro**

![Historial (filtros, lote, tabla), claro, 1000x640](img/gui-v2/historial-claro-1000x640.png)

![Historial (filtros, lote, tabla), claro, 720x480](img/gui-v2/historial-claro-720x480.png)

**oscuro**

![Historial (filtros, lote, tabla), oscuro, 1000x640](img/gui-v2/historial-oscuro-1000x640.png)

![Historial (filtros, lote, tabla), oscuro, 720x480](img/gui-v2/historial-oscuro-720x480.png)

### Drawer Tarjetas de red

**claro**

![Drawer Tarjetas de red, claro, 1000x640](img/gui-v2/red-claro-1000x640.png)

![Drawer Tarjetas de red, claro, 720x480](img/gui-v2/red-claro-720x480.png)

**oscuro**

![Drawer Tarjetas de red, oscuro, 1000x640](img/gui-v2/red-oscuro-1000x640.png)

![Drawer Tarjetas de red, oscuro, 720x480](img/gui-v2/red-oscuro-720x480.png)

### Drawer Ajustes

**claro**

![Drawer Ajustes, claro, 1000x640](img/gui-v2/ajustes-claro-1000x640.png)

![Drawer Ajustes, claro, 720x480](img/gui-v2/ajustes-claro-720x480.png)

**oscuro**

![Drawer Ajustes, oscuro, 1000x640](img/gui-v2/ajustes-oscuro-1000x640.png)

![Drawer Ajustes, oscuro, 720x480](img/gui-v2/ajustes-oscuro-720x480.png)

### Modal de consentimiento M-Lab

**claro**

![Modal de consentimiento M-Lab, claro, 1000x640](img/gui-v2/mlab-claro-1000x640.png)

![Modal de consentimiento M-Lab, claro, 720x480](img/gui-v2/mlab-claro-720x480.png)

**oscuro**

![Modal de consentimiento M-Lab, oscuro, 1000x640](img/gui-v2/mlab-oscuro-1000x640.png)

![Modal de consentimiento M-Lab, oscuro, 720x480](img/gui-v2/mlab-oscuro-720x480.png)

## Otros escenarios

Mismo patrón de nombre en `docs/img/gui-v2/<escenario>-<claro|oscuro>-<ancho>x<alto>.png`:

- `detallado`: Modo detallado
- `parcial`: TEST! cancelado (—, "No medido")
- `cola`: En cola
- `diagnostico`: Diagnóstico en curso
- `ping`: Resultado de Ping
- `internet`: Resultado de Internet
- `velocidad`: Resultado de velocidad
- `vacio`: Sin equipos + aviso de cortafuegos
- `anadir`: Mini-formulario Añadir equipo
- `sinhist`: Exportar deshabilitado con motivo (historial desactivado)
- `popover`: Popover ⓘ
- `historial-registro`: Registro en solo lectura
- `historial-desactivado`: Historial desactivado
- `exportar`: Modal de exportación
- `borrar`: Modal de borrado
- `red-inactivas`: Drawer Red con inactivas abiertas
- `ajustes-error`: Ajustes: error al guardar
- `error`: Servidor con error (config_error)

## Notas

- Las fuentes de las capturas son las de Linux (Cascadia Mono/Segoe UI no están): en Windows las cifras en mono serán algo más estrechas.
- Los iconos de los botones (↻ ⤓ ⧉ ⓘ ⚙) son glifos Unicode; el icono de "Red" es provisional.
- Las gráficas de la maqueta son SVG de ejemplo (ejes en HTML); la real usará `chart.js`.

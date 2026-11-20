# NPerf — Especificación de Diseño y Comportamiento de GUI (`gui-parameters.md`)

> Documento de especificación UI/UX y comportamiento para implementar el rediseño de `internal/ui/web/`.  
> Diseñado para ser consumido directamente por un agente o desarrollador sin ambigüedades.

---

## 1. Contexto Técnico y Restricciones Obligatorias

Antes de escribir código, respeta estrictamente estas restricciones de arquitectura:

1. **Entorno:** Windows 10/11 sobre **WebView2** con HTML, CSS y JS puro embebido en el binario Go (`internal/ui/web/`).
2. **CSP Estricta (Sin CDNs externos):** Prohibido usar Tailwind CDN, Google Fonts CDN o librerías externas por red. Todo el CSS debe escribirse en hojas locales con **CSS Custom Properties** (`--var`). Si se usan fuentes locales (`Inter`, `JetBrains Mono`), deben estar embebidas en `internal/ui/web/fonts/` en formato `.woff2`, con *fallback* inmediato a fuentes nativas de Windows 11 (`Segoe UI Variable`, `Cascadia Mono`).
3. **Contrato con Go (`bridge.go`):** Toda comunicación ocurre vía `nperfCall(secreto, método, ...args)` usando únicamente la lista blanca existente:
   `state`, `selectPeer`, `setDirection`, `setDuration`, `setAlias`, `setKey`, `addSubnet`, `removeSubnet`, `startTest`, `cancelTest`, `startProbe`, `cancelProbe`, `adapters`, `monitor`, `addHost`, `removeHost`, `history`, `historyRecord`, `deleteHistory`, `deleteAllHistory`, `exportHistory`, `setHistoryEnabled`, `setInternetTargets`, `startInternet`, `startSpeed`, `openPrivacy`, `openLocationSettings`.
4. **Reglas de negocio inmutables:**
   - **Exclusión mutua:** `TEST!` y Diagnósticos (`Ping`, `Traceroute`, `MTU`, `Puertos`, `Internet`, `Velocidad`) nunca corren a la vez. Respeta `can_test`, `test_blocked_reason`, `can_probe`, `probe_blocked_reason` tal como llegan desde Go (no inventar textos de bloqueo en el frontend).
   - **Seguridad:** `has_key` es booleano; la clave compartida es de solo escritura y jamás se muestra.
   - **Errores de configuración:** `config_error` debe mostrarse siempre como **banner persistente** debajo del header (nunca como toast temporal).
   - **Consentimiento M-Lab:** Obligatorio en cada ejecución de `Velocidad a internet`.

---

## 2. Sistema de Diseño: NOC Telemetry + Precision Slate

Estilo visual: herramienta técnica moderna de precisión. Fondos neutros con contraste controlado, bordes finos de `1px` en lugar de sombras pesadas, números tabulares y color reservado exclusivamente para acciones principales, gráficas y semáforo de salud.

### 2.1 Tokens de Color (CSS Variables)

El tema debe seguir automáticamente `@media (prefers-color-scheme: dark)` y soportar clase explícita `.theme-dark` / `.theme-light` en `<html>`.

```css
:root {
  /* === MODO CLARO (Precision Slate + NOC Telemetry) === */
  --bg-canvas: #f4f6f8;         /* Fondo base de la app */
  --bg-sidebar: #eef1f5;        /* Columna izquierda de equipos */
  --bg-surface: #ffffff;        /* Tarjetas, header, drawers, modales */
  --bg-surface-hover: #f8fafc;  /* Hover en filas/tarjetas */
  --bg-input: #f8fafc;          /* Inputs y selects */
  --bg-subtle: #e9eef4;         /* Fondos de badges neutros / barras vacías */

  --border-subtle: #dde2e7;     /* Bordes de 1px entre paneles y tarjetas */
  --border-strong: #cbd5e1;     /* Bordes de inputs y botones secundarios */

  --text-primary: #0f172a;      /* KPIs, nombres de equipo, títulos */
  --text-secondary: #475569;    /* Textos de apoyo, etiquetas */
  --text-muted: #64748b;        /* Metadatos, placeholders, unidades */

  /* Acento principal (NOC Cyan / Precision Blue) */
  --accent-primary: #0284c7;
  --accent-primary-hover: #0369a1;
  --accent-primary-soft: rgba(2, 132, 199, 0.10);
  --accent-primary-border: rgba(2, 132, 199, 0.35);

  /* Semántica / Semáforo */
  --ok-fg: #059669;
  --ok-bg: rgba(5, 150, 105, 0.12);
  --ok-border: rgba(5, 150, 105, 0.30);

  --warn-fg: #b45309;
  --warn-bg: rgba(217, 119, 6, 0.12);
  --warn-border: rgba(217, 119, 6, 0.35);

  --err-fg: #dc2626;
  --err-bg: rgba(220, 38, 38, 0.10);
  --err-border: rgba(220, 38, 38, 0.32);

  /* Series de gráficas (evitar rojo/verde para no confundir con semáforo) */
  --chart-fwd: #0284c7;         /* TCP/UDP Ida (Cian técnico) */
  --chart-rev: #7c3aed;         /* TCP/UDP Vuelta (Violeta técnico) */
  --chart-rtt: #f59e0b;         /* RTT bajo carga (Ámbar) */
  --chart-grid: #e2e8f0;

  /* Overlay y sombras mínimas */
  --backdrop: rgba(15, 23, 42, 0.45);
  --shadow-drawer: -8px 0 24px rgba(15, 23, 42, 0.14);
  --shadow-modal: 0 16px 40px rgba(15, 23, 42, 0.22);
}

@media (prefers-color-scheme: dark) {
  :root {
    /* === MODO OSCURO (NOC Telemetry + Precision Slate) === */
    --bg-canvas: #0f1217;
    --bg-sidebar: #13171e;
    --bg-surface: #171c24;
    --bg-surface-hover: #1d232d;
    --bg-input: #11151c;
    --bg-subtle: #202733;

    --border-subtle: #262e3a;
    --border-strong: #354052;

    --text-primary: #f1f5f9;
    --text-secondary: #cbd5e1;
    --text-muted: #8b949e;

    --accent-primary: #0ea5e9;
    --accent-primary-hover: #38bdf8;
    --accent-primary-soft: rgba(14, 165, 233, 0.14);
    --accent-primary-border: rgba(14, 165, 233, 0.40);

    --ok-fg: #10b981;
    --ok-bg: rgba(16, 185, 129, 0.14);
    --ok-border: rgba(16, 185, 129, 0.32);

    --warn-fg: #f59e0b;
    --warn-bg: rgba(245, 158, 11, 0.14);
    --warn-border: rgba(245, 158, 11, 0.35);

    --err-fg: #ef4444;
    --err-bg: rgba(239, 68, 68, 0.14);
    --err-border: rgba(239, 68, 68, 0.35);

    --chart-fwd: #0ea5e9;
    --chart-rev: #a78bfa;
    --chart-rtt: #fbbf24;
    --chart-grid: #242c38;

    --backdrop: rgba(4, 7, 12, 0.62);
    --shadow-drawer: -12px 0 32px rgba(0, 0, 0, 0.45);
    --shadow-modal: 0 20px 50px rgba(0, 0, 0, 0.55);
  }
}
```

### 2.2 Tipografía y Escala

```css
:root {
  --font-sans: 'Inter', 'Segoe UI Variable', 'Segoe UI', system-ui, sans-serif;
  --font-mono: 'JetBrains Mono', 'Cascadia Mono', 'Consolas', ui-monospace, monospace;

  --radius-sm: 6px;   /* Botones, inputs, badges */
  --radius-md: 8px;   /* Tarjetas de equipo, bloques */
  --radius-lg: 10px;  /* Tarjetas KPI, contenedores, modales */

  --z-header: 20;
  --z-popover: 30;
  --z-drawer: 40;
  --z-modal: 50;
  --z-toast: 60;
}
```

**Reglas tipográficas obligatorias:**
- Todo dato numérico, IP, puerto, velocidad (`Mbps`/`Gbps`), latencia (`ms`), porcentaje (`%`), MAC y fecha debe usar `font-family: var(--font-mono)` y `font-variant-numeric: tabular-nums`.
- Encabezados de sección (`ESTE EQUIPO`, `EN LA RED`, `SIN NPERF`, `DIAGNÓSTICO`): `10px`–`11px`, peso `600`, `text-transform: uppercase`, `letter-spacing: 0.05em`, color `--text-muted`.
- KPIs principales: `20px`–`22px`, peso `700`, `--font-mono`, `line-height: 1.1`.
- Texto base de interfaz: `12px`–`13px`, `line-height: 1.4`.
- Tamaño mínimo de objetivo interactivo (botones, inputs, checkboxes con label): altura mínima `32px` (`28px` solo en acciones terciarias compactas dentro de tablas).

---

## 3. Arquitectura de Layout (*App Shell*) y Ventana

Se adopta definitivamente la **Opción B: Paneles Deslizables (*Drawers*)**.

### 3.1 Estructura de la Ventana (`1000×640` por defecto, mínimo `720×480`)

```
┌─────────────────────────────────────────────────────────────────────────┐
│ HEADER GLOBAL (48px fijo)                                               │
│ [N] NPerf | [Panel] [Historial (N)]      [● Servidor :5300] [Red] [⚙]  │
├──────────────────┬──────────────────────────────────────────────────────┤
│ BANNER PERSISTENTE (solo si existe config_error o firewall_hint)        │
├──────────────────┼──────────────────────────────────────────────────────┤
│ SIDEBAR EQUIPOS  │ ÁREA PRINCIPAL (flex: 1, min-width: 0)               │
│ (260px fijo en   │                                                      │
│  >= 860px;       │  - Vista "Panel":                                    │
│  220px en        │      1) Barra contextual TEST! + Diagnósticos        │
│  720px–859px)    │      2) Barra de progreso (solo si running/queued)   │
│                  │      3) Resultados (Pirámide Invertida)              │
│  - Este equipo   │                                                      │
│  - En la red     │  - Vista "Historial":                                │
│  - Sin NPerf     │      1) Barra de filtros (sin recortes)              │
│  - [+ Añadir]    │      2) Acciones en lote (Exportar / Borrar)         │
│                  │      3) Tabla o Vista de Registro (solo lectura)     │
└──────────────────┴──────────────────────────────────────────────────────┘
                        ▲ Overlay Drawer Derecho (380px–420px max)
                          para "Tarjetas de red" o "Ajustes"
```

### 3.2 Reglas de Comportamiento de Ventana y *Breakpoints*

- **Sin scroll horizontal global:** `body` y `#app` deben usar `height: 100vh; overflow: hidden;` y delegar el scroll independientemente al **Sidebar** y al **Área Principal**.
- **Ancho `>= 900px` (Estándar `1000×640`):**
  - Sidebar fijo de `260px`.
  - KPIs de resultado en grid de `4 columnas` (`repeat(4, minmax(0, 1fr))`).
  - Drawer derecho de `400px` de ancho.
- **Ancho `720px – 899px` (Ventana mínima `720×480`):**
  - Sidebar se reduce a `220px`.
  - KPIs de resultado pasan a grid de `2 columnas` (`repeat(2, minmax(0, 1fr))`).
  - Los botones de utilidad del header (`Red`, `Ajustes`) muestran solo icono + tooltip si falta espacio, manteniendo `Servidor :5300` visible.
  - Los filtros de Historial usan `flex-wrap: wrap` (nunca `overflow: hidden` que corte botones como ocurría antes).
  - Drawer derecho ocupa `min(380px, 85vw)`.
- **Atajos de teclado globales:**
  - `Escape`: Cierra en orden de prioridad: 1) Popover abierto → 2) Modal activo (excepto durante exportación bloqueante) → 3) Drawer abierto.
  - `Foco visible`: Todo elemento interactivo debe tener `:focus-visible { outline: 2px solid var(--accent-primary); outline-offset: 2px; }`.

---

## 4. Especificación Detallada por Componente

### 4.1 Header Global (`height: 48px`)
- **Izquierda:**
  - Logo compacto `NPerf`.
  - Navegación de 2 vistas principales: `Panel` y `Historial` (con contador discreto de registros si está disponible).
  - *Nota:* Al cambiar a `Historial`, la barra de controles de `TEST!` desaparece porque pertenece únicamente a la vista `Panel`.
- **Derecha:**
  - **Indicador de Servidor Local:** Píldora siempre visible con punto de estado (`6px`):
    - Normal: `● Servidor :5300 · Activo` (verde `--ok-fg`).
    - Reiniciando (al guardar Ajustes): `● Reiniciando :5300...` (ámbar `--warn-fg` con animación pulso).
    - Error (`config_error` presente): `● Servidor con error` (rojo `--err-fg`), clicable para abrir Ajustes.
  - **Separador vertical** `1px`.
  - **Botón `Red` (Drawer Tarjetas de red):** Muestra icono + resumen corto de la interfaz activa (ej. `Wi-Fi 6` o `Ethernet`). Al hacer clic abre/cierra el Drawer de `Tarjetas de red`.
  - **Botón `Ajustes` (Drawer Ajustes):** Icono de engranaje + texto `Ajustes`. Al hacer clic abre/cierra el Drawer de `Ajustes`. Si el otro drawer estaba abierto, reemplaza su contenido suavemente sin parpadear el backdrop.

### 4.2 Sidebar de Equipos (Única responsabilidad: Equipos)
**Eliminar del sidebar:** Los acordeones antiguos de *Tarjetas de red* y *Ajustes*, y los inputs permanentes de alta manual.

1. **Sección `ESTE EQUIPO`:**
   - Tarjeta destacada (no seleccionable como destino de test salvo si se permite loopback explícito, o diferenciada con borde sutil): Alias, badge `disponible`, IP en mono, y una línea compacta de enlace (`Wi-Fi 6 · ↑721 / ↓961 Mbps`).
2. **Sección `EN LA RED`:**
   - Lista de `peers[]`. Cada tarjeta muestra:
     - Fila 1: Icono de medio (Wi-Fi / Ethernet / desconocido) + Nombre (con `title` completo para tooltip si trunca) + Badge de estado (`disponible`, `ocupado`, `en cola N`, `inalcanzable`).
     - Fila 2: IP en `--font-mono` a la izquierda + velocidad de enlace compacta a la derecha (evitar concatenar SSID largo para que no se corte con `...`).
   - Estado `inalcanzable`: Borde discontinuo (`dashed`), opacidad `0.62`.
   - Estado seleccionado: Fondo `--accent-primary-soft` + borde `1px solid var(--accent-primary)`.
   - **Estado vacío:** Si `peers[]` está vacío, mostrar tarjeta informativa: *"No se detectan equipos con NPerf en esta red."* + si `firewall_hint` es verdadero, mostrar aviso accionable sobre puertos TCP/UDP `5300` y UDP `5301`.
3. **Sección `SIN NPERF`:**
   - Lista de `no_agent[]` (puerta de enlace y hosts manuales). Badge neutro `sin NPerf`. Los hosts manuales muestran acción `Quitar` discreta (visible en hover o foco, nunca robando espacio al nombre).
4. **Pie fijo del Sidebar (`+ Añadir equipo`):**
   - Botón de ancho completo con borde discontinuo: `+ Añadir equipo`.
   - Al hacer clic, despliega un **mini-formulario compacto** justo encima del botón con:
     - Input `IP o nombre` (requerido, foco automático).
     - Input `Alias (opcional)`.
     - Botones `Cancelar` y `Añadir` (con estado *busy* mientras responde `addHost`).

### 4.3 Vista Principal: `Panel`

#### A. Barra Contextual de Operación (Fija arriba del Panel)
Dividida en dos filas limpias dentro de una superficie `--bg-surface`:

- **Fila 1 (Medición `TEST!`):**
  - `Sentido`: `<select>` con las opciones de `directions[]` (fuente mono).
  - `Duración (s)`: `<input type="number" min="3" max="300">` compacto + etiqueta `total_label` (ej. `· 40 s total`).
  - A la derecha:
    - Si `!can_test`: mostrar `test_blocked_reason` en texto ámbar `11px` junto al botón deshabilitado (nunca duplicar el aviso en dos sitios).
    - Si `test.status` es `idle | done | error | cancelled`: botón primario **`TEST!`** (`--accent-primary`).
    - Si `test.status` es `queued | running`: botón destructivo **`Cancelar`** (`--err-fg`).
    - Si `test.status` es `cancelling`: botón deshabilitado **`Cancelando...`** con spinner.
- **Fila 2 (Herramientas de Diagnóstico):**
  - Grupo destino seleccionado: `Ping`, `Traceroute`, `MTU`, `Puertos`.
  - Separador vertical `1px`.
  - Grupo internet: `Internet`, `Velocidad a internet` + botón icono **`ⓘ`** pegado a su derecha.
  - **Eliminación de ruido:** Quitar la línea de texto permanente sobre M-Lab que aparecía arriba de todas las herramientas. En su lugar, el botón **`ⓘ`** abre un popover discreto explicando que NDT7 (M-Lab) publica la IP pública como datos abiertos (CC0) y solicita confirmación en cada prueba.
  - Si `!can_probe`: deshabilitar los botones de diagnóstico y mostrar `probe_blocked_reason` como tooltip y texto de estado único.

#### B. Barra de Progreso en Curso (Solo visible durante `queued`, `running`, `cancelling`)
- Franja horizontal limpia debajo de la barra contextual:
  - **En cola (`queued`):** Indicador ámbar + *"En cola en [Equipo] — Posición #[queue_position]"* + botón `Cancelar espera`.
  - **Midiendo (`running`):** Indicador cian animado + 5 chips de fase (`RTT`, `TCP Ida`, `TCP Vuelta`, `UDP Ida`, `UDP Vuelta`) marcando completada / activa / pendiente + tiempo transcurrido en `--font-mono` (sin ETA falsa).
  - **Diagnóstico en curso (`probe.status === 'running'`):** Indicador cian + nombre de herramienta + progreso reportado por Go + botón `Cancelar`.

#### C. Área de Resultados (Pirámide Invertida)

1. **Para resultados de `TEST!`:**
   - **Cabecera de resultado + Acciones a 1 clic:**
     - Izquierda: `Destino: Nombre (IP)` + badge de estado (`terminado` / `cancelado` / `error`) + línea técnica de enlace PHY de ambos extremos.
     - Derecha: Botones compactos **`↻ Repetir`**, **`⤓ Exportar`** (abre Modal de Exportación) y **`⧉ Copiar resumen`** (copia texto plano con KPIs y diagnóstico al portapapeles).
   - **Bloque 1 — Diagnóstico (Semáforo):**
     - Tarjeta con borde izquierdo o borde completo según salud (`--ok`, `--warn`, `--err`), icono circular de estado, título resumido en negrita (ej. *"Limitado por enlace Wi-Fi"*) y *badges* compactos para hallazgos críticos (ej. `Bufferbloat +104 ms`).
     - Texto explicativo en lenguaje claro debajo, sin concatenar párrafos interminables.
   - **Bloque 2 — 4 Tarjetas KPI (Visibles sin scroll):**
     1. **TCP Ida (↑):** Valor grande en `Mbps`, `% PHY` en esquina superior derecha, barra de progreso comparativa respecto al máximo del test, y pie con `MB` + retransmisiones.
     2. **TCP Vuelta (↓):** Valor grande en `Mbps`, `% PHY`, barra comparativa (color `--chart-rev`), y pie con `MB` + retransmisiones.
     3. **RTT:** Valor medio grande en `ms`, sublínea `mín / máx`, y alerta en ámbar si hubo subida de latencia bajo carga.
     4. **UDP (Pérdida / Jitter):** Pérdida `%` y jitter `ms` destacados + resumen de ambos sentidos.
     - *Regla de datos faltantes:* Si una métrica es `not_measured` o `unavailable`, mostrar `—` (guion largo) con etiqueta *"No medido"* / *"No disponible"*, **nunca `0`**.
   - **Bloque 3 — Gráfica Principal (Visible en el primer pantallazo):**
     - Contenedor compacto (`height: 150px`–`180px`) con título, leyenda arriba a la derecha (`TCP Ida`, `TCP Vuelta`), ejes con unidades (`Mbps`, `s`) y grillas sutiles `--chart-grid`.
   - **Bloque 4 — Tablas Técnicas Completas (Controladas por toggle `Modo detallado`):**
     - Plegadas por defecto si `Modo detallado` está desactivado (o visibles al activar el switch en la cabecera de la sección).
     - Tablas con `max-width: 100%`, columnas numéricas alineadas a la derecha (`text-align: right`), fuente mono tabular, evitando grandes huecos vacíos.

2. **Para resultados de `Internet` y Diagnósticos (`Ping`, `Traceroute`, `MTU`, `Puertos`):**
   - Mantener la misma cabecera con botones **`↻ Repetir`**, **`⤓ Exportar`** y **`⧉ Copiar resumen`**.
   - **Corrección obligatoria en `Internet`:** El recuadro de `Diagnóstico combinado` muestra el veredicto y hallazgos una sola vez; **eliminar la lista de viñetas duplicada** debajo de la tabla de ruta cuando el mismo mensaje ya está en el diagnóstico combinado.

### 4.4 Vista: `Historial`
- **Sin barra de `TEST!` arriba.**
- **Barra de Filtros (Corregida):**
  - Usar CSS Grid o Flex con `flex-wrap: wrap; gap: 8px; align-items: end;` dentro de una tarjeta `--bg-surface` con `padding: 12px`.
  - Campos: `Equipo`, `Tipo`, `Desde`, `Hasta`, y botones `Filtrar` y `Limpiar`. **Nunca** usar alturas fijas con `overflow: hidden` que recorten los botones.
  - Eliminar el checkbox duplicado `Guardar resultados en el historial` de esta vista (pertenece solo a `Ajustes`). Si el historial está desactivado, mostrar un banner informativo discreto con enlace *"Activar en Ajustes"*.
- **Barra de Acciones en Lote:**
  - Izquierda: Checkbox `Seleccionar todo` + contador `X registros · Y seleccionados`.
  - Derecha: Botón primario `Exportar seleccionados (Y)` (deshabilitado si `Y === 0`, abre el Modal de Exportación) + botón `Borrar seleccionados` + botón destructivo `Borrar todo` (ambos abren Modal de Confirmación).
- **Tabla de Historial:**
  - **Corrección obligatoria en columna `Tipo`:** Usar `display: inline-flex; align-items: center; gap: 8px;` para que el badge (`terminado`, `cancelado`, `error`) **nunca se pegue** al texto del tipo (`Ping`, `MTU`, `Puertos`).
  - Fila con `error`: tinte sutil `--err-bg` en texto de estado y fecha, manteniendo legibilidad.
  - Botón `Abrir` abre la vista de registro individual en solo lectura con botón `← Volver al historial` y botón `Exportar este registro`.
- **Eliminar formulario de exportación embebido al pie:** Mover toda la exportación al **Modal de Exportación** (Sección 4.7).

### 4.5 Drawer Derecho: `Tarjetas de red`
- Ancho `400px` (`max-width: 90vw`), animación `transform: translateX(0)` en `200ms cubic-bezier(0.16, 1, 0.3, 1)`.
- **Polling inteligente:** Activar `adapters()` (5 s) y `monitor()` (1 s) **solo** cuando este drawer esté abierto (además de una carga inicial al arrancar para poblar el resumen del botón `Red` en el header).
- **Jerarquía de tarjetas (Progressive Disclosure):**
  1. **Interfaces activas primero:** Tarjeta destacada con borde `--accent-primary-border`, badge `Activa`, SSID/banda/canal, señal `%` y `dBm`, tasas `↑ / ↓` en tiempo real, IPv4, estándar, BSSID, MAC y nombre del adaptador en grilla de 2 columnas.
  2. **Interfaces inactivas o virtuales colapsadas por defecto:** Un acordeón cerrado al pie: *"Ver N interfaces inactivas o virtuales (Bluetooth, Ethernet, Tailscale...)"*. Al expandirlo, muestra tarjetas compactas con menor contraste (`opacity: 0.78`) y badge `desconectada` o `virtual`.

### 4.6 Drawer Derecho: `Ajustes`
- Organizado en tarjetas claras con espaciado de `12px` y validación en línea:
  1. **Banner de feedback de servicio:** Al guardar cualquier ajuste, mostrar banner superior en el drawer (`Aplicando y reiniciando servidor :5300...` → `Guardado · Servidor reiniciado` o error) sincronizado con el indicador del Header.
  2. **Alias:** Input + botón `Guardar` (deshabilitado si no cambió o está vacío).
  3. **Clave compartida:** Badge `Configurada` / `No configurada` (según `has_key`), texto explicativo *"Solo escritura; nunca se muestra"*, input `type="password"` + botón `Guardar`.
  4. **Historial local:** Switch `Guardar resultados en el historial` con nota de retención (`90 días / 50 MB`).
  5. **Destinos de latencia a internet (máx. 4):** Lista en mono con botón `Quitar` + input IPv4 pública con validación antes de enviar + botón `Añadir` + botón secundario `Restaurar valores por defecto`.
  6. **Subredes extra:** Lista CIDR con botón `Quitar` + input `192.168.x.0/24` + botón `Añadir`.

### 4.7 Modales

1. **Modal de Consentimiento M-Lab (Obligatorio antes de `startSpeed`):**
   - Título claro, párrafos concisos explicando publicación de IP pública (CC0), consumo de datos (~20 s) y límite diario (mostrar cuota restante si el backend la expone; si no, *"límite de 40 pruebas al día"*).
   - **Jerarquía corregida del pie:**
     - Izquierda: **`Política de M-Lab`** y **`Política de NPerf`** como **enlaces de texto discretos** (`text-decoration: underline`, color `--accent-primary`), **NO** como botones con borde que compitan con `Cancelar`.
     - Derecha: Botón secundario `Cancelar` + Botón primario `Aceptar y medir`.
2. **Modal de Exportación (Compartido por `Panel` e `Historial`):**
   - Selector `Formato` (`CSV para Excel (ES)`, `CSV estándar`, `JSON`, `HTML autocontenido`).
   - Inputs opcionales: `Cliente`, `Técnico`, `Notas`.
   - Checkbox: `Incluir intervalos (segundo CSV)` (solo habilitado cuando el formato es CSV).
   - Botones `Cancelar` y `Exportar...` (estado *busy* mientras el diálogo nativo de Windows está abierto para impedir dobles exportaciones).
3. **Modal de Confirmación de Borrado:**
   - Exige confirmación explícita indicando el alcance exacto (*"¿Borrar los 2 registros seleccionados?"* o *"¿Borrar todo el historial (8 registros)? Esta acción no se puede deshacer."*).

---

## 5. Matriz de Comportamiento de Botones y Acciones

| Elemento / Botón | Método Bridge Go | Condición de Deshabilitado | Estado *Busy* / Progreso | Feedback de Éxito / Error |
| :--- | :--- | :--- | :--- | :--- |
| **`TEST!`** | `startTest()` | `!can_test` o `status !== idle/done/error/cancelled` | Cambia a botón **`Cancelar`** y muestra barra de fases | Al terminar: renderiza KPIs + gráfica + toast *"TEST! completado"* |
| **`Cancelar` (Test)** | `cancelTest()` | `status === 'cancelling'` | Texto `Cancelando...` + deshabilitado | Vuelve a `TEST!` + toast *"Medición cancelada"* |
| **`Ping` / `Traceroute` / `MTU` / `Puertos`** | `startProbe(kind, ...)` | `!can_probe` o sin destino válido | Muestra barra de progreso con botón `Cancelar` | Renderiza resultado del diagnóstico en Panel |
| **`Internet`** | `startInternet()` | `!can_probe` | Barra de progreso de latencia | Renderiza diagnóstico combinado (sin duplicados) + tablas |
| **`Velocidad a internet`** | Abre Modal M-Lab → `startSpeed()` | `!can_probe` | En modal: cierra al confirmar y muestra progreso NDT7 | Renderiza velocidades M-Lab + atribución |
| **`Añadir` (Host manual)** | `addHost(ip, alias)` | Input IP vacío o envío en curso | Botón `Añadiendo...` deshabilitado | Limpia inputs, cierra mini-form, toast *"Equipo añadido"* |
| **`Quitar` (Host manual)** | `removeHost(id)` | Envío en curso | Opacidad `0.5` | Elimina de `Sin NPerf`, toast *"Equipo eliminado"* |
| **`Guardar` (Alias / Clave)** | `setAlias(v)` / `setKey(v)` | Envío en curso o valor inválido | Botón `Guardando...` + Header pasa a `Reiniciando :5300...` | Banner verde en Ajustes + Header vuelve a `Activo` |
| **Switch Historial** | `setHistoryEnabled(bool)` | Envío en curso | Deshabilitado durante llamada | Feedback de reinicio de servidor |
| **`Añadir` / `Quitar` (Destinos / Subredes)** | `setInternetTargets` / `addSubnet` / `removeSubnet` | Envío en curso o límite `4` alcanzado | Botón deshabilitado durante llamada | Lista actualizada + feedback de reinicio |
| **`Exportar...` (Modal)** | `exportHistory(opts)` | Exportación en curso (evita doble diálogo nativo) | Botón `Exportando...` deshabilitado | Cierra modal + toast con ruta exportada (o aviso si canceló) |
| **`Borrar` / `Borrar todo`** | Abre Modal → `deleteHistory` / `deleteAllHistory` | 0 seleccionados (para borrar selección) | Botón `Borrando...` en modal | Cierra modal, refresca tabla, toast *"Registros eliminados"* |
| **`Política de M-Lab` / `NPerf`** | `openPrivacy(kind)` | Nunca | — | Abre URL/política desde Go |

---

## 6. Reglas de Renderizado con Polling de 500 ms (Anti-Parpadeo)

Dado que la UI consulta `state()` cada **500 ms**, la implementación en JS **NO debe** hacer `innerHTML = ...` destructivo sobre toda la ventana en cada tick:

1. **Separar estado de UI local del snapshot de Go:**
   - Variables locales en memoria JS: `activeView` (`panel` | `history`), `activeDrawer` (`null` | `adapters` | `settings`), `isAddHostOpen`, `isDetailedMode`, `isInactiveAdaptersOpen`, `activeModal`, `pendingActions` (Set de botones en estado *busy*).
   - El polling de `state()` **nunca** cierra drawers, modales, popovers ni colapsa acordeones.
2. **Protección de inputs con foco:**
   - Antes de actualizar el valor de un `<input>` o `<select>` desde `state()` (por ejemplo `duration_seconds` o `alias`), comprobar `if (document.activeElement !== inputEl)`. Así el usuario nunca pierde lo que está escribiendo.
3. **Diffing ligero por hash/firma:**
   - Comparar `JSON.stringify` (o firma de campos relevantes) de cada subsección (`peers`, `test.report`, `probe.result`) antes de re-renderizar su DOM. Si no cambió desde el tick anterior, **no tocar el DOM** (cero parpadeos, cero pérdida de scroll ni selección de texto).

---

## 7. Checklist de Validación Antes de Entregar

- [ ] Funciona 100% offline dentro de WebView2 sin errores de CSP ni peticiones a CDNs.
- [ ] Tema claro y oscuro responden correctamente a `prefers-color-scheme` con contraste AA mínimo.
- [ ] En ventana mínima `720×480` no hay scroll horizontal global, ni filtros cortados, ni texto superpuesto.
- [ ] El sidebar solo contiene Equipos y el botón `+ Añadir equipo`; *Tarjetas de red* y *Ajustes* abren exclusivamente en el Drawer derecho.
- [ ] El indicador `Servidor :5300` es visible en todo momento en el Header.
- [ ] En `TEST!`, los 4 KPIs y la gráfica son visibles sin necesidad de scroll en `1000×640`.
- [ ] En `Historial`, los *badges* tienen separación clara del texto de `Tipo` y la exportación ocurre en Modal.
- [ ] Todos los botones que escriben configuración o exportan tienen estado *busy* que impide dobles envíos.
- [ ] `Escape` cierra popovers, modales y drawers en el orden correcto.
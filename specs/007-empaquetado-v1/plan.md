# 007 - Plan

## Enfoque

### 1. Saludo UDP
`greetUDP` (internal/client/client.go) da 10 intentos × 300 ms = 3 s, y cada lectura que no sea el acuse (p. ej. un datagrama de datos de rev) gasta un intento y reenvía al instante. Hipótesis a confirmar en T1, en este orden:
- **H1 (producto)**: el presupuesto se mide en intentos, no en tiempo; con CPU saturada (`-race`, paquetes en paralelo, UDP a varios Gbps en loopback por la regla del 90 % del TCP) el despachador del servidor (`dispatchUDP`) tarda > 3 s en procesar el saludo.
- **H2 (producto)**: datagramas ajenos al acuse consumen intentos sin esperar.
- **H3 (servidor)**: el socket del servidor aún tiene en cola datos de la fase anterior (búfer de 4 MB) y el saludo espera detrás.
T1 instrumenta (contadores en test, no logs en producción), reproduce con estrés (`-race -count`, `GOMAXPROCS` bajo y carga de CPU en paralelo) y escribe un test de regresión determinista con un servidor UDP falso (acuse retrasado, datagramas basura antes del acuse). Corrección prevista (T2): presupuesto por **tiempo** (plazo total ≤ 5 s, reenvío cada 300 ms con un tope de reenvíos), las lecturas que no son el acuse no consumen reenvíos ni cortan la espera, y el error final se mantiene. Si T1 muestra otra causa (p. ej. H3 o un fallo del test), el implementer se detiene y lo informa al coordinator antes de corregir. La causa y la corrección se anotan en §22.

### 2. Versión
- `version.Current` pasa de `const` a `var` y a **"1.0.0"**: sigue siendo la versión de protocolo (handshake, User-Agent de M-Lab, historial) y la fuente de la regla de compatibilidad.
- Nuevo `version.Build = "dev"` (inyectable: `-X nperf/internal/version.Build=X.Y.Z`) y `version.Display()`. Solo se muestra; nunca entra en el protocolo, así un `dev` habla con cualquier 1.x.
- Scripts de build: `git describe --tags --exact-match --match 'v[0-9]*'` → `X.Y.Z`; si falla, `dev`. Publicación: el tag debe ser exactamente `v` + `version.Current` (el bump de versión se hace en código antes del tag).

### 3. Icono (`tools/icongen`, solo biblioteca estándar)
Una sola geometría en Go (cuadrado redondeado azul #1E6FD9 con borde #0B4A9E y dos flechas blancas opuestas) de la que salen `assets/icon/nperf.svg`, un PNG por tamaño (rasterizado propio con supermuestreo 8×8; variantes de trazo más grueso en 16/20/24), `nperf.ico` (entradas PNG, válidas en Windows 10/11) y `preview.png` (todos los tamaños a 1:1 sobre #FFFFFF y #202020, más el de 256). No hay rasterizador SVG en la biblioteca estándar: el SVG es una salida, no una entrada; la fuente real es la geometría, y un test comprueba que todo está al día.

### 4. Recursos `.syso` (decisión §22)
Opciones evaluadas:
- **A, elegida**: `go run github.com/tc-hib/go-winres@v0.3.3 make` (MIT, mantenida, versión fija; `go run mod@versión` verifica el módulo con sum.golang.org y no toca `go.mod`). Configuración en `cmd/nperf/winres/winres.json` y `cmd/nperf-gui/winres/winres.json` (RT_GROUP_ICON `"#1"`, RT_VERSION; **sin** RT_MANIFEST para no cambiar el comportamiento de DPI/estilos). Salida `rsrc_windows_amd64.syso` en cada `cmd/*`, **commiteada** con versión `dev` (0.0.0.0): builds y tests normales no necesitan red ni la herramienta. La publicación la regenera con `--product-version X.Y.Z --file-version X.Y.Z.0`.
- B, descartada: generador propio de COFF con árbol de recursos y relocaciones `ADDR32NB`; ~300 líneas de formato binario delicado sin beneficio frente a A.
- C, descartada: añadir la herramienta como dependencia o `tools.go` (cambia `go.mod`).
Script único `scripts/gen-resources.sh [X.Y.Z]` (bash; también corre en el runner de Windows). Si T11 comprueba que v0.3.3 no es la última estable o que la salida no es determinista, se usa la última estable y se anota en §22; si no es determinista, se quita la comprobación de "al día" del CI.
La ventana usa el icono con `WindowOptions.IconId: 1` (go-webview2); la bandeja ya carga el recurso 1.

### 5. Publicación
- `tools/release` (Go, biblioteca estándar): `-version X.Y.Z -exe-dir D -out dist [-check-copyright]` valida archivos (y con `-check-copyright` rechaza el marcador `TITULAR-PENDIENTE` en los `winres.json`), crea el zip con `archive/zip` (orden y fechas fijas), `SHA256SUMS` (formato `sha256sum`) y `notes.md` con la sección `## [X.Y.Z]` del CHANGELOG (hasta la siguiente `## `). Testeable en Linux.
- `.github/workflows/release.yml`: `on: push: tags: ['v*.*.*']` + validación de formato estricto; job windows-latest: vet, `go test -p 1 ./...`, `gen-resources.sh X.Y.Z`, build con `-trimpath -ldflags "-X …Build=X.Y.Z"` (+ `-H=windowsgui` en la GUI), `nperf.exe version` = `NPerf X.Y.Z`, `tools/release`, `gh release create vX.Y.Z --draft --title "NPerf X.Y.Z" --notes-file dist/notes.md dist/*.zip dist/SHA256SUMS`. `permissions: contents: write` solo en ese job; sin acciones de terceros nuevas.
- `ci.yml`: build con los scripts/ldflags; job ubuntu `-race` añade el paso de estrés (criterio 3) y el paquete de prueba (criterio 10: compilación cruzada, `tools/release -version <Current>`, `sha256sum -c`).
- `docs/licencias-terceros.txt`: avisos de go-webview2 (MIT), go-winloader (ISC), golang.org/x/sys y Go (BSD-3) y WebView2Loader.dll (licencia del SDK de Microsoft), copiados de la caché de módulos.

## Archivos y paquetes
- `internal/client/client.go`, `greet_test.go` (saludo).
- `internal/version/version.go`, `version_test.go`; `cmd/nperf/main.go`, `cli_test.go`, `e2e_test.go` (comando `version`).
- `tools/icongen/` (main.go, raster.go, ico.go, icongen_test.go); `assets/icon/` (nperf.svg, nperf.ico, preview.png).
- `cmd/nperf/winres/winres.json`, `cmd/nperf-gui/winres/winres.json`, `cmd/*/rsrc_windows_amd64.syso`, `scripts/gen-resources.sh`, `scripts/build-windows.{sh,ps1}`.
- `internal/gui/window_windows.go` (IconId); `cmd/nperf-gui/pe_test.go` (recursos).
- `tools/release/` (main.go, release_test.go); `.github/workflows/ci.yml`, `release.yml`.
- `CHANGELOG.md`, `README.md`, `docs/licencias-terceros.txt`, `docs/validacion-empaquetado.md`, `docs/decisiones.md` §22.

## Riesgos
- Subir la versión de protocolo a 1.0.0 rompe la compatibilidad con builds 0.x ya repartidos (solo de desarrollo): se avisa en el CHANGELOG.
- El `.syso` commiteado puede quedar desfasado del icono o del `winres.json`: el CI regenera y compara (si la salida es determinista).
- El marcador de copyright debe sustituirse antes del primer tag: el workflow lo bloquea.
- go-winres se descarga en la publicación: si el proxy de Go no responde, el borrador no se crea (se reintenta empujando de nuevo el workflow, no el tag).
- La corrección del saludo podría alargar la detección de un UDP bloqueado de 3 s a ≤ 5 s: aceptable y documentado.
- Iconos con entradas PNG en tamaños pequeños: soportados desde Windows Vista; se valida a mano en el Explorador y la bandeja.

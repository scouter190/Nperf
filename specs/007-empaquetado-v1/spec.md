# 007 - Empaquetado v1

> Aprobada por el usuario (alcance acordado en el chat).

## Objetivo
Dejar NPerf listo para publicar la v1.0.0: CI estable, ejecutables con icono propio y datos de versión, y una publicación automatizada en GitHub que solo crea un borrador de Release cuando el usuario empuja un tag.

## Alcance
**Incluye**
1. **Saludo UDP estable bajo carga**: analizar el fallo intermitente "el servidor no respondió al saludo UDP" (TestLiveFullBoth en ubuntu `-race`; TestDiagnosisBothEnds en internal/app `-race`), decidir si es del producto o del test y corregir la causa sin ocultar fallos reales (sin subir solo plazos de test, sin reintentar tests, sin `t.Skip`).
2. **Icono propio**: diseño sencillo y genérico (sin marcas de terceros), legible a 16 px, válido en fondos claros y oscuros. Fuente SVG en el repo y `.ico` con 16, 20, 24, 32, 40, 48, 64 y 256 px. Vista previa PNG para que el usuario lo apruebe **antes** de embeberlo. Embebido como recurso de Windows (grupo de icono ID 1) en `nperf.exe` y `nperf-gui.exe`; la ventana y la bandeja usan ese icono.
3. **Versión**: versión mostrada inyectada con `-ldflags -X` desde el tag git (`vX.Y.Z` → `X.Y.Z`; sin tag, `dev`). Recurso VERSIONINFO (ProductName "NPerf", FileDescription, FileVersion/ProductVersion, OriginalFilename, LegalCopyright con el titular que indique el usuario). `nperf version` y la ayuda general muestran la versión. La versión de protocolo pasa a 1.0.0.
4. **Publicación**: workflow que, al empujar un tag `vX.Y.Z`, ejecuta vet y tests en windows-latest, construye ambos `.exe` con la versión, empaqueta `NPerf-X.Y.Z-windows-amd64.zip` (nperf.exe, nperf-gui.exe, README.md, privacidad.md, licencias de terceros y LICENSE si existe) con `SHA256SUMS`, y crea un Release **en borrador** con la sección del CHANGELOG. `CHANGELOG.md` con la 1.0.0 (specs 001-006 en lenguaje de usuario) y `README.md` breve.

**Excluye**: firma de código, instalador, auto-actualización, manual de usuario, rediseño de la UI (ni favicon), manifiesto de aplicación nuevo, otras pruebas intermitentes (p. ej. TestStreamsOutOfRangeRejected), crear tags o Releases reales (lo decide el usuario después).

## Criterios de aceptación

### CI (automáticos)
1. Un test de regresión reproduce la causa identificada del fallo del saludo UDP: falla con el código anterior (el reviewer lo comprueba revirtiendo solo el cambio de producto) y pasa con la corrección.
2. Con un puerto UDP que nunca responde, el cliente sigue fallando con "el servidor no respondió al saludo UDP (¿puerto UDP bloqueado?)" en un plazo acotado (≤ 6 s) y documentado.
3. El job ubuntu `-race` incluye `go test -race -count=10 -run 'TestLiveFullBoth|TestDiagnosisBothEnds' ./internal/client ./internal/app` y pasa; ningún test del saludo se salta, se reintenta ni amplía su plazo como única medida.
4. Sin `-X`, `nperf version` imprime `NPerf dev` y sale con 0; un binario compilado con `-X nperf/internal/version.Build=1.2.3` imprime `NPerf 1.2.3`. La primera línea de la ayuda general incluye la versión y la lista de comandos incluye `nperf version`.
5. El saludo de protocolo sigue usando la versión semver (`1.0.0`), no la mostrada: un binario `dev` y uno `1.x` interoperan; con uno `0.x` el saludo falla con el error de versión incompatible actual.
6. `assets/icon/nperf.ico` contiene exactamente los 8 tamaños, todos decodificables; existen `nperf.svg` y `preview.png`; regenerarlos con el generador da archivos idénticos byte a byte.
7. Contraste (calculado en test): fondo del icono frente a #FFFFFF y #202020 ≥ 3:1; símbolo frente al fondo del icono ≥ 4,5:1.
8. `nperf.exe` y `nperf-gui.exe` compilados con `GOOS=windows` contienen RT_GROUP_ICON con ID 1 y 8 imágenes y RT_VERSION con ProductName "NPerf" y su OriginalFilename; los subsistemas PE no cambian.
9. `go.mod` y `go.sum` no cambian en toda la spec; las herramientas nuevas usan solo la biblioteca estándar.
10. En cada push, el CI genera un paquete de prueba (sin publicar): el zip contiene exactamente los archivos listados, `SHA256SUMS` se verifica con `sha256sum -c` y las notas extraídas del CHANGELOG para la versión de protocolo no están vacías (sin sección → falla).
11. El workflow de publicación solo se dispara con tags `v<n>.<n>.<n>`; falla si el tag no coincide con la versión de protocolo o si el titular del copyright sigue sin definir; crea el Release con `--draft` y solo ese job tiene `contents: write`.
12. Durante la spec no se crea ningún tag ni Release (el reviewer comprueba `git tag` y la lista de Releases).

### Manuales (Windows, `docs/validacion-empaquetado.md`)
13. El usuario aprueba la vista previa del icono antes de embeberlo.
14. El Explorador muestra el icono de NPerf en ambos `.exe` (vistas pequeña, mediana y grande; tema claro y oscuro).
15. En `nperf-gui.exe`, la ventana, la barra de tareas y la bandeja muestran el icono de NPerf al 100 % y 150 % de escala.
16. Propiedades › Detalles muestra descripción, producto NPerf, versión y copyright.
17. El `nperf.exe` del CI dice `NPerf dev`; cuando el usuario empuje un tag, el exe del borrador dice `NPerf X.Y.Z` y el zip coincide con `SHA256SUMS`.
18. El usuario revisa que la entrada 1.0.0 del CHANGELOG se entiende sin conocimientos técnicos.

## Preguntas para el usuario
1. **Licencia**: no hay LICENSE. ¿Cuál quieres (p. ej. MIT) o ninguna (todos los derechos reservados)? Sin respuesta, el zip sale sin LICENSE.
2. **Titular del copyright** (VERSIONINFO): ¿qué nombre figura ("© 2026 …")? Hasta entonces queda un marcador y el workflow de publicación se niega a crear el borrador.
3. **Icono**: aprobación de `assets/icon/preview.png` (propuesta: cuadrado redondeado azul con dos flechas blancas opuestas, ida y vuelta).

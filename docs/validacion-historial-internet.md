# Validación manual de historial, exportación e internet (criterios 16-22, spec 006)

Objetivo: comprobar a mano, en Windows y sin administrador, que el historial persiste, que las exportaciones se abren bien y que las pruebas de internet (latencia y velocidad M-Lab) se comportan como pide la spec.

## 1. Requisitos
- Dos equipos Windows 10/11 con NPerf, **A** (cliente, `nperf-gui.exe` y `nperf.exe`) y **B** (servidor, `nperf.exe server` o `nperf-gui.exe`). Cuenta normal, sin "Ejecutar como administrador".
- Excel con configuración regional española (separador de lista `;`, decimal `,`) para el criterio 17.
- Un navegador y un visor/impresora PDF ("Microsoft Print to PDF") para el criterio 18.
- Para el criterio 19: acceso al router (desconectar el WAN) o un cable WAN desenchufable, y un enlace WiFi del que alejarse del punto de acceso.
- Para el criterio 20: otra prueba de velocidad web (p. ej. una web de test) y el Firewall de Windows en modo avanzado solo si tu cuenta puede crear reglas de salida sin administrador; si no, usa la red/router para bloquear el puerto 443 saliente.
- Carpeta del historial: `%LocalAppData%\NPerf\historial` (pega esa ruta en el Explorador o en Ejecutar; un JSON por resultado). El contador diario de M-Lab es `%LocalAppData%\NPerf\mlab-diario.json`.
- Equipo A: ______  Equipo B: ______  Windows: ______  Excel: ______

## 2. Criterios

### 16. Persistencia del historial
Objetivo: los resultados sobreviven al cierre de la app y el TEST! se ve igual que al terminar.
1. Abre la carpeta del historial y anota cuántos archivos hay (puede no existir aún).
2. En A abre `nperf-gui.exe`, elige B, duración 30 s, protocolo "Ambos" y pulsa **TEST!**. Espera al final y fíjate en las gráficas y cifras.
3. Lanza un ping y un traceroute (diagnósticos) a B o a un host conocido; espera a que terminen.
4. Cierra NPerf del todo (menú de la bandeja > Salir) y vuelve a abrir `nperf-gui.exe`.
5. Abre el historial y localiza los tres registros (TEST!, ping, traceroute). Abre el del TEST!.
6. En consola: `.\nperf.exe history list` y `.\nperf.exe history show <id>` con el id del TEST!. Sin registros la CLI dice `No hay registros en el historial.`

Esperado: los tres registros están; el TEST! muestra las mismas cifras y gráficas por fase que al terminar; la carpeta tiene un JSON por registro; la CLI los lista.
- [ ] OK  [ ] Fallo: ______

### 17. CSV en Excel en español
Objetivo: el CSV se abre sin pasos de importación.
1. En el historial de la ventana selecciona varios registros (incluido el TEST!) y exporta a CSV.
2. Haz doble clic en el archivo para abrirlo en Excel (configuración regional española).
3. Comprueba columnas separadas, cabeceras legibles y números con coma decimal tratados como número (se pueden sumar/ordenar).
4. Comprueba que ninguna celda de texto se interpreta como fórmula.
5. Opcional, por consola: `.\nperf.exe history export -all -format csv -csv excel-es -o %TEMP%\h.csv` (la variante `std` usa coma y punto, y `excel-es` punto y coma y coma decimal). Al acabar sale `Exportados N registro(s) en <ruta>`.

Esperado: abre directamente con una columna por campo y números correctos, sin asistente de importación.
- [ ] OK  [ ] Fallo: ______

### 18. Informe HTML, sin conexión y en PDF A4
Objetivo: informe legible sin red, en ambos temas.
1. Exporta registros a HTML desde la ventana (o `.\nperf.exe history export <id> -format html -o %TEMP%\informe.html`).
2. Desconecta el equipo de la red (modo avión o desconectar WiFi/cable) y abre el HTML en el navegador. El título es "Informe de red - NPerf".
3. Comprueba que se ven las gráficas, cada una con su título, y que no hay recursos que fallen al cargar.
4. Imprime a "Microsoft Print to PDF" con papel A4 y márgenes por defecto; abre el PDF y revisa que el texto y las gráficas se leen y no se cortan.
5. Repite 2-4 con el tema del sistema en claro y en oscuro.

Esperado: el informe se ve completo sin conexión; el PDF A4 es legible, sin cortes; ambos temas legibles y con los valores numéricos visibles sin depender del color.
- [ ] OK  [ ] Fallo: ______

### 19. Veredicto de latencia a internet
Objetivo: el veredicto distingue los cuatro escenarios. Usa el botón **Internet** de la ventana (o `.\nperf.exe internet`); sin administrador.
1. Red sana: pulsa **Internet**. Esperado: "sin problemas detectados".
2. Desconecta el WAN del router (la LAN y el equipo siguen conectados al router) y repite. Esperado: "sin salida a internet".
3. Reconecta el WAN. Aléjate del punto de acceso WiFi hasta degradar la señal (WiFi con pérdida) y repite. Esperado: "el problema está en la red local (cable/WiFi/router)".
4. Bloquea el ICMP saliente (regla de salida del firewall para ICMPv4, o una red que lo filtre) y repite. Esperado: "ICMP filtrado (sin veredicto de latencia)" usando el respaldo TCP 443 (la salida muestra `TCP 443` por destino).
5. Anota el texto exacto de cada veredicto. En la CLI aparece como `Veredicto: ...`; si el router no responde a ping se añade la nota "puerta de enlace no responde a ping".
6. Si el problema está después del router, el veredicto es "el problema está después del router (proveedor)" y se ejecuta el traceroute (opcional, solo si puedes provocarlo).

Esperado: los cuatro escenarios dan su veredicto; el resultado queda en el historial.
- [ ] OK  [ ] Fallo: ______

### 20. Velocidad a internet (M-Lab NDT7)
Objetivo: la prueba exige consentimiento, mide y no excede el límite.
1. Antes de pulsar, comprueba que junto a **Velocidad a internet** hay un aviso visible de que se publicará tu IP pública.
2. Pulsa **Velocidad a internet**. Debe abrirse el diálogo de consentimiento "Prueba de velocidad con M-Lab", con el aviso de IP pública/datos abiertos, el consumo de datos (cientos de MB) y el límite de 40 pruebas al día por usuario de Windows en este equipo, más dos enlaces (política de M-Lab y política de NPerf).
3. Rechaza (cancelar). Esperado: no se envía tráfico y el contador `%LocalAppData%\NPerf\mlab-diario.json` no cambia.
4. Repite y acepta. Anota el tiempo total (≤ 35 s), la descarga y la subida. Haz a la vez otra prueba web de velocidad y compara (mismo orden de magnitud, ±25 %, orientativo). Se muestra la atribución "Velocidad medida con NDT7 de M-Lab".
5. Repite una segunda prueba: el diálogo debe aparecer de nuevo (en cada prueba).
6. Bloquea el puerto 443 saliente y repite aceptando. Esperado: descarga/subida "no disponible" con motivo, sin colgarse.
7. CLI: `.\nperf.exe internet -speed` pide `¿Acepta y quiere hacer la prueba de velocidad? [s/N]:`; con N sale con `prueba de velocidad no aceptada. No se envió tráfico`. Sin terminal exige `-accept-mlab`.
8. Límite: con el contador en 40 (edita `mlab-diario.json` poniendo el contador de hoy en 40 con la app cerrada, o repite 40 veces), la siguiente prueba se rechaza con "límite diario de pruebas M-Lab alcanzado" sin tráfico. Restaura el archivo tras la prueba.

Esperado: aviso visible antes de pulsar; diálogo en cada prueba; resultado en ≤ 35 s coherente; 443 bloqueado da "no disponible" con motivo; el rechazo no gasta cuota; la prueba 41 se rechaza.
- [ ] OK  [ ] Fallo: ______

### 21. Borrar todo y historial desactivado
Objetivo: el borrado es real y desactivar impide escribir.
1. Con registros en la carpeta, usa "borrar todo" en el historial de la ventana y confirma.
2. Comprueba que la carpeta `%LocalAppData%\NPerf\historial` queda vacía y la lista sin registros.
3. En Ajustes desactiva el historial.
4. Lanza un TEST! y un ping; espera a que terminen.
5. Comprueba que no se creó ningún archivo en la carpeta.
6. CLI: `.\nperf.exe ping <host> -save` imprime `aviso: el historial está desactivado; no se guardó el resultado`. Sin `-save` no se guarda nada.
7. Reactiva el historial y comprueba que un TEST! vuelve a guardarse.

Esperado: tras borrar todo la carpeta está vacía; desactivado no se crean archivos; reactivado guarda.
- [ ] OK  [ ] Fallo: ______

### 22. Política de privacidad
Objetivo: la política está publicada, accesible y coincide con la realidad. Revisión del usuario.
1. En el diálogo de consentimiento de **Velocidad a internet** pulsa el enlace de la política de NPerf: debe abrir `https://github.com/scouter190/Nperf/blob/main/docs/privacidad.md` (ya subida a `main`).
2. Pulsa el de M-Lab: debe abrir `https://www.measurementlab.net/privacy/`.
3. Lee `docs/privacidad.md` y compara con lo que ves: rutas locales (`%LocalAppData%\NPerf` y la carpeta de configuración), qué se guarda en el historial, qué se envía a M-Lab (IP, fecha, metadatos), límite de 40 al día, que nada va a otros servicios.
4. Anota cualquier discrepancia.

Esperado: ambos enlaces abren la página correcta y el contenido coincide con el comportamiento de NPerf.
- [ ] OK  [ ] Fallo: ______

## 3. Resultado
Criterios OK: ___ / 7. Observaciones: ______

# Validación manual de equipos sin NPerf (criterios 11-16, spec 004)

Objetivo: comprobar a mano que `nperf ping|trace|mtu|ports` y los diagnósticos de la ventana coinciden con las herramientas de Windows y funcionan sin administrador ni cambios de firewall.

## 1. Requisitos
- Equipo **A** Windows 10/11 con `nperf.exe` y `nperf-gui.exe` (`.\scripts\build-windows.ps1` o artefacto del CI). Cuenta de usuario normal; no usar "Ejecutar como administrador"; firewall de Windows con su configuración por defecto (sin reglas añadidas para NPerf).
- Router con acceso a Internet (ideal MikroTik, para el puerto 8291) y su IP: R = ______
- Para el criterio 13: una VPN o conexión PPPoE/túnel que reduzca el MTU (opcional pero recomendable).
- Para el criterio 15: un portátil **W** por WiFi, que puede ser el propio A.
- Equipo A: ______  Windows: ______  Router: ______

## 2. Criterios

### 11. Ping coherente con `ping -n 10`
Objetivo: pérdida y RTT iguales a los de Windows.
1. En una consola normal ejecuta `ping -n 10 <R>` y anota pérdida y media.
2. Ejecuta `.\nperf.exe ping <R> -count 10`. Salida: líneas `Ping a ...`, `Enviados 10, recibidos N, pérdida X %`, `RTT mín/media/máx: ...` y `Jitter: ...`.
3. Repite ambos 2-3 veces y compara la media.
4. Ejecuta `.\nperf.exe ping <R> -json` y comprueba que stdout es un solo JSON con `format_version` y `command: "ping"`.
5. Ejecuta `.\nperf.exe ping 192.0.2.1 -count 3` (IP sin respuesta) y consulta `echo %ERRORLEVEL%`.

Esperado: pérdida igual y media a ±2 ms de `ping`; código 0 con respuestas; con 100 % de pérdida el código es 1.
- [ ] OK  [ ] Fallo: ______

### 12. Traceroute igual a `tracert -d`
Objetivo: mismos saltos que Windows.
1. Ejecuta `tracert -d 8.8.8.8` y anota la IP de cada salto.
2. Ejecuta `.\nperf.exe trace 8.8.8.8`. Salida: `Ruta a ...`, una línea por salto con `pérdida X %`, y `Destino alcanzado.` (o `Destino no alcanzado.`) más los hallazgos con `- `.
3. Compara salto a salto. Los saltos que `tracert` muestra con `*` pueden aparecer sin respuesta en NPerf; un salto que limita ICMP debe llevar el hallazgo "el equipo limita respuestas ICMP (no es pérdida real)".
4. Ejecuta `.\nperf.exe trace 8.8.8.8 -json` y comprueba `hops` y `findings`.

Esperado: la misma secuencia de IP intermedias que `tracert -d` (las variaciones entre ejecuciones también se ven en `tracert`); el primer salto es la puerta de enlace.
- [ ] OK  [ ] Fallo: ______

### 13. MTU coincide con `ping -f -l`
Objetivo: 1500 en Ethernet y valor reducido tras VPN/PPPoE.
1. Con cable/LAN normal ejecuta `.\nperf.exe mtu 8.8.8.8`. Esperado: `MTU: 1500 bytes (carga ICMP máxima 1472 bytes)`.
2. Verifica con Windows: carga = MTU - 28 (20 de cabecera IP + 8 de ICMP). `ping -f -l 1472 8.8.8.8` responde; `ping -f -l 1473 8.8.8.8` da "El paquete necesita ser fragmentado pero la bandera DF está establecida".
3. Conecta la VPN/PPPoE/túnel y repite `.\nperf.exe mtu <destino>`. Anota el MTU N.
4. Comprueba con `ping -f -l <N-28> <destino>` (responde) y `ping -f -l <N-27> <destino>` (fragmentación requerida).
5. Con la VPN activa, en la ventana lanza **MTU** hacia el mismo destino: debe aparecer la nota "MTU reducido en la ruta (N; VPN/PPPoE/túnel): posibles cortes con paquetes grandes" (solo la muestra la ventana; `nperf mtu` imprime únicamente el MTU y las sondas enviadas).

Esperado: el MTU de NPerf es el mayor tamaño que `ping -f -l` deja pasar más 28, en ambos casos. Si no hay respuesta ICMP en la ruta, sale `MTU: no disponible (<motivo>)`, no un número inventado.
- [ ] OK  [ ] Fallo: ______

### 14. Puertos coherentes con navegador / Winbox
Objetivo: puertos de administración abiertos y el resto cerrado o filtrado.
1. Ejecuta `.\nperf.exe ports <R>`. Salida: `Puertos TCP de ...` y una línea `  <n>/tcp  abierto|cerrado|filtrado`.
2. Abre `http://<R>` y `https://<R>` en el navegador; si es MikroTik, conecta con Winbox a `<R>:8291`.
3. Compara: los puertos que responden en el navegador/Winbox (80, 443, 8291) deben salir `abierto`; los que no se usan, `cerrado` o `filtrado`.
4. Ejecuta `.\nperf.exe ports <R> -p 22,80,8000-8010` y comprueba que respeta la lista.
5. Ejecuta `.\nperf.exe ports <R> -p 1-100` y comprueba el rechazo (máx. 32 puertos), código 2.

Esperado: abiertos = los que atiende el router; ninguno abierto que el navegador/Winbox no confirme.
- [ ] OK  [ ] Fallo: ______

### 15. En la ventana
Objetivo: destinos a mano, grupo "Sin NPerf", los cuatro diagnósticos, cancelar y diagnóstico combinado. (Los botones se describen por nombre; su aspecto lo cubre la spec de interfaz.)
1. Abre `nperf-gui.exe`. En el grupo **"Sin NPerf"** debe aparecer la **puerta de enlace** automáticamente, sin haberla añadido.
2. Añade un destino por IP (la de otro equipo o `8.8.8.8`) y otro por nombre (p. ej. `dns.google`), con y sin alias. Ambos quedan en "Sin NPerf" y sobreviven al reiniciar la app.
3. Pruebas de error al añadir: la IP de la puerta de enlace ("esa dirección ya está en la lista como puerta de enlace"), un destino repetido ("<destino> ya está en la lista"), `http://x` ("el destino no debe llevar esquema (use solo IP o nombre)"), `10.0.0.0/24` ("no se admiten CIDR, rutas ni URL"), `0.0.0.0` ("0.0.0.0 no es un destino válido").
4. Selecciona un destino sin NPerf y lanza **Ping**, **Traceroute**, **MTU** y **Puertos**; cada uno muestra resultado equivalente al de la CLI. Comprueba también que los botones se ofrecen para un equipo con NPerf.
5. Lanza **Traceroute** a `8.8.8.8` y pulsa **Cancelar** a mitad: la prueba termina en ~1 s y pasa a cancelada, sin dejar la ventana bloqueada.
6. Bloqueo: inicia un **TEST!** hacia un equipo con NPerf y, mientras corre, intenta un diagnóstico: debe rechazarse con "hay un TEST! en curso: espere a que termine para lanzar un diagnóstico" (botones desactivados). Con un diagnóstico en curso, intenta TEST!: "hay un diagnóstico en curso: espere a que termine para iniciar un TEST!". Un segundo diagnóstico simultáneo: "ya hay un diagnóstico en curso".
7. Quita un destino manual (permitido); la puerta de enlace no se puede quitar ("la puerta de enlace no se puede quitar de la lista").
8. Portátil **W** por WiFi, lejos del AP (señal baja): lanza el diagnóstico combinado hacia la puerta de enlace y hacia Internet. Debe señalar el tramo WiFi: "pérdida/latencia en el tramo WiFi de este equipo (señal N %)". Repite cerca del AP y por cable: con cable, "problema en el primer tramo (cable, switch o gateway)" solo si hay pérdida real; si todo va bien, "enlace sano hasta <destino>".

Esperado: todo lo anterior; ningún diagnóstico envía tráfico al añadir el destino; los resultados coinciden con los de los criterios 11-14.
- [ ] OK  [ ] Fallo: ______

### 16. Sin administrador y con firewall por defecto
Objetivo: nada requiere elevación ni reglas de firewall.
1. En el Administrador de tareas (columna Elevado) confirma que `nperf.exe` y `nperf-gui.exe` no están elevados.
2. Con el firewall de Windows en su configuración por defecto (sin reglas de NPerf y sin aceptar avisos de "permitir acceso"), repite `ping`, `trace`, `mtu` y `ports` (CLI) y los cuatro diagnósticos en la ventana.
3. Comprueba que no aparece ningún aviso UAC ni diálogo de firewall por los diagnósticos.

Esperado: los cuatro diagnósticos funcionan igual que en los criterios 11-15 sin elevación ni reglas añadidas.
- [ ] OK  [ ] Fallo: ______

## 3. Resultado
Fecha: ______  Versión/commit: ______  Observaciones: ______

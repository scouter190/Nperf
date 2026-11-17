# Validación manual de gráficas en vivo y monitor (criterios 11-17, spec 005)

Objetivo: comprobar a mano que las gráficas en vivo, el RTT bajo carga y el monitor de tarjetas funcionan en Windows sin administrador y sin alterar la medición.

## 1. Requisitos
- Dos equipos Windows 10/11 con NPerf, **A** (cliente) y **B** (servidor, `nperf.exe server` o `nperf-gui.exe`), con `nperf.exe` y `nperf-gui.exe`. Cuenta normal, sin "Ejecutar como administrador", firewall por defecto.
- Cable Gigabit entre A y B (red C) y, para el criterio 12, un enlace WiFi entre ellos (red W).
- Para el criterio 13: una subida grande a Internet (o WiFi saturado con otro equipo transfiriendo).
- Para el criterio 15: un equipo **V** con la versión de NPerf de la spec 004 (sin muestras en vivo).
- Para el criterio 16: Administrador de tareas (pestaña Rendimiento) y un archivo grande para descargar.
- Equipo A: ______  Equipo B: ______  Equipo V: ______  Windows: ______

## 2. Criterios

### 11. TEST! por cable de 30 s en "Ambos"
Objetivo: gráfica en vivo coherente con el resultado final.
1. En A abre `nperf-gui.exe`, elige B (por cable), duración 30 s, protocolo "Ambos" y pulsa **TEST!**.
2. Durante la prueba observa la gráfica de cada fase (TCP ida/vuelta y UDP ida/vuelta).
3. Al terminar compara la media visual de cada fase con la velocidad del resultado final.
4. Comprueba que la línea nominal (p. ej. 1000 Mbps) está a la altura de ese valor en el eje.
5. En consola ejecuta `.\nperf.exe client <B> -duration 30` y cuenta las líneas de intervalo (`  tcp ida 0-1 s: N Mbps`, y con UDP `, pérdida X %, jitter Y ms`).

Esperado: la gráfica se mueve en vivo en cada fase; la media visual coincide con el resultado final; la línea nominal está en su sitio; la consola muestra una línea por segundo.
- [ ] OK  [ ] Fallo: ______

### 12. TEST! por WiFi
Objetivo: variación por intervalo y referencia PHY visibles.
1. Conecta A a B por WiFi y lanza **TEST!** de 30 s en "Ambos".
2. En TCP comprueba que se ve la variación por intervalo y la referencia PHY (velocidad de enlace WiFi).
3. En las fases UDP comprueba que se ven la pérdida y el jitter por intervalo (también en consola: `pérdida X %, jitter Y ms`).

Esperado: variación por intervalo y referencia PHY en TCP; pérdida y jitter por intervalo en UDP.
- [ ] OK  [ ] Fallo: ______

### 13. RTT bajo carga y bufferbloat
Objetivo: la nota aparece solo cuando hay colas llenas.
1. Con cable LAN sin congestión lanza **TEST!** de 30 s A -> B. La gráfica de latencia debe quedar plana junto al RTT inicial y **no** debe aparecer la nota.
2. Provoca bufferbloat: sube un archivo grande a Internet desde la red, o satura el WiFi con otro equipo. Hazlo contra un destino cuyo camino pase por ese enlace y repite el TEST!.
3. Comprueba que la gráfica de latencia sube frente al RTT inicial y aparece la nota `la latencia sube N ms con carga en <sentido>: colas llenas en el enlace (bufferbloat)` (en la ventana y en el resultado de consola).
4. Compara el throughput con un TEST! hecho contra el equipo V de la spec 004 en las mismas condiciones.

Esperado: sin congestión no hay nota; con colas grandes la latencia sube (>= 100 ms sobre el RTT medio inicial) y aparece la nota; el throughput coincide con el del cliente de la spec 004 (±10 %).
- [ ] OK  [ ] Fallo: ______

### 14. Las sondas no alteran la velocidad
Objetivo: diferencia < 1 % con y sin sondas de RTT (solo TCP: el UDP de `-proto both` depende del TCP de la misma ejecución y no es comparable entre ejecuciones).
1. Por cable y sin otro tráfico en la red, define A = `.\nperf.exe client <B> -duration 30 -proto tcp -direction fwd` y B = `.\nperf.exe client <B> -duration 30 -proto tcp -direction fwd -no-load-rtt`.
2. Ejecuta alternando A, B, A, B (al menos 2 ejecuciones de cada una) y anota la velocidad TCP de cada una.
3. Calcula la media de cada grupo y la diferencia relativa entre medias.

Esperado: diferencia entre medias < 1 %; con `-no-load-rtt` no hay medición de RTT bajo carga.
- [ ] OK  [ ] Fallo: ______

### 15. Equipo con la versión de la spec 004
Objetivo: degradación elegante ante un servidor sin muestras en vivo.
1. En A lanza **TEST!** contra V (versión de la spec 004).
2. Espera a que acabe.

Esperado: el resultado final es completo; la ventana muestra el mensaje "gráfica no disponible: el otro equipo no envía muestras" y no hay errores.
- [ ] OK  [ ] Fallo: ______

### 16. Monitor de tarjetas
Objetivo: contadores coherentes y bajo consumo en reposo.
1. En A abre el monitor de tarjetas de `nperf-gui.exe`, con el Administrador de tareas (Rendimiento) abierto en la misma tarjeta.
2. Inicia una descarga grande y compara la velocidad de entrada de la tarjeta en NPerf con la del Administrador de tareas durante la descarga.
3. Anota el uso de CPU de `nperf-gui.exe` en reposo con el panel visible (sin descarga).
4. Oculta el panel del monitor y, aparte, cierra la ventana para que quede en la bandeja. Espera 30 s y anota la CPU en cada caso.

Esperado: la entrada sube de forma coherente con el Administrador de tareas (±10 %); con el panel oculto o en la bandeja la CPU vuelve al valor de reposo.
- [ ] OK  [ ] Fallo: ______

### 17. Temas
Objetivo: legibilidad en ambos temas.
1. Con el tema claro, repite una vista con gráficas (criterio 11) y revisa gráficas, ejes, línea nominal y valores.
2. Cambia el tema del sistema/aplicación a oscuro y repite.
3. Comprueba que los valores numéricos se leen sin depender del color (p. ej. mirando la pantalla en escala de grises o ignorando el color de las series).

Esperado: ambos temas son legibles y todos los valores numéricos son visibles sin depender del color.
- [ ] OK  [ ] Fallo: ______

## 3. Resultado
Criterios OK: ___ / 7. Observaciones: ______

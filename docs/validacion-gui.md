# Validación manual de la interfaz (criterios 14-19, spec 002)

Objetivo: comprobar a mano lo que el CI no puede (ventana, bandeja, tema, aviso sin WebView2, firewall).

## 1. Requisitos
- Dos PC Windows en la misma LAN: **A** y **B** (opcional **C** para la cola del criterio 16).
- `nperf-gui.exe` y `nperf.exe` en cada uno: `.\scripts\build-windows.ps1` o el artefacto del CI.
- Misma clave en Ajustes de cada equipo (se guarda y reinicia el servicio interno).
- Firewall: permitir `nperf-gui.exe` solo en red privada (5300 TCP/UDP y 5301 UDP).
- Cuenta de usuario normal; no usar "Ejecutar como administrador".
- Equipo A: ______  B: ______  C: ______  Windows: ______

## 2. Criterios

### 14. Arranque, idioma y tema
Objetivo: la ventana abre rápido, sin consola, en español y sigue el tema de Windows.
1. Doble clic en `nperf-gui.exe` en A (cronometra hasta ver la ventana).
2. Windows > Configuración > Personalización > Colores: pon el modo en Claro; luego en Oscuro.

Esperado: ventana en < 2 s, sin ventana de consola, textos en español; la ventana adopta el tema (si no cambia sola, anótalo y cierra/abre la app).
- [ ] OK  [ ] Fallo: ______

### 15. Descubrimiento
Objetivo: dos PC se ven en <= 10 s y el que se cierra pasa a gris en <= 15 s.
1. Abre `nperf-gui.exe` en A y en B (misma clave).
2. Mide el tiempo hasta que B aparece en la lista de A (y A en la de B). "Este equipo" aparece aparte.
3. En B, abre la bandeja y elige **Salir** (o cierra el proceso). Mide el tiempo hasta que A lo marca "inalcanzable" (gris).

Esperado: aparición <= 10 s; inalcanzable <= 15 s.
- [ ] OK  [ ] Fallo: ______

### 16. TEST! y cola
Objetivo: la medición muestra RTT, TCP y UDP por sentido, y la cola funciona.
1. En A, selecciona B y pulsa **TEST!** (sentido y duración por defecto, o 10 s).
2. Revisa el panel de detalle: RTT, TCP y UDP en ambos sentidos.
3. Con C (o una consola `nperf.exe client <IP-B> -key CLAVE`) mide a B a la vez que A: el equipo que mide en cola ve en su ventana "En cola, posición 1" (la consola muestra su propio aviso de cola); en la lista de un tercer equipo, B aparece con la etiqueta "en cola 1".
4. Pulsa **Cancelar** en el equipo en cola.

Esperado: resultados completos; el equipo en cola sale de la cola y vuelve a poder medir (B queda disponible tras la medición en curso).
- [ ] OK  [ ] Fallo: ______

### 17. Bandeja e instancia única (T16)
Objetivo: cerrar oculta, el equipo sigue visible y medible, Salir lo retira; no hay segunda instancia.
1. En B, cierra la ventana con la X. Esperado: el icono sigue en la bandeja (en "iconos ocultos" si hace falta) y no hay ventana.
2. Desde A, comprueba que B sigue en la lista y haz TEST! hacia B: se mide con normalidad.
3. En B, doble clic en el icono: la ventana se abre. Ciérrala otra vez y usa clic derecho > **Abrir**: se abre.
4. Con B abierto (o en bandeja), ejecuta `nperf-gui.exe` de nuevo. Esperado: MessageBox con el texto "NPerf ya está abierto" (título "NPerf"); el proceso nuevo sale y no se crea otro proceso (Administrador de tareas: un solo `nperf-gui.exe`).
5. Reinicia el Explorador (Administrador de tareas > Explorador de Windows > Reiniciar). Esperado: el icono reaparece en la bandeja.
6. Clic derecho > **Salir**. Esperado: el icono y el proceso desaparecen; en A, B pasa a "inalcanzable" en <= 15 s (objetivo ~10 s) y TEST! hacia B no está disponible.

- [ ] OK  [ ] Fallo: ______

### 18. Sin WebView2 (T17)
Objetivo: aviso nativo con el enlace del runtime; la CLI sigue funcionando.
Preparación (sin desinstalar nada en tu PC): usa una VM o Windows Sandbox donde no esté instalado Microsoft Edge WebView2 Runtime (Windows 11 lo incluye en general; comprueba que no figure en Aplicaciones instaladas). Alternativa: política que lo bloquee en un equipo de prueba. Copia ahí `nperf-gui.exe` y `nperf.exe`.
1. Ejecuta `nperf-gui.exe`.
2. Ejecuta `.\nperf.exe status <IP>` o `.\nperf.exe server` (la CLI no usa WebView2).
3. En un equipo con WebView2 (A), comprueba que **no** aparece ese aviso al abrir la app.

Esperado: cuadro de mensaje en español que dice que falta Microsoft Edge WebView2 Runtime (o está bloqueado por política) e incluye https://go.microsoft.com/fwlink/p/?LinkId=2124703; `nperf.exe` funciona; con runtime presente no hay aviso.
- [ ] OK  [ ] Fallo: ______

### 19. Sin administrador y firewall
Objetivo: funciona sin elevación y avisa si el firewall bloquea el descubrimiento.
1. Confirma en el Administrador de tareas (columna Elevado) que `nperf-gui.exe` no está elevado.
2. En A, crea una regla de entrada que bloquee UDP 5301 (o desactiva la excepción de `nperf-gui.exe`), con B en marcha, y reinicia la app en A.
3. Espera 15 s con la lista vacía.

Esperado: aparece la sugerencia "No se encontró ningún equipo. Revise el firewall de Windows: debe permitir 5300 TCP/UDP y 5301 UDP." Al restaurar la regla y reiniciar, B aparece. Elimina la regla de prueba al terminar.
- [ ] OK  [ ] Fallo: ______

## 3. Defender / SmartScreen
Objetivo: el binario no se bloquea ni se marca como amenaza.
1. Descarga `nperf-gui.exe` (artefacto del CI) en un equipo limpio y ábrelo. SmartScreen puede avisar por binario sin firmar ("Más información" > "Ejecutar de todas formas"); anótalo, no es un fallo.
2. Revisa Seguridad de Windows > Protección contra virus > Historial de protección.

Esperado: ninguna detección ni cuarentena de `nperf-gui.exe` ni `nperf.exe`.
- [ ] OK  [ ] Fallo: ______

## 4. Resultado
Fecha: ______  Versión/commit: ______  Observaciones: ______

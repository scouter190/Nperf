# Validación manual de la detección de red (criterios 12-16, spec 003)

Objetivo: comprobar a mano lo que el CI no puede (WiFi real, permiso de ubicación, semáforo entre equipos reales, servicio WLAN).

## 1. Requisitos
- Equipos Windows 10/11 en la misma LAN: **A** (PC por cable, ideal 1 Gbps), **B** (portátil por WiFi), opcional **C** (segundo PC por cable a 1 Gbps).
- `nperf-gui.exe` y `nperf.exe` en cada uno (`.\scripts\build-windows.ps1` o artefacto del CI), misma clave en Ajustes, firewall como en docs/validacion-gui.md.
- Cuenta de usuario normal; no usar "Ejecutar como administrador".
- Equipo A: ______  B: ______  C: ______  Windows (B): ______

## 2. Criterios

### 12. Datos iguales a Windows
Objetivo: SSID, banda, canal, señal y velocidad coinciden con las herramientas de Windows.
1. En B (WiFi), ejecuta `netsh wlan show interfaces` y anota SSID, Banda, Canal, Señal y Velocidad de recepción/transmisión.
2. Abre `nperf-gui.exe` en B, despliega **Tarjetas de red** y marca **Modo detallado**. Compara SSID, Banda, Canal, Señal, Tasa ↑ y Tasa ↓.
3. En B ejecuta `.\nperf.exe adapters` y compara los mismos datos (línea `WiFi: <ssid>, <banda>, canal <n>, señal <%>`).
4. Ejecuta `.\nperf.exe adapters -json` y comprueba que es JSON válido con `format_version`.
5. En A (cable), abre Configuración > Red e Internet > Ethernet (o "Estado de Ethernet" en el Panel de control) o ejecuta en PowerShell `Get-NetAdapter | Format-Table Name, Status, LinkSpeed`. Compara la velocidad con la tarjeta de A en el panel y en `nperf adapters`.

Esperado: SSID, banda, canal y velocidad iguales; señal % dentro de ±5 puntos (varía segundo a segundo, repite la lectura); en cable la velocidad es la de Windows.
- [ ] OK  [ ] Fallo: ______

### 13. Ubicación desactivada (Windows 11 24H2+)
Objetivo: aviso con botón y el resto de datos visibles.
1. En B: Configuración > Privacidad y seguridad > Ubicación. Desactiva **Permitir que las aplicaciones de escritorio accedan a su ubicación** (o el interruptor general de Ubicación).
2. Reinicia `nperf-gui.exe` y abre **Tarjetas de red**.
3. Comprueba el aviso: "Windows no permite leer los datos WiFi: activa el permiso de ubicación para las aplicaciones en Configuración › Privacidad › Ubicación."
4. Pulsa **Abrir Configuración de ubicación**.
5. Ejecuta `.\nperf.exe adapters` y observa la línea WiFi.
6. Reactiva el permiso al terminar y comprueba que el aviso desaparece (≤ 5 s con el panel abierto).

Esperado: aviso visible; el botón abre la página de Ubicación de Configuración; tipo y velocidad de la tarjeta siguen visibles; los datos WiFi aparecen como no disponibles con el motivo (p. ej. "permiso de ubicación denegado"); no hay error fatal.
- [ ] OK  [ ] Fallo: ______

### 14. Lista desde otro PC
Objetivo: icono, velocidad y SSID correctos en la lista de otro equipo en ≤ 10 s.
1. Abre `nperf-gui.exe` en A y en B (misma clave; permiso de ubicación activado en B).
2. En A, mide el tiempo hasta que B aparece en la lista, y revisa su icono (WiFi), su velocidad y su SSID.
3. En B, comprueba el mismo dato de A (icono de cable y velocidad, sin SSID).
4. Compara con lo anotado en el criterio 12.

Esperado: en ≤ 10 s cada equipo muestra el icono (cable / WiFi), la velocidad de enlace y, en WiFi, el SSID correctos. "Este equipo" aparece aparte con sus propios datos.
- [ ] OK  [ ] Fallo: ______

### 15. Semáforo en TEST!
Objetivo: diagnóstico por sentido con el extremo limitante.
1. En A, selecciona B y pulsa **TEST!** (sentido ambos, duración por defecto o 10 s).
2. En el panel de detalle, revisa **Diagnóstico**: una fila por sentido con punto de color y %.
3. Lee el texto: debe decir "Limitado por el WiFi de <alias de B> (enlace <velocidad>…)." con la velocidad de enlace de B.
4. Repite entre A y C (ambos cable a 1 Gbps).

Esperado: A↔B muestra semáforo por sentido y "Limitado por el WiFi de <alias>"; A↔C sale en verde ("Sin limitación detectada" al pasar el ratón por el punto) con el texto "Limitado por ambos extremos (enlace 1 Gbps)." Si un extremo no informa su enlace, el texto añade "Diagnóstico parcial: faltan datos del otro extremo."
- [ ] OK  [ ] Fallo: ______

### 16. Sin administrador y con el servicio WLAN detenido
Objetivo: todo funciona sin elevación; sin WlanSvc la app sigue arrancando y midiendo.
1. Confirma en el Administrador de tareas (columna Elevado) que `nperf-gui.exe` y `nperf.exe` no están elevados.
2. Preparación por un administrador (omitir si no hay quien pueda): en B, `services.msc` > **Configuración automática de WLAN** (WlanSvc) > Detener; o en PowerShell elevado `Stop-Service WlanSvc`. Al terminar, volver a iniciarlo (`Start-Service WlanSvc`).
3. Con el servicio detenido, abre `nperf-gui.exe` en B y despliega **Tarjetas de red**; ejecuta `.\nperf.exe adapters`.
4. Desde A, haz TEST! hacia B.

Esperado: la app arranca sin elevación; la tarjeta WiFi conserva tipo y velocidad y muestra el WiFi como no disponible con el motivo "servicio WLAN no disponible"; `nperf adapters` termina con código 0; la prueba mide con normalidad (el diagnóstico puede salir parcial).
- [ ] OK  [ ] Fallo: ______

## 3. Resultado
Fecha: ______  Versión/commit: ______  Observaciones: ______

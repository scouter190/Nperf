# memory.md - NPerf

## 1. Concepto
- **NPerf**: utilidad ligera de diagnóstico de red para Windows, uso similar a iperf.
- Mide enlaces punto a punto: equipo-equipo, equipo-router, equipo-AP.
- Detecta tarjetas de red, tipo de conexión y WiFi; señala cuellos de botella.

## 2. Plataforma
- **Windows** únicamente. Lenguaje **Go**. **.exe portable** (sin instalador en v1).
- Interfaz: **Walk (Win32 nativo)**, consumo mínimo.
- **Sin administrador**. Diagnósticos que lo requieran se omiten con aviso.
- Firewall: la app detecta si está bloqueada y muestra un **aviso guiado** para permitirla.
- Al cerrar la ventana **se minimiza a la bandeja** y sigue respondiendo y midiendo.
- Arranque con Windows: **configurable**.

## 3. Red y descubrimiento
- **Punto a punto total**: cada equipo mide contra cualquier otro.
- Descubrimiento automático en la **subred local**; subredes extra **a mano**.
- **Puerto fijo** por defecto, configurable.
- **Clave compartida**, guardada en la configuración local.
- El mapa lista **todos los equipos con NPerf abierto**. Inalcanzables **en gris**.
- Versiones: compatibles si coincide la **versión mayor** (1.x con 1.x); si no, aviso.

## 4. Protocolo de medición
- Protocolo **propio** (opción A). Compatibilidad con iperf3 fuera de v1.
- Validar resultados contra iperf3 en la misma red.
- Control TCP autenticado. Negociación: `sentido`, `duracion`, `flujos`, `tasa_udp`.
- Por defecto TEST! corre **TCP y luego UDP**.
- TCP: **4 flujos paralelos** por defecto, configurable.
- UDP: tasa = **90 % del throughput TCP** medido.
- Métricas: throughput, jitter, pérdida, retransmisiones, RTT (ping antes de la carga).
- Concurrencia: si un equipo ya está en prueba, la nueva va a **cola con aviso**.

## 5. Sentido
- El usuario elige: **Origen → Destino**, **Destino → Origen** o **Ambos** (default).
- Etiquetas con nombres de equipos. Sentido no medido = **"no medido"**.

## 6. Duración
- **Combo box** editable: 5, 10 (default), 30, 60, 120, 300 s. Rango **3–300 s**.
- En "Ambos" aplica **por sentido**; se muestra el total.
- **Recuerda** la última elección. Aplica a pruebas **manuales y programadas**.

## 7. Interfaz
- Tema tipo **MikroTik / WinBox**: grises con acento azul, colores suaves.
- Claro/oscuro **automático** según el sistema. Idioma **español**.
- **Compacta** por defecto, con modo **detallado**.
- Barra superior fija: **[Sentido ▾] [Duración ▾] [TEST!]**.
- Selección: **clic en dos nodos** del mapa, luego TEST!.
- Resultado: en el **panel de detalle** y el enlace cambia de color en el mapa.

## 8. Mapa y visualización
- Nodos: equipos, router, AP, gateway; enlaces coloreados.
- Cada nodo: **nombre, IP, tipo de conexión, estado, versión de NPerf**.
- Velocidad de enlace: **1000/1000, 100/100, 10/10** Mbps, cable y WiFi.
- Gráficas en vivo de los **últimos 5 min**. Tablas con semáforo. Panel de detalle a la derecha.

## 9. Semáforo y diagnóstico
- % del enlace nominal: verde > 80 %, amarillo 50–80 %, rojo < 50 %.
- Cuello: cada enlace contra el **más lento de su tramo**.
- Subida y bajada se comparan y reportan por separado.

## 10. Tarjetas de red
- **Identidad:** nombre, fabricante, MAC, IP, driver.
- **Enlace:** Ethernet/WiFi/USB/celular, velocidad, duplex.
- **Estado:** conectada, errores, descartes, uso.
- **Capacidades:** estándar WiFi, bandas, Wake-on-LAN, ahorro de energía.
- **WiFi:** RSSI, banda, canal, ancho de canal, 802.11, BSSID, ruido.

## 11. Sin agente y datos técnicos
- Routers y APs: **ping y traceroute**. Sin SNMP ni agente en v1.
- Topología: **tabla ARP y gateway**. Sin LLDP ni dibujo manual.
- Técnicos: traceroute por salto, MTU, puertos y conexiones TCP/UDP.

## 12. Ejecución
- Manual (TEST!) y programada con **intervalo fijo**, mayor que la duración total.
- Prueba hacia internet: **opcional**.

## 13. Histórico y alertas
- **Solo en vivo**; botón para **exportar la sesión** en CSV o JSON.
- Alertas: color en pantalla, sonido opcional.
- Notificación de Windows **solo si hay cuello** en prueba programada.
- Registro de eventos en **archivo de texto** con fecha y hora.

## 14. Proyecto
- Nombre **NPerf**. Repo nuevo en GitHub, propuesta `scouter190/nperf` (pendiente).
- Si se acaba el crédito en la nube: clonar el repo y seguir con Claude Code local.

## 15. Pendientes
- [ ] Confirmar dueño y nombre del repositorio.
- [ ] Confirmar Walk como librería de interfaz.
- [ ] Escribir la especificación en el repositorio.
- [ ] Validar resultados contra iperf3.

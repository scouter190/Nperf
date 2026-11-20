# Cambios de NPerf

Todos los cambios relevantes se anotan aquí. El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/).

## [Sin publicar]

## [1.0.0] - pendiente

Primera versión publicada de NPerf, medidor de red para Windows que funciona sin privilegios de administrador.

### Aviso de compatibilidad

La versión 1.0.0 usa la versión de protocolo 1.0.0. Las versiones 0.x de prueba no son compatibles: al conectar con ellas la prueba se rechaza con un mensaje de versión incompatible. Actualiza todos los equipos a la misma versión mayor.

### Medidor entre dos equipos

- `nperf server` y `nperf client <host>`: mide el enlace entre dos equipos con NPerf, con clave compartida que nunca viaja por la red.
- Mide RTT, throughput TCP (varios flujos en paralelo), UDP con jitter y pérdida, y retransmisiones TCP (exactas o estimadas según el sistema).
- Elige sentido (ida, vuelta o ambos), duración (3 a 300 s), protocolo y tasa UDP. Por defecto el UDP usa el 90 % de lo medido por TCP.
- Si el equipo remoto está ocupado, la prueba espera en cola (hasta 5 en espera) y se avisa de la posición.
- `nperf status <host>` indica, sin clave, si el equipo está disponible u ocupado.
- Salida legible o `-json` para automatizar.

### Ventana, descubrimiento y bandeja

- `nperf-gui.exe`: ventana de escritorio con botón TEST! para medir sin usar la consola.
- Descubre automáticamente los equipos con NPerf de la red local, con su estado, y permite añadir otras subredes.
- Se mantiene en la bandeja del sistema: cerrar la ventana la oculta y el equipo sigue disponible como destino de pruebas.
- Ajustes de alias, duración y puerto de descubrimiento.
- Necesita Microsoft Edge WebView2; si falta, la aplicación lo indica y ofrece la descarga.

### Detección de la red

- Lista los adaptadores del equipo, con tipo (cable, WiFi u otro), velocidad nominal y, en WiFi, SSID, banda y canal. Los adaptadores virtuales se muestran aparte.
- `nperf adapters` ofrece lo mismo desde la consola.
- Tras cada prueba, un semáforo y un diagnóstico por sentido comparan lo medido con la velocidad nominal del enlace para señalar si el límite está en el cable, en el WiFi o en otro punto.
- El SSID y el canal WiFi pueden requerir activar los servicios de ubicación de Windows; la aplicación ofrece abrir el ajuste.

### Equipos sin agente

- Diagnóstico de routers, APs, impresoras y otros equipos que no tienen NPerf: `nperf ping`, `trace`, `mtu` y `ports`, también desde la ventana.
- Puedes añadir equipos manualmente y la puerta de enlace aparece como destino.
- Ping con pérdida, RTT y jitter; traceroute con análisis; descubrimiento de MTU con aviso si es menor de 1500; comprobación de puertos TCP.
- Diagnóstico combinado con veredicto sencillo.
- No se ejecutan diagnósticos durante un TEST! ni al revés.

### Gráficas en vivo

- Durante el TEST!, gráficas de velocidad por segundo de cada fase frente a la velocidad nominal del enlace; en UDP, pérdida y jitter por intervalo.
- Latencia bajo carga frente al RTT inicial, con aviso de posible bufferbloat si la latencia sube más de 100 ms.
- Con un equipo remoto antiguo que no envía muestras, el resultado es el mismo y se indica "gráfica no disponible".
- Monitor de tráfico en vivo de las tarjetas de red (últimos 5 minutos), solo mientras el panel está visible.
- `-json` añade los intervalos y la latencia bajo carga; la salida legible imprime una línea por intervalo.

### Historial, exportación y prueba hacia internet

- Historial local automático (activable o desactivable en Ajustes) de cada TEST!, diagnóstico y prueba hacia internet, con lista, filtros, revisión de resultados, borrado y retención de 90 días o 50 MB. Nunca guarda la clave, la MAC ni el BSSID.
- Exportación a CSV (formato estándar o Excel en español), JSON e informe HTML para entregar al cliente, con campos opcionales de cliente, técnico y notas, listo para imprimir a PDF. NPerf no envía el historial a ningún sitio.
- Prueba de latencia hacia internet (botón Internet y `nperf internet`): compara la puerta de enlace con destinos públicos (por defecto 1.1.1.1 y 8.8.8.8, editables) y da un veredicto entre red local y proveedor.
- Prueba de velocidad de descarga y subida con M-Lab NDT7, solo a petición y con consentimiento explícito en cada prueba. Envía tu IP pública a M-Lab, consume datos y tiene un límite de 40 pruebas por día. Ver `privacidad.md`.
- Opción `-save` en la CLI para guardar el resultado en el historial.

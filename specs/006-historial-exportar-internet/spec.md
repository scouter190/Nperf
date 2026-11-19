# 006 - Historial, exportar y prueba hacia internet

> Aprobada por el usuario (con las condiciones de M-Lab verificadas en sus páginas oficiales). Cambia §13 de `docs/decisiones.md` ("solo en vivo"); decisiones en §21.

## Objetivo
Que el técnico conserve y entregue lo que mide, y que pueda separar un problema de la LAN de uno del proveedor.
- **Historial local**: cada TEST!, diagnóstico y prueba hacia internet queda guardado en el equipo y se puede revisar después tal como se vio al terminar.
- **Exportar**: CSV para Excel y un informe HTML para entregar al cliente.
- **Prueba hacia internet**: latencia, pérdida y jitter frente a la puerta de enlace con veredicto "LAN vs internet", y, a petición, velocidad de descarga/subida con M-Lab NDT7.

Desarrolla §12 ("prueba hacia internet: opcional") y §13 (exportar).

## Alcance

### Incluye
- **Historial automático**, activado por defecto y desactivable en Ajustes:
  - Guarda cada TEST! (también cancelados o con error, marcados así), cada diagnóstico de la spec 004 (ping, trace, MTU, puertos, combinado) y cada prueba hacia internet (latencia y velocidad).
  - Ubicación: `%LocalAppData%\NPerf\historial` (`os.UserCacheDir()`), un archivo JSON por resultado, escritura atómica, sin administrador.
  - Contenido: id, fecha/hora local con zona, tipo, estado (completado/cancelado/error), versión de NPerf, origen y destino (alias, IP, SSID, tipo de enlace y velocidad nominal, spec 003), parámetros y el resultado completo en JSON v1. En el TEST! incluye `intervals` y `loaded_rtt` (spec 005), semáforo, diagnóstico y notas.
  - Nunca se guardan la clave, la MAC ni el BSSID.
  - Retención: **90 días o 50 MB** en total; se borra lo más antiguo primero. Se revisa al arrancar y tras cada guardado.
  - Al leer, los archivos son no confiables: se validan, tienen tope de tamaño por archivo y los corruptos se omiten con aviso, sin abortar la lista.
  - GUI y CLI pueden escribir a la vez sin pisarse (nombres únicos y renombrado atómico).
- **Historial en la ventana**:
  - Lista por fecha con filtros por equipo, tipo y rango de fechas.
  - Abrir un resultado lo muestra igual que al terminar (gráficas, semáforo y notas), solo lectura.
  - Borrar uno, los seleccionados o todo (con confirmación). Exportar los seleccionados.
  - Todo dato leído del disco se muestra como texto (sigue el test que prohíbe innerHTML).
- **Exportar** uno o varios resultados, con el diálogo estándar de Windows para elegir dónde. NPerf no envía el historial a ningún sitio.
  - **CSV** UTF-8 con BOM: un archivo de resumen (una fila por fase y sentido del TEST!, una por diagnóstico o prueba hacia internet) y, opcional, otro con los intervalos. En la ventana, formato Excel en español (`;` y coma decimal); en la CLI, estándar (`,` y punto) por defecto, con `-csv excel-es` para el otro.
  - **JSON**: los registros tal como se guardan (§13).
  - **Informe para el cliente**: HTML autocontenido (CSS en línea, gráficas SVG embebidas, sin scripts ni recursos externos), pensado para imprimir a PDF en A4 desde el navegador. Lleva fecha, equipos con alias, IPs y SSID, tipo de enlace y velocidades, resultados por sentido, semáforo y diagnóstico en lenguaje claro, gráficas y los campos opcionales cliente/técnico/notas que se rellenan al exportar. Nunca clave, MAC ni BSSID. Si incluye una prueba de velocidad M-Lab, lleva la atribución a M-Lab y el aviso de la IP pública.
- **Prueba de latencia hacia internet** (botón "Internet" y `nperf internet`):
  - Ping en paralelo a la puerta de enlace y a los destinos públicos (por defecto **1.1.1.1 y 8.8.8.8**, editables en Ajustes, máx. 4, IPv4), con los valores de la spec 004. Mide pérdida, RTT mín/media/máx y jitter.
  - Sin respuesta ICMP de internet: intenta TCP al 443 de los mismos destinos para distinguir "sin salida" de "ICMP filtrado".
  - Traceroute a un destino público solo si el veredicto es "proveedor" (análisis de la spec 004).
  - **Veredicto**:
    - Gateway sano (pérdida 0 y RTT ≤ 20 ms, §19) e internet sano → "sin problemas detectados".
    - Gateway sano e internet degradado → "el problema está después del router (proveedor)".
    - Gateway degradado → "el problema está en la red local (cable/WiFi/router)".
    - Sin respuesta ICMP ni TCP 443 → "sin salida a internet". Solo TCP 443 → "ICMP filtrado" (sin veredicto de latencia).
    - Internet degradado (**supuesto**, se revisa tras la validación): pérdida ≥ 2 %, RTT medio > 100 ms o jitter > 30 ms.
- **Velocidad a internet con M-Lab NDT7** (botón "Velocidad a internet" y `nperf internet -speed`), solo a petición explícita, nunca automática ni programada:
  - **Consentimiento informado antes de cada prueba** ([develop](https://www.measurementlab.net/develop/): "prior to installation of the client or initiation of the test"):
    - Ventana: diálogo antes de cada prueba con qué se mide, qué publica M-Lab (IP pública, fecha/hora y metadatos de la prueba, en datos abiertos CC0, conservados indefinidamente), el consumo de datos y enlaces a la [política de privacidad de M-Lab](https://www.measurementlab.net/privacy/) y a la de NPerf. Sin aceptar no se mide.
    - CLI: el mismo texto por stderr y confirmación (`s/N`) por la entrada estándar antes de cualquier medición, salvo el flag explícito `-accept-mlab`. Sin terminal interactiva y sin `-accept-mlab` → error de uso, sin tráfico.
  - **Política de privacidad de NPerf** propia y publicada (`docs/privacidad.md`), compatible con la de M-Lab ([AUP](https://www.measurementlab.net/aup/)): NPerf no envía datos fuera del equipo salvo la prueba a M-Lab; qué publica M-Lab y en qué condiciones.
  - **Límite de 40 pruebas interactivas al día por equipo** ([AUP](https://www.measurementlab.net/aup/)): contador local persistente compartido por la ventana y la CLI; la 41.ª del día se rechaza sin tráfico con un mensaje claro. Una prueba = descarga + subida de una pulsación o un `-speed`.
  - Servidor elegido con **Locate v2** ([locate-v2](https://www.measurementlab.net/develop/locate-v2)), el más cercano; las URLs devueltas se usan tal cual, sin quitar parámetros (p. ej. `access_token`).
  - Protocolo [ndt7](https://github.com/m-lab/ndt-server/blob/main/spec/ndt7-protocol.md): descarga y luego subida por WebSocket sobre TLS, **hasta ~10 s por sentido** (el servidor puede cerrar a los 13 s), tope duro 15 s por sentido. User-Agent y metadatos que identifican al cliente (`NPerf/<versión>`).
  - **Prohibido** alterar los resultados, compensar al usuario por medir o interferir con la plataforma: un solo servidor, una conexión por sentido, sin reintentos automáticos, sin pruebas en paralelo.
  - Muestra Mbps de descarga y subida, RTT mínimo informado por el servidor si lo da, servidor usado y hora.
  - Aviso visible junto al botón, en el resultado, en la salida de la CLI (stderr) y en el informe: "M-Lab publica en sus datos abiertos la IP pública de quien mide" y que la prueba consume datos (cientos de MB en enlaces rápidos).
  - Fallos → "no disponible" con motivo, sin abortar el resto: sin respuesta de locate / sin servidor / límite de M-Lab, conexión o TLS bloqueados (firewall o proxy), cortes a mitad (se guarda lo medido de cada sentido y se marca incompleto), respuestas fuera de protocolo, límite diario alcanzado.
  - Sin dependencias Go nuevas: cliente WebSocket (RFC 6455, solo lo que usa NDT7) y NDT7 propios con la librería estándar (`crypto/tls`, `net/http`).
  - Mide hasta el servidor de M-Lab, no solo al proveedor; no entra en el veredicto LAN/internet.
- Exclusión de §19 ampliada: latencia y velocidad a internet no corren durante un TEST! ni a la vez que otro diagnóstico, y viceversa.
- **CLI**:
  - `nperf history list [-kind] [-host] [-since] [-json]`, `nperf history show <id> [-json]`, `nperf history export <id...>|-all -format csv|json|html [-csv std|excel-es] -o <ruta>`.
  - `nperf internet [-speed [-accept-mlab]] [-json]`, códigos de salida de §19.
  - La CLI solo guarda en el historial con `-save` (en `client`, `ping`, `trace`, `mtu`, `ports`, `internet`).
  - JSON aditivo, `format_version` 1. Comandos y flags en inglés, mensajes en español.

### Excluye
- Sincronizar, compartir o importar historiales entre equipos.
- Pruebas programadas, alertas, notificaciones y registro de eventos en texto (§13: spec posterior). Pruebas M-Lab automáticas o programadas.
- Mapa de red, ping continuo con gráfica y guardar el monitor de tarjetas.
- IPv6 hacia internet; medir el DNS del proveedor; elegir servidor M-Lab a mano; otros proveedores de velocidad (Ookla, Cloudflare).
- Proxy HTTP para M-Lab (con proxy obligatorio → "no disponible" con motivo).
- PDF generado por NPerf; cifrar el historial (lo protegen los permisos del perfil).
- Dependencias Go o JS nuevas; recursos externos en la UI o en el informe.

## Criterios de aceptación

### Verificables en CI
1. Guardado (directorio temporal, reloj inyectado):
   - Cada tipo (TEST!, cancelado, error, ping, trace, MTU, puertos, combinado, latencia y velocidad a internet) produce un registro que, releído, es igual al resultado original, con `intervals` y `loaded_rtt`.
   - Ningún registro contiene la clave, la MAC ni el BSSID (test que busca valores de prueba en todos los archivos).
2. Retención: por encima de 90 días o 50 MB se borran primero los más antiguos hasta cumplir. Desactivado, no se escribe nada.
3. Robustez:
   - Archivo corrupto, mayor que el tope o con tipos incorrectos se omite y el resto se lista.
   - Dos escritores concurrentes (`-race`) no pierden ni mezclan registros.
   - Un fallo de disco no aborta la prueba: se muestra el resultado y "no se pudo guardar en el historial".
4. Filtros por equipo, tipo y fechas; borrar uno, varios y todo; orden por fecha.
5. CSV:
   - Cabeceras fijas, una fila por fase y sentido, BOM; `excel-es` usa `;` y coma decimal, `std` usa `,` y punto.
   - Campos con separador, comillas o saltos de línea se escapan.
   - Valores que empiezan por `=`, `+`, `-` o `@` se neutralizan (inyección de fórmulas).
   - Sentido no medido → celda vacía y estado.
6. Informe HTML:
   - Contiene fecha, equipos, IPs, SSID, resultados y gráficas SVG; no tiene `<script>`, `http(s)://` en `src`/`href`/`url()` ni otros recursos externos (salvo el enlace de texto de atribución a M-Lab, sin carga).
   - Los datos de red están escapados (test con alias y SSID `<script>`).
   - No contiene clave, MAC ni BSSID; con prueba M-Lab lleva atribución y aviso de IP pública.
7. Veredicto de latencia con `Echoer`/`Dialer` simulados: tabla de casos (todo sano, proveedor, LAN, sin salida, ICMP filtrado con TCP 443 abierto, límites 2 % / 1,99 %, 100 / 100,1 ms, 30 / 30,1 ms). El traceroute solo corre en "proveedor". Destinos: por defecto los dos, máx. 4, se rechazan no IPv4 unicast públicos.
8. WebSocket y NDT7 contra un servidor TLS de prueba local (sin red externa):
   - Handshake (clave/aceptación, subprotocolo `net.measurementlab.ndt.v7`), máscara en tramas del cliente, fragmentos, ping/pong y cierre.
   - Descarga y subida miden Mbps dentro de ±5 % de lo que envía/recibe el servidor de prueba.
   - Servidor que no cierra → el cliente corta a los 15 s (reloj inyectado); cierre del servidor a los 13 s es un final normal; trama mayor que el tope o JSON de medición inválido → error o se ignora, nunca pánico.
   - Locate caído, sin resultados, límite de M-Lab (429), TLS rechazado, conexión rechazada y corte a mitad → "no disponible" con el motivo correspondiente; lo medido se conserva y se marca incompleto.
9. CLI:
   - `history list|show|export` e `internet` (con y sin `-speed`, con servidor de prueba) con y sin `-json`, con los códigos de salida correctos; sin `-save` no escribe en el historial, con `-save` sí.
   - El JSON lleva `format_version: 1` y `command`; el aviso de M-Lab va por stderr.
   - Los tests de las specs 001–005 pasan sin modificarlos.
10. Backend de la interfaz:
    - El puente valida las funciones nuevas (lista blanca y tipos; ids de historial sin rutas, `..` ni separadores).
    - Exclusión TEST!/diagnóstico/internet respetada; la velocidad M-Lab solo arranca por su llamada explícita.
    - La clave nunca aparece. El test que prohíbe innerHTML sigue pasando.
11. Consentimiento:
    - Backend: la velocidad M-Lab no arranca sin el consentimiento explícito de esa llamada; cada prueba lo pide de nuevo (no se recuerda).
    - La UI muestra el diálogo antes de cada prueba, con el texto del aviso y los enlaces fijos a la política de M-Lab y a la de NPerf (abiertos por el puente con URL fija).
    - CLI: sin `-accept-mlab` pregunta por stderr y lee la respuesta; "n" o vacío → no mide (código 1); sin terminal interactiva → código 2 sin tráfico (el servidor de prueba no recibe nada); `-accept-mlab` sin `-speed` → código 2.
12. Límite diario (reloj y directorio inyectados): 40 pruebas en el mismo día se permiten y la 41.ª se rechaza sin llamar a locate, con mensaje claro; al cambiar de día vuelve a 0; el contador sobrevive a reiniciar y lo comparten ventana y CLI; reservas concurrentes (`-race`) nunca superan 40; archivo corrupto no rompe la app.
13. Locate v2 e identificación: la petición va a `/v2/nearest/ndt/ndt7` con `client_name`/`client_version`; las URLs de descarga y subida se usan byte a byte como las devolvió locate (con `access_token`); User-Agent `NPerf/<versión>` en locate y en el handshake WebSocket; solo se aceptan URLs `wss://`.
14. Integridad: los Mbps mostrados y guardados son los medidos (sin factores ni ajustes; test con tasa conocida del servidor de prueba); una sola conexión por sentido, un servidor y ningún reintento automático (contadores en el servidor de prueba).
15. `go vet` y `go test ./...` pasan en windows-latest; ahí se comprueba que el historial se crea bajo `%LocalAppData%\NPerf` sin administrador. `go.mod` sin dependencias nuevas.

### Validación manual en Windows 10/11
16. Tras un TEST! "Ambos" de 30 s, un ping y un traceroute, cerrar y reabrir NPerf: los tres están en el historial y el TEST! se ve igual que al terminar, con gráficas.
17. El CSV de la ventana se abre en Excel (configuración regional española) con columnas y números correctos, sin pasos de importación.
18. El informe se abre en el navegador sin conexión y se imprime/guarda como PDF legible en A4, con temas claro y oscuro del sistema.
19. Latencia a internet: red sana → "sin problemas"; WAN del router desconectado → "sin salida a internet"; WiFi degradado (lejos del AP) → LAN; red que bloquea ICMP saliente → "ICMP filtrado".
20. Velocidad M-Lab: con internet da descarga y subida en ≤ 35 s en total, del mismo orden que otra prueba web hecha a la vez (±25 %, orientativo); con el puerto 443 bloqueado en el firewall de Windows da "no disponible" con motivo; el aviso de IP pública se ve antes de pulsar y el diálogo de consentimiento aparece en cada prueba.
21. Borrar todo vacía la carpeta. Con el historial desactivado, un TEST! no crea archivos.
22. Privacidad: `docs/privacidad.md` está publicada en la URL que abre NPerf, los dos enlaces del diálogo abren la página correcta y su contenido coincide con lo que hace NPerf (revisión del usuario).

## Riesgos
- **Privacidad**: historial e informe guardan IPs, alias y SSID de redes de clientes; el informe sale del equipo en manos del técnico (decisión del usuario: siempre incluidos). Mitigan la retención y "borrar todo".
- **M-Lab publica la IP pública** de quien mide como datos abiertos CC0 con retención indefinida (riesgo aceptado por el usuario); se avisa y se pide consentimiento en cada prueba.
- **Términos de M-Lab**: verificados por el usuario ([develop](https://www.measurementlab.net/develop/), [AUP](https://www.measurementlab.net/aup/), [privacy](https://www.measurementlab.net/privacy/), [locate-v2](https://www.measurementlab.net/develop/locate-v2), [ndt7](https://github.com/m-lab/ndt-server/blob/main/spec/ndt7-protocol.md)). Pueden cambiar: se revisan antes de cada versión publicada.
- **Uso comercial (pendiente de producto, no bloquea)**: si NPerf se comercializa, la AUP pide a los integradores comerciales una contribución a M-Lab (patrocinio o en especie).
- **Límite diario por perfil**: el contador vive en el perfil del usuario de Windows; con varios usuarios en el mismo PC cada uno cuenta aparte.
- **Cliente WebSocket propio**: viable con la librería estándar (handshake HTTP/1.1 Upgrade sobre `crypto/tls`, framing RFC 6455 con máscara), del orden de cientos de líneas; riesgo de errores de protocolo, mitigado con el servidor de prueba del criterio 8. Alternativa si resultara inviable: `golang.org/x/net/websocket` (módulo oficial Go, BSD), que requeriría aprobación como dependencia nueva.
- **Cambios en M-Lab**: la API locate o el protocolo pueden cambiar o limitar clientes; el fallo queda en "no disponible" sin afectar al resto.
- **Proxy y firewall corporativos**: Go no usa el proxy del sistema de Windows; en redes con proxy obligatorio la velocidad dará "no disponible".
- **Consumo de datos**: en enlaces rápidos una prueba puede mover cientos de MB o más; solo a petición, con aviso y consentimiento.
- **DNS públicos y ICMP**: pueden despriorizar ICMP y dar falsos "degradado"; por eso dos destinos y umbral con margen (supuesto).
- **Excel y configuración regional**: el CSV `excel-es` puede leerse mal en Excel en otro idioma o en scripts (por eso la CLI usa `std`).
- **Tamaño**: un TEST! de 300 s con 4 fases ronda las 1200 muestras; acotar tamaño por registro y tiempo de carga de la lista (en el plan).

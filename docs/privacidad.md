# Política de privacidad de NPerf

NPerf es una herramienta de medición de red para Windows. No recoge telemetría y no envía datos a sus autores. Esta página explica qué datos maneja, dónde se quedan y qué sale del equipo.

## 1. Datos que se guardan en tu equipo

Todo es local, en tu perfil de Windows (`%LocalAppData%\NPerf` y la carpeta de configuración de NPerf):

- **Configuración**: alias del equipo, duración por defecto, puertos, subredes y destinos manuales, y la clave compartida (opcional).
- **Historial** (carpeta `historial`): resultados de pruebas y diagnósticos, con direcciones IP, alias, nombre de la red WiFi (SSID) y los resultados medidos.
- **Contador diario** de pruebas de velocidad a M-Lab (`mlab-diario.json`).

La clave **nunca** se guarda en el historial ni se envía por la red (solo se usa para comprobar la identidad mediante HMAC).

**Retención**: el historial conserva como máximo 90 días o 50 MB; se borra primero lo más antiguo. Puedes desactivarlo en Ajustes (entonces no se escribe nada) y borrarlo todo con "Borrar todo". En la CLI solo se guarda con `-save`. Los archivos no están cifrados: los protegen los permisos de tu perfil de Windows. Exportar un resultado lo entrega a quien tú elijas; NPerf no lo envía a ningún sitio.

## 2. Qué viaja por la red local

- **Anuncio de descubrimiento** (UDP, puerto 5301): alias, tipo de enlace, velocidad del enlace y SSID. Es visible para cualquier equipo de la misma red; así está decidido en el diseño de NPerf.
- **Pruebas entre equipos NPerf**: se autentican con la clave compartida (HMAC). La clave no viaja.

## 3. Qué sale a internet

- **Latencia**: pings a destinos públicos (por defecto 1.1.1.1 y 8.8.8.8) o a los que configures. Esos destinos ven tu IP pública, como en cualquier ping.
- **Prueba de velocidad con M-Lab (NDT7)**: solo a petición (botón o `nperf internet -speed`) y **tras tu consentimiento antes de cada prueba** (en la CLI, confirmación o `-accept-mlab`). Hay un máximo de 40 pruebas interactivas al día por perfil de usuario de Windows (cada usuario del PC tiene su propia cuota). Nunca se hacen de forma automática ni programada.

## 4. Qué publica M-Lab

Al hacer una prueba, Measurement Lab (M-Lab) publica tu **IP pública**, la fecha y hora y metadatos de la medición como datos abiertos bajo licencia **CC0**, y los conserva **indefinidamente**. No puede deshacerse. Consulta su política: https://www.measurementlab.net/privacy/ y su política de uso aceptable: https://www.measurementlab.net/aup/. Si no lo aceptas, no uses la prueba de velocidad; el resto de NPerf funciona sin ella.

## 5. Contacto

NPerf es de código abierto: https://github.com/scouter190/Nperf (consultas e incidencias en el repositorio).

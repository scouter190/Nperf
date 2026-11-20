# NPerf

NPerf es una herramienta para medir el rendimiento de la red local entre equipos Windows: velocidad TCP y UDP en ambos sentidos, latencia, jitter, pérdida, diagnósticos (ping, traza, MTU, puertos), historial y prueba de velocidad a internet. No recoge telemetría.

## Requisitos
- Windows 10 u 11 (64 bits). No necesita privilegios de administrador.
- Runtime de Microsoft Edge WebView2 (viene con Windows 11 y con Edge actualizado) para la ventana gráfica.

## Contenido
- `nperf.exe`: consola (servidor y cliente).
- `nperf-gui.exe`: ventana gráfica con bandeja del sistema.

Ejecuta `nperf help` para ver todos los comandos y opciones.

## Primer uso
1. En cada equipo, abre `nperf-gui.exe` (o ejecuta `nperf server`).
2. Define la misma clave compartida en ambos (Ajustes en la ventana, o `-key`).
3. Permite NPerf en el firewall de Windows solo para **red privada** cuando aparezca el aviso.
4. Elige el otro equipo en la lista y pulsa TEST!, o usa `nperf client <host>`.

## Más información
- Privacidad: [docs/privacidad.md](docs/privacidad.md)
- Cambios: [CHANGELOG.md](CHANGELOG.md)
- Licencias de terceros: [docs/licencias-terceros.txt](docs/licencias-terceros.txt)

## Licencia
Todos los derechos reservados (c) 2026 scouter190: el código se publica solo para consulta, sin permiso de uso, copia, modificación ni redistribución sin autorización escrita. Ver [LICENSE](LICENSE); los componentes de terceros conservan sus licencias.

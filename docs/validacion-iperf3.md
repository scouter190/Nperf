# Validación manual frente a iperf3 (criterios 12 y 13)

Objetivo: comprobar en tu red que (12) el throughput TCP de NPerf queda dentro de ±10 % de `iperf3 -P 4 -t 10`, en ambos sentidos, y (13) que todo funciona sin privilegios de administrador.

## 1. Requisitos
- Dos equipos Windows en la misma LAN: **A** (cliente) y **B** (servidor).
- NPerf compilado en ambos: `.\scripts\build-windows.ps1` (o el artefacto `nperf-windows` del CI).
- iperf3 para Windows: descarga el binario oficial desde la página de descargas de iperf (iperf.fr / software.es.net/iperf). Verifica que la fuente sea la oficial; no requiere instalación, basta descomprimir y copiar a ambos equipos.
- Firewall de Windows: al primer arranque permite `nperf.exe` e `iperf3.exe` **solo en red privada** (desmarca "Pública").
- Usa una cuenta de usuario normal. No uses "Ejecutar como administrador".

## 2. Preparación
```powershell
# En B: anota la IP
ipconfig
```
- Misma clave en ambos lados: `-key MI_CLAVE` o `config.json` en `%APPDATA%\NPerf` (`{"key":"MI_CLAVE"}`).
- Sin otras cargas pesadas en la red (descargas, streaming, copias).
- Anota el tipo de enlace de A y B: cable o WiFi: ______ / ______
- NPerf usa el puerto 5300 e iperf3 el 5201: pueden estar activos a la vez.

## 3. Pasos
En **B**, en dos ventanas PowerShell:
```powershell
.\nperf.exe server -key MI_CLAVE
```
```powershell
.\iperf3.exe -s
```

### Sentido fwd (A envía a B)
En **A**:
```powershell
.\nperf.exe client <IP-B> -key MI_CLAVE -proto tcp -direction fwd -streams 4 -duration 10 -json
.\iperf3.exe -c <IP-B> -P 4 -t 10
```

### Sentido rev (B envía a A)
```powershell
.\nperf.exe client <IP-B> -key MI_CLAVE -proto tcp -direction rev -streams 4 -duration 10 -json
.\iperf3.exe -c <IP-B> -P 4 -t 10 -R
```

Repite **3 veces** cada sentido, alternando el orden (rep. 1: NPerf primero; rep. 2: iperf3 primero; rep. 3: NPerf primero). Espera unos segundos entre pruebas. De iperf3 toma la línea `[SUM]` del receptor (Mbits/sec). De NPerf toma `tcp.fwd.throughput_mbps` (sentido fwd) o `tcp.rev.throughput_mbps` (sentido rev) del JSON. Para extraerlo directamente:
```powershell
(.\nperf.exe client <IP-B> -key MI_CLAVE -proto tcp -direction fwd -streams 4 -duration 10 -json | ConvertFrom-Json).tcp.fwd.throughput_mbps
```
Cambia `fwd` por `rev` (en `-direction` y en la ruta) para el otro sentido. Con `-json`, `status` debe ser `measured`.

## 4. Resultados
Diferencia % = |NPerf − iperf3| / iperf3 × 100. Criterio: **≤ 10 %**, usando la mediana de las 3 repeticiones por sentido.

| Sentido | Rep. | NPerf Mbps | iperf3 Mbps | Diferencia % |
|---|---|---|---|---|
| fwd | 1 | | | |
| fwd | 2 | | | |
| fwd | 3 | | | |
| fwd | mediana | | | |
| rev | 1 | | | |
| rev | 2 | | | |
| rev | 3 | | | |
| rev | mediana | | | |

Criterio 12 cumplido si ambas medianas dan ≤ 10 %.

## 5. Criterio 13: sin administrador
- No usaste "Ejecutar como administrador" en ningún paso.
- La barra de título de PowerShell **no** dice "Administrador".
- Todas las pruebas de la sección 3 se completaron igualmente.

Criterio 13 cumplido si las tres afirmaciones son ciertas.

## 6. Si no cumple
Reporta: tipo de red (cable/WiFi) de A y B, router/switch, velocidad nominal del enlace, la tabla completa y las salidas `-json` de NPerf y de iperf3 completas (sin recortar).

Problemas comunes (los textos entre comillas son los de NPerf):
- **Firewall bloquea**: la conexión no se establece y NPerf informa "no se pudo conectar a <dirección> (tiempo agotado)". Con UDP puede aparecer "el servidor no respondió al saludo UDP (¿puerto UDP bloqueado?)". Revisa que NPerf e iperf3 estén permitidos en red privada y que la red esté marcada como privada.
- **Puerto ocupado**: otro proceso usa 5300 o 5201. Cambia con `-port` en ambos lados (iperf3: `-p`).
- **Clave distinta**: el cliente muestra `error: el servidor respondió: clave incorrecta` (o, si la clave mala es la del servidor, `el servidor no demostró conocer la clave: clave incorrecta`), con código de salida 1. Usa la misma `-key` o el mismo `config.json`.
- **Servidor ocupado**: NPerf encola clientes (1 en curso, 5 en espera); el sexto recibe "cola llena". Espera o reintenta.
- Diferencias grandes entre repeticiones suelen indicar WiFi o carga ajena: repite con cable.

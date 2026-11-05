# Entorno local en Windows 10/11
## 1. Instalar herramientas

En PowerShell:

```powershell
winget install --id Git.Git -e
winget install --id GoLang.Go -e
```

Go debe ser 1.27.3 (alternativa: instalador de esa versión desde https://go.dev/dl/). Instala también Claude Code siguiendo https://docs.claude.com/en/docs/claude-code. Reabre PowerShell y comprueba:

```powershell
git --version
go version
claude --version
```

## 2. Clonar el repositorio

```powershell
cd "$HOME\Documents"
git clone https://github.com/scouter190/Nperf.git
cd Nperf
```

## 3. Verificar

```powershell
go vet ./...
go test ./...
go build -o nperf.exe ./cmd/nperf
```

## 4. Probar en la misma máquina

Terminal 1 (servidor, puerto 5300):

```powershell
.\nperf.exe server
```

Terminal 2 (cliente):

```powershell
.\nperf.exe client 127.0.0.1
```

Opciones: `-port` (ambos) y `-duration` (cliente, segundos, por defecto 10).

## 5. Firewall de Windows

Al abrir el puerto 5300 con `nperf.exe server`, Windows mostrará un aviso del Firewall. Marca **Redes privadas** y pulsa **Permitir acceso**. Para probar entre dos PC, hazlo en el PC servidor y usa su IP en el cliente (`.\nperf.exe client <IP-del-servidor>`). En loopback (127.0.0.1) normalmente no hace falta.

## 6. Trabajar con Claude Code

```powershell
cd "$HOME\Documents\Nperf"
claude --agent coordinator
```

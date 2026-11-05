# Compila nperf.exe (consola) y nperf-gui.exe (GUI) de forma nativa en Windows, en la raíz del repo.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
go build -o nperf.exe ./cmd/nperf
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -ldflags '-H=windowsgui' -o nperf-gui.exe ./cmd/nperf-gui
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host 'OK: nperf.exe nperf-gui.exe'

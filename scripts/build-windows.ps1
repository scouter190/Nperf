# Compila nperf.exe de forma nativa en Windows, en la raíz del repo.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
go build -o nperf.exe ./cmd/nperf
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host 'OK: nperf.exe'

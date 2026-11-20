# Compila nperf.exe (consola) y nperf-gui.exe (GUI) de forma nativa en Windows, en la raíz del repo.
# Versión: tag exacto vX.Y.Z en HEAD -> X.Y.Z; si no, "dev".
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
$ver = 'dev'
$tag = ''
try {
    $ErrorActionPreference = 'Continue'
    $tag = "$(git describe --tags --exact-match --match 'v[0-9]*' 2>$null)".Trim()
} catch { $tag = '' } finally { $ErrorActionPreference = 'Stop' }
if ($tag -match '^v(\d+\.\d+\.\d+)$') { $ver = $Matches[1] }
$ldx = "-X nperf/internal/version.Build=$ver"
go build -trimpath -ldflags $ldx -o nperf.exe ./cmd/nperf
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -trimpath -ldflags "$ldx -H=windowsgui" -o nperf-gui.exe ./cmd/nperf-gui
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "OK: nperf.exe nperf-gui.exe (versión $ver)"

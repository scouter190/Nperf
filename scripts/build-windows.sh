#!/usr/bin/env bash
# Compila nperf.exe (consola) y nperf-gui.exe (GUI) para Windows amd64
# (compilación cruzada) en la raíz del repo.
# Versión: tag exacto vX.Y.Z en HEAD -> X.Y.Z; si no, "dev".
set -euo pipefail
cd "$(dirname "$0")/.."
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
ver=dev
if tag=$(git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null); then
  if [[ "$tag" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
    ver="${BASH_REMATCH[1]}"
  fi
fi
ldx="-X nperf/internal/version.Build=$ver"
go build -trimpath -ldflags "$ldx" -o nperf.exe ./cmd/nperf
go build -trimpath -ldflags "$ldx -H=windowsgui" -o nperf-gui.exe ./cmd/nperf-gui
echo "OK: nperf.exe nperf-gui.exe (versión $ver)"

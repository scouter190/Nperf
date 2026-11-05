#!/usr/bin/env bash
# Compila nperf.exe (consola) y nperf-gui.exe (GUI) para Windows amd64
# (compilación cruzada) en la raíz del repo.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOOS=windows GOARCH=amd64 CGO_ENABLED=0
go build -o nperf.exe ./cmd/nperf
go build -ldflags "-H=windowsgui" -o nperf-gui.exe ./cmd/nperf-gui
echo "OK: nperf.exe nperf-gui.exe"

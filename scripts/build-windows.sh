#!/usr/bin/env bash
# Compila nperf.exe para Windows amd64 (compilación cruzada) en la raíz del repo.
set -euo pipefail
cd "$(dirname "$0")/.."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o nperf.exe ./cmd/nperf
echo "OK: nperf.exe"

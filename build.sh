#!/usr/bin/env bash
# Script de compilación para las máquinas virtuales.
# Requiere Go 1.22+ instalado (sudo apt-get install -y golang-go)
set -e
echo "[build] Compilando expendedora..."
GOPROXY=off GOFLAGS="-mod=vendor" go build -o expendedora ./cmd/expendedora/
echo "[build] Binario 'expendedora' generado correctamente."

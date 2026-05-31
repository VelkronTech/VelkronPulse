#!/bin/bash
# Velkron Pulse — Cross-Compilation Script
# Builds static binaries for all target platforms.
# Requires Go 1.21+.
#
# Usage:
#   chmod +x build/build-all.sh
#   ./build/build-all.sh

set -e

VERSION="1.0.1"
LDFLAGS="-s -w -X main.version=${VERSION}"
OUTDIR="build"

mkdir -p "${OUTDIR}"

echo "=== Velkron Pulse Cross-Compilation ==="
echo "Version: ${VERSION}"
echo ""

build() {
    local GOOS="$1"
    local GOARCH="$2"
    local SUFFIX="$3"
    local OUTPUT="${OUTDIR}/velkron-pulse-${GOOS}-${GOARCH}${SUFFIX}"

    echo "Building ${OUTPUT}..."
    GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED=0 go build \
        -ldflags="${LDFLAGS}" \
        -o "${OUTPUT}" \
        .
    echo "  -> ${OUTPUT} ($(du -h "${OUTPUT}" | cut -f1))"
}

# Linux
build "linux" "amd64" ""
build "linux" "arm64" ""

# macOS
build "darwin" "amd64" ""
build "darwin" "arm64" ""

# Windows
build "windows" "amd64" ".exe"

echo ""
echo "=== Build complete ==="
ls -lh "${OUTDIR}/"

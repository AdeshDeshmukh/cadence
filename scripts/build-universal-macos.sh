#!/usr/bin/env bash
set -euo pipefail

# scripts/build-universal-macos.sh
# Builds macOS Universal (Fat) Mach-O binaries containing arm64 and x86_64
# for both cadence (CLI) and cadenced (daemon).

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist/bin/darwin-universal"
TMP_DIR="$(mktemp -d -t cadence-universal-build-XXXXXX)"

trap 'rm -rf "${TMP_DIR}"' EXIT

echo "==> Building Cadence macOS Universal (arm64 + x86_64) Binaries"
mkdir -p "${DIST_DIR}"

build_binary() {
    local name="$1"
    local pkg="$2"

    echo "--- Building ${name} ---"

    echo "  [1/3] Compiling darwin/arm64..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "${TMP_DIR}/${name}-arm64" "${pkg}"

    echo "  [2/3] Compiling darwin/amd64..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "${TMP_DIR}/${name}-amd64" "${pkg}"

    echo "  [3/3] Assembling universal Mach-O fat binary..."
    local out_bin="${DIST_DIR}/${name}"

    if command -v lipo >/dev/null 2>&1; then
        lipo -create -output "${out_bin}" "${TMP_DIR}/${name}-amd64" "${TMP_DIR}/${name}-arm64"
    else
        echo "  Notice: lipo not found in PATH; using Go-native Mach-O fat assembler..."
        go run -tags build ./scripts/pack_universal.go "${TMP_DIR}/${name}-arm64" "${TMP_DIR}/${name}-amd64" "${out_bin}"
    fi

    chmod +x "${out_bin}"

    echo "  ✓ Verifying ${out_bin}:"
    if command -v file >/dev/null 2>&1; then
        file "${out_bin}"
    fi
    if command -v lipo >/dev/null 2>&1; then
        lipo -info "${out_bin}"
    fi
    ls -lh "${out_bin}"
    echo ""
}

cd "${REPO_ROOT}"

build_binary "cadence" "./cmd/cadence"
build_binary "cadenced" "./cmd/cadenced"

echo "==> macOS Universal Binaries successfully generated at:"
echo "    - ${DIST_DIR}/cadence"
echo "    - ${DIST_DIR}/cadenced"

#!/usr/bin/env bash
set -euo pipefail

# scripts/package-deb.sh
# Packages Cadence for Debian / Ubuntu (.deb)

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${CADENCE_VERSION:-1.0.0}"
ARCH="${DEB_ARCH:-amd64}"
DIST_PACKAGES="${REPO_ROOT}/dist/packages"
TMP_DIR="$(mktemp -d -t cadence-deb-XXXXXX)"

trap 'rm -rf "${TMP_DIR}"' EXIT

mkdir -p "${DIST_PACKAGES}"

echo "==> Building Cadence Debian Package (v${VERSION}, ${ARCH})"

# 1. Compile Linux binaries if not already present
BIN_DIR="${TMP_DIR}/bin"
mkdir -p "${BIN_DIR}"

echo "  Compiling Linux binaries..."
CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build -ldflags="-s -w" -o "${BIN_DIR}/cadence" "${REPO_ROOT}/cmd/cadence"
CGO_ENABLED=0 GOOS=linux GOARCH="${ARCH}" go build -ldflags="-s -w" -o "${BIN_DIR}/cadenced" "${REPO_ROOT}/cmd/cadenced"

# 2. Package .deb
OUT_DEB="${DIST_PACKAGES}/cadence_${VERSION}_${ARCH}.deb"

echo "  Building .deb package..."
go run "${REPO_ROOT}/scripts/pack_deb.go" \
  "${BIN_DIR}/cadence" \
  "${BIN_DIR}/cadenced" \
  "${REPO_ROOT}/packaging/deb/systemd/cadenced.service" \
  "${OUT_DEB}"

echo "  ✓ Generated Debian Package: ${OUT_DEB}"
ls -lh "${OUT_DEB}"

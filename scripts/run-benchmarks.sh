#!/usr/bin/env bash
set -euo pipefail

# scripts/run-benchmarks.sh
# Runs Cadence automated benchmark suite and outputs performance metrics.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${REPO_ROOT}/dist"

mkdir -p "${DIST_DIR}"

echo "==> Running Cadence Performance Benchmark Suite..."
go test -v ./tests/benchmarks/...

echo "==> Generating Benchmark Markdown Report..."
go run ./cmd/cadence benchmark report > "${DIST_DIR}/benchmarks-local.md"

echo "  ✓ Benchmark report saved to ${DIST_DIR}/benchmarks-local.md"
cat "${DIST_DIR}/benchmarks-local.md"

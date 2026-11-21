#!/usr/bin/env bash
set -euo pipefail

# scripts/package-all.sh
# Master release packaging script generating all release artifacts:
# 1. Homebrew Formula (Formula/cadence.rb)
# 2. Debian Package (.deb)
# 3. RPM Spec & Package (.rpm)
# 4. Windows Installer (Inno Setup & NSIS & PowerShell)
# 5. macOS Universal Binaries

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_PACKAGES="${REPO_ROOT}/dist/packages"

echo "============================================================"
echo "    Cadence Multi-Platform Release Packaging Pipeline       "
echo "============================================================"

mkdir -p "${DIST_PACKAGES}"

# 1. Homebrew Formula
echo "==> [1/4] Verifying Homebrew Formula..."
test -f "${REPO_ROOT}/Formula/cadence.rb"
echo "  ✓ Homebrew formula present at Formula/cadence.rb"

# 2. Debian Package (.deb)
echo "==> [2/4] Generating Debian (.deb) Package..."
"${REPO_ROOT}/scripts/package-deb.sh"

# 3. RPM Package (.rpm)
echo "==> [3/4] Packaging RPM..."
"${REPO_ROOT}/scripts/package-rpm.sh"

# 4. Windows Installer
echo "==> [4/4] Staging Windows Installer..."
mkdir -p "${DIST_PACKAGES}/windows"
cp "${REPO_ROOT}/packaging/windows/cadence.iss" "${DIST_PACKAGES}/windows/"
cp "${REPO_ROOT}/packaging/windows/installer.nsi" "${DIST_PACKAGES}/windows/"
cp "${REPO_ROOT}/packaging/windows/install.ps1" "${DIST_PACKAGES}/windows/"
echo "  ✓ Windows installer specifications staged."

echo ""
echo "==> Release Packages successfully assembled in:"
echo "    ${DIST_PACKAGES}"
ls -lh "${DIST_PACKAGES}"

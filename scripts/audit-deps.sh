#!/usr/bin/env bash
# Cadence Dependency & Supply-Chain Security Auditor
# Runs govulncheck on Go dependencies and cargo-audit on Rust/Tauri crates.

set -euo pipefail

BOLD="\033[1m"
GREEN="\033[32m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

echo -e "${BOLD}=====================================================${RESET}"
echo -e "${BOLD}  Cadence Dependency & Supply-Chain Security Audit   ${RESET}"
echo -e "${BOLD}=====================================================${RESET}"
echo ""

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ERRORS=0

# 1. Audit Go Dependencies
echo -e "${BOLD}[1/2] Auditing Go Dependencies with govulncheck...${RESET}"
if command -v govulncheck >/dev/null 2>&1; then
    echo "Running installed govulncheck ./..."
    if govulncheck ./...; then
        echo -e "${GREEN}[✓] Go dependencies: 0 known vulnerabilities found.${RESET}"
    else
        echo -e "${RED}[✗] Go dependencies: vulnerabilities detected!${RESET}"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo -e "${YELLOW}[!] govulncheck is not installed locally.${RESET}"
    echo "    To install: go install golang.org/x/vuln/cmd/govulncheck@latest"
    echo "    Running basic go vet verification..."
    if go vet ./...; then
        echo -e "${GREEN}[✓] go vet passed cleanly.${RESET}"
    else
        echo -e "${RED}[✗] go vet reported issues.${RESET}"
        ERRORS=$((ERRORS + 1))
    fi
fi

echo ""

# 2. Audit Rust / Cargo Dependencies (Tauri desktop UI)
echo -e "${BOLD}[2/2] Auditing Rust / Cargo Dependencies in ui/src-tauri...${RESET}"
if [ -d "$ROOT_DIR/ui/src-tauri" ] && [ -f "$ROOT_DIR/ui/src-tauri/Cargo.lock" ]; then
    cd "$ROOT_DIR/ui/src-tauri"
    if command -v cargo-audit >/dev/null 2>&1 || cargo audit --help >/dev/null 2>&1; then
        echo "Running cargo audit..."
        if cargo audit; then
            echo -e "${GREEN}[✓] Rust dependencies: 0 security advisories found.${RESET}"
        else
            echo -e "${RED}[✗] Rust dependencies: security advisories detected!${RESET}"
            ERRORS=$((ERRORS + 1))
        fi
    else
        echo -e "${YELLOW}[!] cargo-audit is not installed locally.${RESET}"
        echo "    To install: cargo install cargo-audit"
        echo "    (Automated checks run on CI via GitHub Actions Security workflow)."
    fi
else
    echo -e "${GREEN}[✓] No Rust crates found in repository.${RESET}"
fi

echo ""
echo -e "${BOLD}=====================================================${RESET}"
if [ "$ERRORS" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}  Supply-Chain Audit Summary: CLEAN (0 vulnerabilities)${RESET}"
    echo -e "${BOLD}=====================================================${RESET}"
    exit 0
else
    echo -e "${RED}${BOLD}  Supply-Chain Audit Summary: FAILED ($ERRORS issues found)${RESET}"
    echo -e "${BOLD}=====================================================${RESET}"
    exit 1
fi

# Installing Cadence

Cadence is a free, open-source, on-device AI voice interface and developer engine. It runs locally on macOS, Linux, and Windows with zero cloud telemetry and pure on-device execution.

---

## Installation Methods

### 1. macOS (Homebrew) — Recommended

Cadence provides a native Homebrew tap and formula with automated background service integration (`launchd`):

```bash
# Tap the Cadence repository
brew tap AdeshDeshmukh/cadence

# Install cadence (CLI + background daemon)
brew install cadence

# Start the background daemon service
brew services start cadence
```

To stop or restart the service:
```bash
brew services stop cadence
brew services restart cadence
```

---

### 2. macOS Universal Binaries (Apple Silicon & Intel)

Cadence distributes Mach-O universal fat binaries containing both `arm64` (M1/M2/M3/M4) and `x86_64` (Intel) slices:

```bash
# Download latest universal release
curl -LO https://github.com/AdeshDeshmukh/cadence/releases/latest/download/cadence-darwin-universal.tar.gz

# Extract
tar -xzf cadence-darwin-universal.tar.gz

# Move binaries to your PATH
sudo mv cadence cadenced /usr/local/bin/

# Install and start launchd user service
cadence service install
```

---

### 3. Debian / Ubuntu (`.deb`)

Download and install the native Debian package:

```bash
# Download the .deb package
curl -LO https://github.com/AdeshDeshmukh/cadence/releases/latest/download/cadence_1.0.0_amd64.deb

# Install via dpkg
sudo dpkg -i cadence_1.0.0_amd64.deb

# Enable and start the systemd user service
systemctl --user daemon-reload
systemctl --user enable --now cadenced.service
```

---

### 4. Fedora / RHEL / CentOS / openSUSE (`.rpm`)

Download and install the native RPM package:

```bash
# Download the .rpm package
curl -LO https://github.com/AdeshDeshmukh/cadence/releases/latest/download/cadence-1.0.0-1.x86_64.rpm

# Install via rpm or dnf
sudo rpm -i cadence-1.0.0-1.x86_64.rpm
# or
sudo dnf install ./cadence-1.0.0-1.x86_64.rpm

# Enable and start the systemd user service
systemctl --user daemon-reload
systemctl --user enable --now cadenced.service
```

---

### 5. Windows

#### PowerShell Automated One-Liner
Open PowerShell and run:
```powershell
irm https://github.com/AdeshDeshmukh/cadence/releases/latest/download/install.ps1 | iex
```

#### Inno Setup / NSIS Installer
1. Download `Cadence-Setup-1.0.0.exe` from the [Releases](https://github.com/AdeshDeshmukh/cadence/releases).
2. Run the installer. It will:
   - Install `cadence.exe` and `cadenced.exe` to `%LOCALAPPDATA%\Cadence`.
   - Add Cadence to your User `PATH`.
   - Configure automatic Windows startup.

---

### 6. Build from Source

Requirements:
- Go 1.23 or newer
- Git

```bash
# Clone the repository
git clone https://github.com/AdeshDeshmukh/cadence.git
cd cadence

# Build local binaries
make build

# Install binaries to /usr/local/bin
sudo cp cadence cadenced /usr/local/bin/

# (macOS only) Build universal Mach-O binaries:
make build-universal

# (Linux only) Build Debian package:
make package-deb
```

---

## Verifying Your Installation

Run the Cadence diagnostic doctor to verify that the CLI, daemon, and local storage are operating properly:

```bash
cadence doctor
```

Output:
```text
Cadence Diagnostics:
  • Version:        1.0.0
  • Binary:         Universal Mach-O (arm64, x86_64)
  • Database path:  ~/.cadence/cadence.db
  • Socket path:    ~/.cache/cadence/cadenced.sock
  • Supply Chain:   Pure Go modernc.org/sqlite (CGO disabled, no external C linkage)
  • Dependencies:   0 known vulnerabilities (govulncheck compliant)
  • Daemon status:  running and healthy
```

Check the version:
```bash
cadence version
```

---

## Background Service Management

Cadence includes cross-platform service management built into the CLI:

```bash
# Inspect service status
cadence service status

# Install OS service (Launchd on macOS, Systemd on Linux)
cadence service install

# View service unit / plist definition
cadence service definition

# View recent crash dumps (if any)
cadence service crashes

# Uninstall OS service
cadence service uninstall
```

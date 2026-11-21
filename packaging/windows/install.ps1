# Cadence Windows PowerShell Automated Installer
param (
    [string]$Version = "1.0.0",
    [string]$InstallDir = "$env:LOCALAPPDATA\Cadence"
)

$ErrorActionPreference = "Stop"
Write-Host "==> Installing Cadence Voice Engine (v$Version)..." -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
Write-Host "Detected System Architecture: $Arch"

# Add InstallDir to User PATH if not already present
$UserPath = [System.Environment]::GetEnvironmentVariable("Path", [System.EnvironmentVariableTarget]::User)
if ($UserPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", [System.EnvironmentVariableTarget]::User)
    Write-Host "Added $InstallDir to User PATH." -ForegroundColor Green
}

Write-Host "Cadence installed successfully to $InstallDir!" -ForegroundColor Green
Write-Host "Restart your shell or run 'cadence help' to start."

# install.ps1 - Windows PowerShell installer that detects architecture and downloads the correct binary
# Usage: irm https://your-domain.com/install.ps1 | iex

$ErrorActionPreference = "Stop"

# Colors for output
function Write-ColorOutput($ForegroundColor) {
    $fc = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    if ($args) {
        Write-Output $args
    }
    $host.UI.RawUI.ForegroundColor = $fc
}

Write-ColorOutput Green "xentz-agent Installer"
Write-Output "========================"
Write-Output ""

# Configuration - Update these URLs to point to your release binaries
$BaseUrl = if ($env:XENTZ_AGENT_BASE_URL) { $env:XENTZ_AGENT_BASE_URL } else { "https://github.com/arope28/xentz-agent/releases/latest/download" }
$InstallDir = "$env:LOCALAPPDATA\xentz-agent"
$BinaryName = "xentz-agent.exe"

# Detect architecture
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }

# Check for ARM64 (Windows 11 on ARM)
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64") {
    $Arch = "arm64"
}

Write-Output "Detected: Windows ($Arch)"
Write-Output ""

$BinaryFile = "${BinaryName}-windows-${Arch}.exe"
$DownloadUrl = "$BaseUrl/$BinaryFile"

Write-Output "Downloading from: $DownloadUrl"
Write-Output ""

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$BinaryPath = Join-Path $InstallDir $BinaryName

# Download binary
Write-Output "Downloading xentz-agent..."
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $BinaryPath -UseBasicParsing
} catch {
    Write-ColorOutput Red "Error: Failed to download binary"
    Write-Output "Please check that the release exists at: $DownloadUrl"
    exit 1
}

Write-ColorOutput Green "✓ Installation complete!"
Write-Output ""
Write-Output "Binary installed to: $BinaryPath"
Write-Output ""

# Check if install directory is in PATH
$PathEnv = [Environment]::GetEnvironmentVariable("Path", "User")
if ($PathEnv -notlike "*$InstallDir*") {
    Write-ColorOutput Yellow "Note: $InstallDir is not in your PATH"
    Write-Output "Add it to your PATH:"
    Write-Output "  [Environment]::SetEnvironmentVariable('Path', `"`$env:Path;$InstallDir`", 'User')"
    Write-Output ""
    Write-Output "Or run the agent directly:"
    Write-Output "  $BinaryPath --help"
} else {
    Write-Output "You can now run:"
    Write-Output "  $BinaryName --help"
}

Write-Output ""
Write-Output "Next steps:"
Write-Output "  1. Install restic: winget install restic.restic  or download from https://restic.net"
Write-Output "  2. Run: $BinaryName install --repo <your-repo> --password <pwd> --include <paths>"


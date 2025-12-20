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

# Check for restic
function Test-Restic {
    try {
        $null = Get-Command restic -ErrorAction Stop
        Write-ColorOutput Green "✓ restic is already installed"
        restic version
        return $true
    } catch {
        Write-ColorOutput Yellow "⚠ restic is not installed"
        return $false
    }
}

function Install-Restic {
    Write-Output ""
    Write-Output "Attempting to install restic..."
    
    # Try winget first (Windows 10/11)
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        Write-Output "Installing restic via winget..."
        try {
            winget install --id restic.restic --accept-package-agreements --accept-source-agreements
            if ($LASTEXITCODE -eq 0) {
                Write-ColorOutput Green "✓ restic installed successfully"
                return $true
            }
        } catch {
            Write-ColorOutput Red "✗ Failed to install restic via winget"
        }
    }
    
    # Try Chocolatey
    if (Get-Command choco -ErrorAction SilentlyContinue) {
        Write-Output "Installing restic via Chocolatey..."
        try {
            choco install restic -y
            if ($LASTEXITCODE -eq 0) {
                Write-ColorOutput Green "✓ restic installed successfully"
                return $true
            }
        } catch {
            Write-ColorOutput Red "✗ Failed to install restic via Chocolatey"
        }
    }
    
    # Try Scoop
    if (Get-Command scoop -ErrorAction SilentlyContinue) {
        Write-Output "Installing restic via Scoop..."
        try {
            scoop install restic
            if ($LASTEXITCODE -eq 0) {
                Write-ColorOutput Green "✓ restic installed successfully"
                return $true
            }
        } catch {
            Write-ColorOutput Red "✗ Failed to install restic via Scoop"
        }
    }
    
    Write-ColorOutput Yellow "No supported package manager found. Please install restic manually:"
    Write-Output "  winget install restic.restic"
    Write-Output "  Or download from: https://restic.net"
    return $false
}

# Check and install restic
if (-not (Test-Restic)) {
    Write-Output ""
    $response = Read-Host "Would you like to install restic now? (y/N)"
    if ($response -match "^[Yy]$") {
        if (-not (Install-Restic)) {
            Write-Output ""
            Write-ColorOutput Yellow "Please install restic manually before using xentz-agent"
        }
    } else {
        Write-Output ""
        Write-ColorOutput Yellow "Please install restic manually before using xentz-agent:"
        Write-Output "  winget install restic.restic"
        Write-Output "  Or download from: https://restic.net"
    }
    Write-Output ""
}

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
if (-not (Get-Command restic -ErrorAction SilentlyContinue)) {
    Write-Output "  1. Install restic if not already installed"
}
Write-Output "  2. Run: $BinaryName install --repo <your-repo> --password <pwd> --include <paths>"


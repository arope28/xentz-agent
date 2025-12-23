# Supported Platforms and Architectures

## Operating Systems

### ✅ Fully Supported

1. **macOS** (Darwin)
   - Intel (amd64) - All Intel-based Macs
   - Apple Silicon (arm64) - M1, M2, M3, etc.
   - Scheduler: launchd
   - Universal binaries supported (single binary for both architectures)

2. **Windows**
   - amd64 - Standard Windows PCs
   - arm64 - Windows 11 on ARM devices
   - Scheduler: Task Scheduler (via schtasks)

3. **Linux**
   - amd64 - Standard x86_64 Linux systems
   - arm64 - ARM64 Linux (servers, ARM-based devices)
   - armv7 - 32-bit ARM (Raspberry Pi, older ARM devices)
   - Scheduler: systemd (preferred) or cron (fallback)

### 🔄 Optional Support

4. **FreeBSD**
   - amd64 - Included in build script
   - Note: Would need cron or custom scheduler implementation

## Architecture Notes

### macOS: ARM64 vs Intel
- **No code changes needed!** Go handles cross-compilation automatically.
- The same source code works on both architectures.
- Build separate binaries or use a universal binary (created with `lipo` on macOS).
- Universal binaries work on both Intel and Apple Silicon Macs.

### Windows: ARM64 Support
- Windows 11 on ARM can run both ARM64 and amd64 binaries (via emulation).
- ARM64 native binary provides better performance.
- Task Scheduler works identically on both architectures.

### Linux: ARM Variants
- **arm64**: Modern ARM servers, newer Raspberry Pi models (Pi 4+)
- **armv7**: Older Raspberry Pi models (Pi 1-3), embedded devices
- Both use the same Linux scheduler (systemd/cron).

## Build Matrix

| OS      | Architecture | Binary Name                    | Scheduler    |
|---------|--------------|--------------------------------|--------------|
| macOS   | amd64        | xentz-agent-darwin-amd64       | launchd      |
| macOS   | arm64        | xentz-agent-darwin-arm64       | launchd      |
| macOS   | universal    | xentz-agent-darwin-universal    | launchd      |
| Windows | amd64        | xentz-agent-windows-amd64.exe  | Task Scheduler |
| Windows | arm64        | xentz-agent-windows-arm64.exe  | Task Scheduler |
| Linux   | amd64        | xentz-agent-linux-amd64        | systemd/cron |
| Linux   | arm64        | xentz-agent-linux-arm64        | systemd/cron |
| Linux   | armv7        | xentz-agent-linux-armv7        | systemd/cron |
| FreeBSD | amd64        | xentz-agent-freebsd-amd64      | cron         |

## Code Architecture

The codebase is **architecture-agnostic**. All platform-specific logic is isolated in:
- `internal/install/macos_launchd.go` - macOS scheduling
- `internal/install/windows.go` - Windows scheduling  
- `internal/install/linux.go` - Linux scheduling
- `internal/install/install.go` - OS detection and routing

The install logic automatically detects the OS at runtime using `runtime.GOOS` and calls the appropriate installer. No architecture-specific code is needed because:
- Go's standard library handles path separators automatically (`filepath` package)
- All file operations work identically across architectures
- The only difference is the scheduler API (launchd vs Task Scheduler vs systemd)

## Installation Directories

The agent installs to different locations based on the operating system:

- **macOS**: `/usr/local/bin/xentz-agent` (system-wide, requires sudo during installation)
- **Linux**: `~/.local/bin/xentz-agent` (user-specific, XDG standard)
- **Windows**: `%LOCALAPPDATA%\xentz-agent\xentz-agent.exe` (user-specific)

**Note**: On macOS, `/usr/local/bin` is typically already in PATH. On Linux and Windows, you may need to add the installation directory to your PATH.

## Recommendations

### For Distribution
1. **macOS**: Provide both architecture-specific binaries AND a universal binary
2. **Windows**: Provide both amd64 and arm64 (Windows 11 ARM is growing)
3. **Linux**: Provide amd64, arm64, and armv7 (covers most use cases)

### For Users
- **Mac users**: Use the universal binary if available, or the architecture-specific one for your Mac. Installation requires sudo to copy to `/usr/local/bin`.
- **Windows users**: Use amd64 unless you're on Windows 11 ARM
- **Linux users**: Use amd64 for servers/desktops, arm64 for ARM servers, armv7 for Raspberry Pi

## Future Considerations

Potential additions (if needed):
- **OpenBSD**: Similar to FreeBSD, would use cron
- **Solaris/Illumos**: Would need custom scheduler implementation
- **Android**: Would require significant changes (different execution model)

For now, macOS + Windows + Linux covers 99%+ of use cases.


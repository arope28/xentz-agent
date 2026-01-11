# xentz-agent - Cross-Platform Backup Agent

A lightweight backup agent that uses restic to perform scheduled backups on macOS, Windows, and Linux. Features automatic reporting, structured logging, and centralized management via the control plane.

## Supported Platforms

### Operating Systems
- **macOS** (Intel and Apple Silicon)
  - Uses launchd for scheduling
- **Windows** (amd64 and arm64)
  - Uses Task Scheduler for scheduling
- **Linux** (amd64, arm64, and armv7)
  - Uses systemd (preferred) or cron (fallback) for scheduling

### Architectures
- **amd64** (Intel/AMD 64-bit)
- **arm64** (Apple Silicon, Windows on ARM, ARM64 Linux)
- **armv7** (32-bit ARM, e.g., Raspberry Pi)

## Quick Start

### Option 1: Automatic Installer (Recommended)

The installer automatically detects your OS and architecture and downloads the correct binary.

**macOS/Linux:**
```bash
curl -fsSL https://github.com/arope28/xentz-agent/releases/latest/download/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://github.com/arope28/xentz-agent/releases/latest/download/install.ps1 | iex
```

**Or download and run manually:**
- Download `install.sh` (macOS/Linux) or `install.ps1` (Windows)
- Make executable: `chmod +x install.sh`
- Run: `./install.sh` or `.\install.ps1`

### Option 2: Manual Download

1. **Install restic** (required dependency):
   - macOS: `brew install restic`
   - Linux: `sudo apt install restic` or `sudo yum install restic`
   - Windows: Download from [restic.net](https://restic.net)

2. **Download the appropriate binary** for your platform from the [releases page](https://github.com/arope28/xentz-agent/releases).

3. **Install and configure**:
   
   **Token-based enrollment (recommended):**
   ```bash
   ./xentz-agent install --token <install-token> \
     --server https://control-plane.example.com \
     --daily-at 02:00 \
     --include "/Users/yourname/Documents"
   ```
   
   **Legacy mode (direct repository):**
   ```bash
   ./xentz-agent install --repo rest:https://your-repo.com/backup \
     --password "your-password" \
     --daily-at 02:00 \
     --include "/Users/yourname/Documents"
   ```

## Commands

```bash
# Install the agent with token-based enrollment (recommended)
xentz-agent install --token <install-token> --server <control-plane-url> --include <paths>

# Or use legacy mode with direct repository
xentz-agent install --repo <url> --password <pwd> --include <paths>

# Run a backup manually
xentz-agent backup

# Run retention/prune policy
xentz-agent retention

# Check the status of the last backup
xentz-agent status
```

## Features

### Core Functionality
- **Scheduled backups**: Automatic daily backups at configurable times
- **Incremental backups**: Uses restic for efficient incremental backups
- **Retention policies**: Automatic cleanup of old snapshots based on server configuration
- **Multi-user support**: Multiple users can enroll on the same device, each with their own repository

### Management & Monitoring
- **Token-based enrollment**: Secure device enrollment with server-issued credentials
- **Server-driven configuration**: Configuration fetched from control plane on every run
- **Automatic reporting**: Backup and retention metrics automatically sent to control plane
- **Reliable delivery**: Failed reports are spooled locally and retried on subsequent runs
- **Structured logging**: JSON-formatted logs with automatic rotation and shipping to control plane
- **Device kill-switch**: Server can disable devices remotely for security

### Logging
- **Structured JSON logs**: All logs are written in structured JSON format
- **Automatic rotation**: Logs rotate based on size (10MB) and age (7 days)
- **Log compression**: Old rotated logs are automatically compressed
- **Centralized shipping**: Logs are automatically shipped to the control plane for centralized analysis
- **Tagged logs**: All logs include `tenant_id` and `device_id` for correlation
- **Log location**: `~/.xentz-agent/logs/agent.log`

## Building from Source

### Build for All Platforms

**On macOS/Linux:**
```bash
chmod +x build.sh
./build.sh
```

**On Windows:**
```cmd
build.bat
```

This creates executables in the `dist/` directory for all supported platforms and architectures.

### Build for Specific Platform

```bash
# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o xentz-agent ./cmd/xentz-agent

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o xentz-agent ./cmd/xentz-agent

# Windows
GOOS=windows GOARCH=amd64 go build -o xentz-agent.exe ./cmd/xentz-agent

# Linux
GOOS=linux GOARCH=amd64 go build -o xentz-agent ./cmd/xentz-agent
```

## Architecture

### Enrollment Flow
1. **Token-based enrollment**: Agents receive an install token from the control plane
2. **Device registration**: Agent enrolls to get server-assigned `tenant_id`, `device_id`, `device_api_key`, and repository URL
3. **User-scoped repos**: Each user on a device backs up to their own repository path: `{base}/{tenant_id}/{device_id}/{user_id}/`

### Configuration Management
- **Server-driven configuration**: The agent fetches configuration from the control plane on every backup/retention run
- **Local caching**: Config is cached locally and used as fallback if the server is unreachable
- **Dynamic updates**: Configuration changes (backup paths, retention policies) are applied automatically on next run

### Reporting & Logging
- **Automatic reporting**: Backup and retention runs are automatically reported to the control plane with detailed metrics:
  - Files processed, bytes transferred, duration
  - Snapshot IDs, error messages
  - Repository reachability status
- **Reliable delivery**: Failed reports are spooled locally and retried on subsequent runs
- **Structured logging**: All operations are logged in JSON format with automatic rotation
- **Log shipping**: Rotated logs are automatically shipped to the control plane for centralized analysis

### Security
- **Device authentication**: All API calls use device-specific API keys
- **Encrypted passwords**: Restic passwords are encrypted on the server
- **Kill-switch support**: Server can remotely disable devices
- **Sensitive data protection**: Passwords and tokens are automatically redacted from logs

## File Structure

```
~/.xentz-agent/
├── config.json          # Device configuration (tenant_id, device_id, API key)
├── config-cached.json   # Cached server configuration (fallback)
├── restic.pw           # Restic repository password
├── last_run.json        # Last backup run status
├── last_retention_run.json  # Last retention run status
└── logs/
    ├── agent.log        # Current log file
    ├── agent.log.2024-01-15T12-00-00.json  # Rotated logs
    └── agent.log.2024-01-15T12-00-00.json.gz  # Compressed logs
```

## Notes

- **macOS ARM64 vs Intel**: No code changes needed! The same code works on both architectures. Just build separate binaries or use a universal binary (created automatically by `build.sh` on macOS).
- **Linux ARMv7**: Included for compatibility with older ARM devices like Raspberry Pi.
- **Windows on ARM**: Full support for Windows 11 on ARM devices.
- The `install` command automatically detects your OS and uses the appropriate scheduler.
- **Installation directories**:
  - macOS: `/usr/local/bin` (requires sudo during installation)
  - Linux: `~/.local/bin` (user-specific)
  - Windows: `%LOCALAPPDATA%\xentz-agent\` (user-specific)

## API Integration

### Enrollment
The agent calls `POST /v1/install` on the control plane with the install token and device metadata to receive server-issued identifiers (`tenant_id`, `device_id`, `device_api_key`).

### Configuration Fetching
The agent calls `GET /v1/config` on every backup/retention run using the `device_api_key` to fetch the latest configuration.

### Reporting
The agent sends backup and retention metrics to `POST /v1/report` after each run, with automatic retry for failed reports.

### Log Shipping
The agent ships structured logs to `POST /admin/v1/logs` for centralized analysis and troubleshooting.

## Documentation

See the `docs/` directory for detailed documentation:
- `ARCHITECTURE.md` - System architecture overview
- `FLOW.md` - Detailed operation flows
- `INSTALL.md` - Installation instructions
- `PLATFORMS.md` - Platform-specific details
- `SECURITY_REVIEW.md` - Security considerations

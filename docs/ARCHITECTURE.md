# xentz-agent Architecture

This document describes the structure and purpose of each file in the xentz-agent application.

## Project Structure

```
xentz-agent/
├── cmd/xentz-agent/main.go          # Main entry point and CLI
├── internal/
│   ├── backup/                      # Backup operations
│   │   ├── backup.go                # Restic backup execution
│   │   └── retention.go             # Retention and prune policies
│   ├── config/                      # Configuration management
│   │   ├── config.go                # Config struct and file I/O
│   │   └── fetch.go                 # Server config fetching
│   ├── enroll/                      # Device enrollment
│   │   └── enroll.go                # Token-based enrollment logic
│   ├── install/                     # Cross-platform installation
│   │   ├── install.go               # Common installation logic
│   │   ├── linux.go                 # Linux systemd/cron setup
│   │   ├── macos_launchd.go         # macOS launchd setup
│   │   └── windows.go               # Windows Task Scheduler setup
│   ├── report/                      # Monitoring and reporting
│   │   └── report.go                # Report sending with spooling
│   └── state/                       # State management
│       └── state.go                  # Last run status tracking
├── install.sh                       # Shell installer script
├── install.ps1                     # PowerShell installer script
├── install.go                       # Go-based universal installer
├── build.sh                         # Build script for all platforms
└── build.bat                        # Windows build script
```

## File Descriptions

### Entry Point

#### `cmd/xentz-agent/main.go`
**Purpose:** Main entry point for the xentz-agent CLI application.

**Responsibilities:**
- Parses command-line arguments and flags
- Routes commands to appropriate handlers (`install`, `backup`, `retention`, `status`)
- Handles the `install` command with token-based enrollment
- Orchestrates backup and retention operations
- Manages configuration fetching from server with fallback to cached config
- Sends reports after backup/retention runs
- Displays status information

**Key Functions:**
- `usage()` - Displays help text
- Command handlers for each operation
- Integration with all internal packages

### Backup Operations

#### `internal/backup/backup.go`
**Purpose:** Executes restic backup operations.

**Responsibilities:**
- Validates backup configuration (include paths, repository, password file)
- Ensures restic binary is available
- Initializes repository if needed
- Executes `restic backup` with JSON output parsing
- Extracts statistics (files_total, bytes_total, data_added_bytes, snapshot_id)
- Returns `LastRun` state with detailed metrics
- Handles errors and timeouts

**Key Functions:**
- `Run(ctx, cfg)` - Main backup execution
- `ensureRepoInitialized(ctx, cfg)` - Repository initialization check

#### `internal/backup/retention.go`
**Purpose:** Executes restic retention and prune operations.

**Responsibilities:**
- Validates retention policy configuration
- Performs pre-flight connectivity check
- Executes `restic forget` with retention policy
- Executes `restic prune` if configured
- Streams output to both buffer and stdout for user feedback
- Returns `LastRun` state with duration
- Handles long-running operations with appropriate timeouts

**Key Functions:**
- `RunRetention(ctx, cfg)` - Main retention execution
- Pre-flight checks for repository connectivity

### Configuration Management

#### `internal/config/config.go`
**Purpose:** Defines configuration structure and file operations.

**Responsibilities:**
- Defines `Config` struct with all configuration fields
- Manages enrollment data (tenant_id, device_id, device_api_key)
- Handles config file reading and writing
- Manages cached config for fallback scenarios
- Resolves config file paths (default: `~/.xentz-agent/config.json`)

**Key Types:**
- `Config` - Main configuration structure
- `Schedule` - Scheduling configuration
- `Restic` - Restic repository configuration
- `Retention` - Retention policy configuration

**Key Functions:**
- `Load(path)` - Load config from file
- `Save(cfg, path)` - Save config to file
- `WriteCached(cfg)` - Write cached config
- `ReadCached()` - Read cached config

#### `internal/config/fetch.go`
**Purpose:** Fetches configuration from the control plane server.

**Responsibilities:**
- Makes authenticated requests to `GET /v1/config`
- Uses `device_api_key` for authentication
- Implements SSRF protection (blocks localhost, validates schemes)
- Caches successful config fetches
- Handles network errors gracefully
- Validates server URLs

**Key Functions:**
- `FetchFromServer(serverURL, deviceAPIKey)` - Fetch config from server
- `FetchAndCache(serverURL, deviceAPIKey)` - Fetch and cache config
- `LoadWithFallback(serverURL, deviceAPIKey)` - Fetch with cached fallback

### Enrollment

#### `internal/enroll/enroll.go`
**Purpose:** Handles device enrollment with the control plane.

**Responsibilities:**
- Collects device metadata (hostname, OS, architecture)
- Sends enrollment request to `POST /v1/install`
- Uses install token for initial authentication
- Receives server-assigned identifiers (tenant_id, device_id, device_api_key)
- Stores enrollment data in config
- Generates or retrieves user ID

**Key Functions:**
- `Enroll(serverURL, token, userID)` - Main enrollment function
- `GetDeviceMetadata()` - Collect device information
- `GetOrCreateUserID()` - Get or generate user identifier
- `IsEnrolled(cfg)` - Check if device is already enrolled

### Installation

#### `internal/install/install.go`
**Purpose:** Common installation logic shared across platforms.

**Responsibilities:**
- Determines installation directory based on OS
- Handles binary installation
- Provides common utilities for platform-specific installers

#### `internal/install/macos_launchd.go`
**Purpose:** macOS-specific installation using launchd.

**Responsibilities:**
- Creates launchd plist file
- Installs to `/Library/LaunchDaemons/` or `~/Library/LaunchAgents/`
- Schedules backups using launchd
- Handles user vs. system installation

#### `internal/install/linux.go`
**Purpose:** Linux-specific installation using systemd or cron.

**Responsibilities:**
- Creates systemd service file or cron job
- Installs to appropriate system directories
- Handles user vs. system installation
- Supports both systemd and cron scheduling

#### `internal/install/windows.go`
**Purpose:** Windows-specific installation using Task Scheduler.

**Responsibilities:**
- Creates scheduled task using `schtasks`
- Configures task to run as current user
- Handles Windows-specific paths and permissions

### Reporting

#### `internal/report/report.go`
**Purpose:** Sends backup/retention reports to the control plane with spooling.

**Responsibilities:**
- Creates report payloads with metrics
- Sends reports to `POST /v1/report`
- Implements local spooling for failed sends
- Retries spooled reports on next run (oldest first, max 20)
- Cleans up old spooled reports (older than 30 days)
- Truncates error messages to 4096 bytes
- Implements rate limiting and SSRF protection
- Enforces spool directory size limits (100MB)

**Key Functions:**
- `SendReport(report, serverURL, deviceAPIKey)` - Send report immediately
- `SendReportWithSpool(report, serverURL, deviceAPIKey)` - Send with spooling
- `SendPendingReports(serverURL, deviceAPIKey)` - Retry spooled reports
- `CleanupOldReports()` - Remove old spooled reports
- `SpoolReport(report)` - Write report to spool directory

### State Management

#### `internal/state/state.go`
**Purpose:** Tracks last run status for backup and retention operations.

**Responsibilities:**
- Stores last run information (status, time, duration, metrics)
- Persists state to JSON files
- Provides state for `status` command
- Tracks both backup and retention runs separately

**Key Types:**
- `LastRun` - Last run information structure

**Key Functions:**
- `SaveLastRun(run, jobType)` - Save last run state
- `LoadLastRun(jobType)` - Load last run state
- `NewLastRunSuccessWithStats(...)` - Create success state with metrics
- `NewLastRunError(...)` - Create error state

### Installer Scripts

#### `install.sh`
**Purpose:** Shell-based installer for macOS and Linux.

**Responsibilities:**
- Detects OS and architecture
- Downloads appropriate binary from GitHub releases
- Checks for `restic` prerequisite
- Installs binary to appropriate directory (`/usr/local/bin` on macOS, `~/.local/bin` on Linux)
- Sets up scheduled tasks

#### `install.ps1`
**Purpose:** PowerShell-based installer for Windows.

**Responsibilities:**
- Detects Windows architecture
- Downloads appropriate binary from GitHub releases
- Checks for `restic` prerequisite
- Installs binary to `%LOCALAPPDATA%\xentz-agent`
- Sets up Task Scheduler

#### `install.go`
**Purpose:** Go-based universal installer.

**Responsibilities:**
- Cross-platform installer written in Go
- Detects OS and architecture
- Downloads binary from GitHub releases
- Handles prerequisites
- Installs to appropriate directories

### Build Scripts

#### `build.sh`
**Purpose:** Builds executables for all supported platforms.

**Responsibilities:**
- Cross-compiles for macOS (amd64, arm64, universal)
- Cross-compiles for Windows (amd64, arm64)
- Cross-compiles for Linux (amd64, arm64, armv7)
- Cross-compiles for FreeBSD (amd64)
- Outputs binaries to `dist/` directory

#### `build.bat`
**Purpose:** Windows batch script for building executables.

**Responsibilities:**
- Similar to `build.sh` but for Windows environment
- Uses Go cross-compilation

## Data Flow

1. **Installation:** User runs installer → Enrollment → Config stored → Scheduled task created
2. **Backup:** Scheduler triggers → Fetch config → Run backup → Send report
3. **Retention:** Scheduler triggers → Fetch config → Run retention → Send report
4. **Status:** User queries → Load state → Display last run information

## Configuration Storage

- **Config file:** `~/.xentz-agent/config.json`
- **Cached config:** `~/.xentz-agent/config.cached.json`
- **State files:** `~/.xentz-agent/state/backup.json`, `~/.xentz-agent/state/retention.json`
- **Spool directory:** `~/.xentz-agent/spool/`
- **Password file:** `~/.xentz-agent/restic.pw` (default)

## Security Considerations

- SSRF protection in config fetching and reporting
- Input validation for all user-provided data
- Error message sanitization (truncated to 4096 bytes)
- Filename sanitization for spooled reports
- Rate limiting for report sending
- Spool directory size limits
- Secure file permissions


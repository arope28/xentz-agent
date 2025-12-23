# xentz-agent Flow Diagrams

This document describes the flow of the xentz-agent application and its communication with the control plane server.

## Overview

The xentz-agent is a backup agent that:
1. Enrolls with a control plane server using an install token
2. Fetches configuration from the server on each run
3. Executes backups using restic
4. Applies retention policies
5. Reports results back to the server

## Enrollment Flow

```
┌─────────────┐
│   User      │
└──────┬──────┘
       │
       │ 1. xentz-agent install --token <token> --server <url>
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (install command)       │
│  - Collects device metadata          │
│  - Reads install token               │
└──────┬───────────────────────────────┘
       │
       │ 2. POST /v1/install
       │    Authorization: Bearer <token>
       │    Body: { user_id, metadata: { hostname, os, arch } }
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
│  - Validates install token           │
│  - Generates tenant_id, device_id    │
│  - Generates device_api_key           │
│  - Creates repository path           │
│  - Returns enrollment data           │
└──────┬───────────────────────────────┘
       │
       │ 3. Response: { tenant_id, device_id, device_api_key, repo_path, password }
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Stores enrollment data in config  │
│  - Saves config.json                 │
│  - Creates scheduled task            │
└─────────────────────────────────────┘
```

## Backup Flow

```
┌─────────────────────────────────────┐
│  Scheduler (launchd/systemd/cron)   │
│  Triggers at configured time         │
└──────┬───────────────────────────────┘
       │
       │ 1. Executes: xentz-agent backup
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (backup command)        │
│  - Loads local config                │
└──────┬───────────────────────────────┘
       │
       │ 2. GET /v1/config
       │    Authorization: Bearer <device_api_key>
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
│  - Validates device_api_key          │
│  - Returns current config            │
└──────┬───────────────────────────────┘
       │
       │ 3a. Success: Config JSON
       │ 3b. Failure: Use cached config (with warning)
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Validates config                  │
│  - Caches successful fetch           │
└──────┬───────────────────────────────┘
       │
       │ 4. Send pending reports (if any)
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (report.SendPending)    │
│  - Loads spooled reports             │
│  - Sends oldest first (max 20)        │
└──────┬───────────────────────────────┘
       │
       │ 5. POST /v1/report (for each pending)
       │    Authorization: Bearer <device_api_key>
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
│  - Stores reports                    │
└─────────────────────────────────────┘
       │
       │ 6. Execute backup
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (backup.Run)            │
│  - Validates include paths           │
│  - Checks restic availability         │
│  - Ensures repository initialized   │
│  - Executes: restic backup --json    │
└──────┬───────────────────────────────┘
       │
       │ 7. restic backup
       │    RESTIC_REPOSITORY=<repo>
       │    RESTIC_PASSWORD_FILE=<file>
       ▼
┌─────────────────────────────────────┐
│  Restic Repository (via REST API)    │
│  - Stores backup data                │
│  - Returns JSON stats                │
└──────┬───────────────────────────────┘
       │
       │ 8. Parse JSON output
       │    Extract: files_total, bytes_total,
       │             data_added_bytes, snapshot_id
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Saves backup state                │
│  - Creates report payload           │
└──────┬───────────────────────────────┘
       │
       │ 9. POST /v1/report
       │    Authorization: Bearer <device_api_key>
       │    Body: { device_id, job: "backup", status, metrics, ... }
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
│  - Stores report                     │
└─────────────────────────────────────┘
       │
       │ 10a. Success: Report stored
       │ 10b. Failure: Spool report for retry
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - If send failed: Write to spool   │
│  - Cleanup old spooled reports       │
│  - Exit                             │
└─────────────────────────────────────┘
```

## Retention Flow

```
┌─────────────────────────────────────┐
│  Scheduler (launchd/systemd/cron)   │
│  Triggers at configured time         │
└──────┬───────────────────────────────┘
       │
       │ 1. Executes: xentz-agent retention
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (retention command)     │
│  - Loads local config                │
└──────┬───────────────────────────────┘
       │
       │ 2. GET /v1/config
       │    Authorization: Bearer <device_api_key>
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
│  - Returns current config            │
└──────┬───────────────────────────────┘
       │
       │ 3. Config with retention policy
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Validates retention policy        │
│  - Pre-flight connectivity check     │
└──────┬───────────────────────────────┘
       │
       │ 4. restic snapshots --last 1
       │    (connectivity check)
       ▼
┌─────────────────────────────────────┐
│  Restic Repository                   │
│  - Returns snapshot info             │
└──────┬───────────────────────────────┘
       │
       │ 5. Execute retention
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (retention.RunRetention)│
│  - Executes: restic forget           │
│    --keep-daily, --keep-weekly, etc.│
│  - Executes: restic prune (if set)  │
└──────┬───────────────────────────────┘
       │
       │ 6. restic forget/prune
       ▼
┌─────────────────────────────────────┐
│  Restic Repository                   │
│  - Removes old snapshots             │
│  - Prunes unreferenced data          │
└──────┬───────────────────────────────┘
       │
       │ 7. Save retention state
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Creates report payload            │
│  - Sends report (with spooling)     │
│  - Cleanup old spooled reports       │
└─────────────────────────────────────┘
```

## Configuration Fetch Flow (with Fallback)

```
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  LoadWithFallback()                  │
└──────┬───────────────────────────────┘
       │
       │ 1. Attempt: GET /v1/config
       │    Authorization: Bearer <device_api_key>
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
└──────┬───────────────────────────────┘
       │
       │ 2a. Success (200 OK)
       │     └─► Cache config
       │     └─► Return config
       │
       │ 2b. Failure (network error, 401, 500, etc.)
       │     └─► Log warning
       │     └─► Attempt: Read cached config
       │         ├─► Success: Return cached config
       │         └─► Failure: Return error (no config available)
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Uses config (fresh or cached)    │
│  - Proceeds with operation           │
└─────────────────────────────────────┘
```

## Report Spooling Flow

```
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  SendReportWithSpool()               │
└──────┬───────────────────────────────┘
       │
       │ 1. Attempt: POST /v1/report
       │    Authorization: Bearer <device_api_key>
       ▼
┌─────────────────────────────────────┐
│  Control Plane Server                │
└──────┬───────────────────────────────┘
       │
       │ 2a. Success (200 OK)
       │     └─► Report sent, done
       │
       │ 2b. Failure (network error, 401, 500, etc.)
       │     └─► Write report to spool directory
       │     └─► ~/.xentz-agent/spool/<timestamp>-<job>-<status>.json
       ▼
┌─────────────────────────────────────┐
│  Local Spool Directory                │
│  - Stores failed reports              │
│  - Max 20 reports                    │
│  - Oldest first retry                │
└─────────────────────────────────────┘
       │
       │ 3. On next run (backup or retention)
       │    └─► SendPendingReports()
       │        └─► Load spooled reports (oldest first, max 20)
       │        └─► Attempt to send each
       │        └─► Delete on success
       │        └─► Keep on failure (retry next time)
       │
       │ 4. CleanupOldReports()
       │    └─► Delete reports older than 30 days
       │    └─► Enforce 100MB spool directory limit
```

## Status Flow

```
┌─────────────┐
│   User      │
└──────┬──────┘
       │
       │ 1. xentz-agent status
       ▼
┌─────────────────────────────────────┐
│  xentz-agent (status command)        │
│  - Loads backup state                │
│  - Loads retention state             │
└──────┬───────────────────────────────┘
       │
       │ 2. Read state files
       │    ~/.xentz-agent/state/backup.json
       │    ~/.xentz-agent/state/retention.json
       ▼
┌─────────────────────────────────────┐
│  xentz-agent                         │
│  - Formats and displays status       │
│  - Shows last run time, duration     │
│  - Shows metrics (files, bytes)     │
│  - Shows error messages (if any)     │
└─────────────────────────────────────┘
```

## Communication Protocol

### Authentication

All API requests use Bearer token authentication:
```
Authorization: Bearer <device_api_key>
```

### Endpoints

1. **POST /v1/install** (Enrollment)
   - Request: `{ user_id?, metadata: { hostname, os, arch } }`
   - Response: `{ tenant_id, device_id, device_api_key, repo_path, password? }`
   - Auth: Install token (one-time)

2. **GET /v1/config** (Config Fetch)
   - Request: None (auth via header)
   - Response: `{ schedule, include, exclude, restic, retention }`
   - Auth: Device API key

3. **POST /v1/report** (Reporting)
   - Request: `{ device_id, job, started_at, finished_at, status, duration_ms, metrics, error? }`
   - Response: `200 OK` or error
   - Auth: Device API key

### Repository Access

The agent uses restic with REST API backend:
- Repository URL format: `rest:http://user:password@host:port/path`
- Authentication: HTTP Basic Auth (username:password in URL)
- The control plane provides the full repository URL during enrollment

## Error Handling

1. **Config Fetch Failure:**
   - Log warning
   - Use cached config if available
   - Fail gracefully if no cached config

2. **Report Send Failure:**
   - Spool report locally
   - Retry on next run
   - Don't fail the backup/retention operation

3. **Backup/Retention Failure:**
   - Save error state
   - Send failure report (with spooling)
   - Return error to scheduler

## State Persistence

- **Config:** `~/.xentz-agent/config.json`
- **Cached Config:** `~/.xentz-agent/config.cached.json`
- **Backup State:** `~/.xentz-agent/state/backup.json`
- **Retention State:** `~/.xentz-agent/state/retention.json`
- **Spooled Reports:** `~/.xentz-agent/spool/*.json`


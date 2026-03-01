# Testing Restore (Previous Version of a File)

The agent can list snapshots, find files, browse backup contents, check repository integrity, and restore files using the same repository and password as backup—no need to set `RESTIC_*` env vars yourself.

## Using the agent (recommended)

Ensure **restic** is installed and on your PATH (same as for backup).

### List snapshots

```bash
xentz-agent restore snapshots
```

Shows all backup points. Note the **snapshot ID** (e.g. `abc12345`) for the version you want.

### Find which snapshots contain a file

```bash
xentz-agent restore find /path/to/file
```

Lists every snapshot that contains the given path (or path pattern). Use this to see which backup(s) have the file before restoring.

### List files in a snapshot

```bash
xentz-agent restore ls <snapshot_id>
xentz-agent restore ls latest
xentz-agent restore ls <snapshot_id> /path/inside/snapshot
```

Browse the contents of a snapshot. Use `latest` for the most recent backup. Optionally give a path to list only that directory.

### Repository statistics

```bash
xentz-agent restore stats
xentz-agent restore stats <snapshot_id>
```

Shows size statistics (e.g. restore size, unique data). Without a snapshot ID, stats are for the whole repository; with an ID, for that snapshot only.

### Verify repository integrity

```bash
xentz-agent restore check
```

Checks the repository for errors (data integrity, index). Use periodically or if you suspect backup corruption.

### Restore files into a directory

Restore a full snapshot or only specific paths into a target directory:

```bash
# Restore entire snapshot into /tmp/restore-test
xentz-agent restore <snapshot_id> --target /tmp/restore-test

# Restore only one file or path (use absolute path as at backup time)
xentz-agent restore <snapshot_id> --target /tmp/restore-test --path /Users/me/Documents/notes.txt
```

Paths under the snapshot are recreated under the target (e.g. file appears at `/tmp/restore-test/Users/me/Documents/notes.txt`).

### Dump one file to stdout or a file

For a single file, dump its contents:

```bash
# To stdout
xentz-agent restore dump <snapshot_id> /full/path/to/file

# To a file
xentz-agent restore dump <snapshot_id> /full/path/to/file --output ./restored-file
```

Use the **absolute path** of the file as it was when the backup ran.

### Config override

Use the same config as backup by default. To override:

```bash
xentz-agent restore --config /path/to/config.json snapshots
```

## Quick test workflow

1. Create or change a file under one of your backup include paths.
2. Run a backup: `xentz-agent backup --auto-init` (or wait for the scheduled run).
3. Change or delete the file.
4. List snapshots: `xentz-agent restore snapshots ls`.
5. Restore the file from the snapshot you took in step 2 (e.g. `restore dump <id> /path/to/file --output ./restored`).
6. Confirm the restored content matches the version from before step 3.

---

## Alternative: using restic directly

If you prefer to use restic yourself (e.g. from another machine), use the same repository and password as the agent.

### Prerequisites

- **restic** on PATH.
- **Repository:** `config.json` → `restic.repository` (see [paths](INSTALL.md#paths)).
- **Password:** If `restic.password_file` is set, use that file; otherwise retrieve from the control plane (device → Backup Principals) or create a password file from the portal.

### Set environment variables

```bash
export RESTIC_REPOSITORY="rest:https://your-server/restic/..."
export RESTIC_PASSWORD_FILE="$HOME/.config/xentz-agent/restic.pw"   # or path from config
# Or: export RESTIC_PASSWORD="your-restic-password"
```

### Commands

```bash
restic snapshots
restic find /path/to/file
restic ls <snapshot_id> [path]
restic stats [snapshot_id]
restic check
restic restore <snapshot_id> --target /tmp/restore-test [--include /path/to/file]
restic dump <snapshot_id> /path/to/file > /tmp/restored-file
```

Path in `--include` or `dump` must be the **absolute path as on the machine when the backup ran**.

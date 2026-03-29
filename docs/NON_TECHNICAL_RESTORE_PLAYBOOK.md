# Non-technical restore playbook (support handoff)

This script is for support to hand directly to end users in **user mode**. It keeps the flow copy/paste-first and avoids restic internals.

Related:
- Advanced operator commands: [RESTORE.md](RESTORE.md)
- Client-facing recovery overview: [CLIENT_RECOVERY_GUIDE.md](CLIENT_RECOVERY_GUIDE.md)
- Mode/permission checks: [MACOS_FULL_DISK_ACCESS_CHECKLIST.md](MACOS_FULL_DISK_ACCESS_CHECKLIST.md)

---

## 0) Before you start (support checklist)

Confirm with the user:

1. They are on the enrolled machine/profile (or one with access to the same repository).
2. `xentz-agent` and `restic` are installed.
3. They can run:

```bash
xentz-agent status
```

If this fails, run:

```bash
xentz-agent doctor --mode user --check-server
```

and resolve enrollment/auth first.

---

## 1) Fast path: restore latest version of one file

Ask user for the full original file path (example):

`/Users/alex/Documents/Quarterly/report.xlsx`

Then have them run:

```bash
mkdir -p "$HOME/Desktop/xentz-restore"
xentz-agent restore dump latest "/Users/alex/Documents/Quarterly/report.xlsx" --output "$HOME/Desktop/xentz-restore/report.xlsx"
```

Expected result: file appears at `~/Desktop/xentz-restore/report.xlsx`.

If user wants the file back in the original location, ask them to verify the restored file first, then manually copy it over.

---

## 2) Restore from a specific date/time (single file)

1. Find snapshots:

```bash
xentz-agent restore snapshots
```

2. Pick the snapshot near the desired date/time and run:

```bash
mkdir -p "$HOME/Desktop/xentz-restore"
xentz-agent restore dump <SNAPSHOT_ID> "/Users/alex/Documents/Quarterly/report.xlsx" --output "$HOME/Desktop/xentz-restore/report.xlsx"
```

---

## 3) Restore an entire folder to Desktop (safe destination)

```bash
mkdir -p "$HOME/Desktop/xentz-restore-folder"
xentz-agent restore latest --target "$HOME/Desktop/xentz-restore-folder" --path "/Users/alex/Documents/ProjectA"
```

Note: restored paths keep original structure under target. Example output path:

`~/Desktop/xentz-restore-folder/Users/alex/Documents/ProjectA/...`

---

## 4) Restore a full snapshot to alternate location (disaster drill)

```bash
mkdir -p "$HOME/Desktop/xentz-full-restore"
xentz-agent restore <SNAPSHOT_ID> --target "$HOME/Desktop/xentz-full-restore"
```

Use this for validation/drills, not direct overwrite.

---

## 5) Common error handling (support responses)

### A) `operation not permitted` / `fileprovider.detached` (macOS)

Use:
- [MACOS_FULL_DISK_ACCESS_CHECKLIST.md](MACOS_FULL_DISK_ACCESS_CHECKLIST.md)

In short: grant Full Disk Access to terminal + `xentz-agent` + `restic`, then retry.

### B) `invalid or revoked device API key`

User likely has stale enrollment. Use:

```bash
xentz-agent doctor --mode user --check-server
```

Then re-enroll (support provides new token):

```bash
xentz-agent install --mode user --force --token "<NEW_TOKEN>" --server "https://<control-host>" --include "<path>"
```

### C) “No such file in snapshot”

Have user run:

```bash
xentz-agent restore find "/Users/alex/Documents/Quarterly/report.xlsx"
```

Then retry with one of the listed snapshot IDs.

---

## 6) Support closeout checklist

Before closing ticket:

1. User confirms restored file/folder opens correctly.
2. User confirms destination path.
3. Support notes snapshot ID used and timestamp.
4. If drill: record evidence per org process.

---

## Quick copy block (single-file latest)

```bash
mkdir -p "$HOME/Desktop/xentz-restore"
xentz-agent restore dump latest "<FULL_ORIGINAL_PATH>" --output "$HOME/Desktop/xentz-restore/<FILENAME>"
```

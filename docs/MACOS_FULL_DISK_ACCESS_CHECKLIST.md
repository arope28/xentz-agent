# macOS Full Disk Access checklist

Use this guide when backups fail with:

- `operation not permitted`
- `com.apple.fileprovider.detached`
- restic exit code `3` with unreadable files under `~/Documents`, Desktop, or iCloud/File Provider paths

## 1) Identify mode first

Run:

```bash
xentz-agent doctor --mode user
sudo xentz-agent doctor --mode system
```

- If user enrollment is active, follow **User mode**.
- If system enrollment is active, follow **System mode**.

## 2) User mode (recommended default)

Open **System Settings -> Privacy & Security -> Full Disk Access** and add/enable:

1. Your terminal app (Terminal, iTerm, Warp, etc.) for manual runs
2. `xentz-agent` binary (`which xentz-agent`)
3. `restic` binary (`which restic`)

Then validate:

```bash
xentz-agent backup
```

If it still fails, reboot once and retry (macOS privacy/TCC updates can lag).

## 3) System mode (managed / IT)

System mode runs in elevated/daemon context and is stricter for protected folders.

Add/enable Full Disk Access for:

1. `xentz-agent` binary used by the service/daemon
2. `restic` binary used by the service/daemon

Then validate in matching context:

```bash
sudo xentz-agent backup
```

## 4) iCloud / File Provider specifics

If files are cloud placeholders or detached items:

1. Ensure files are downloaded locally before backup
2. Keep FDA enabled for `xentz-agent` and `restic`
3. Prefer narrower include paths vs broad `~/Documents` when possible

## 5) Keep mode and execution context aligned

- **User mode install** -> run without `sudo`
- **System mode install** -> run with `sudo`

Mixing contexts causes confusing auth/permission behavior.

## 6) Quick support commands

```bash
xentz-agent doctor --mode user --check-server
sudo xentz-agent doctor --mode system --check-server
```

If one-off failure is followed by success, document it as a transient permission propagation case and continue monitoring scheduled runs.

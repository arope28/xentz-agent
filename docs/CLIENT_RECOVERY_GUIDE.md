# Client-facing backup recovery guide (v1)

This guide is for end users and support staff. Operator detail: [RESTORE.md](RESTORE.md) (agent CLI). Recovery **tokens** (re-link enrollment): control plane `docs/RECOVERY.md`.
Non-technical support handoff: [NON_TECHNICAL_RESTORE_PLAYBOOK.md](NON_TECHNICAL_RESTORE_PLAYBOOK.md).

## Two different “recovery” concepts

| Situation | What to use | Who runs it |
|-----------|-------------|-------------|
| You need **files back** from a backup (deleted file, disk issue, new machine) | Agent **`restore`** commands (restic-based) | Support-guided; often on the user’s PC with agent installed |
| The agent **lost its enrollment** (new OS install, config wiped) but backups still exist | Portal **recovery token** + agent **`recover`** | Admin generates token; user or support runs one command |

Do not confuse them: **restore** = data from restic repository; **recover** = re-link device identity to the control plane.

## Data restore (files and folders)

Requirements: `xentz-agent` installed, enrolled, `restic` on PATH, machine can reach backup repository.

Fastest option for end users:
`xentz-agent restore guided`

1. List snapshots:  
   `xentz-agent restore snapshots`
2. Restore a single file to a safe folder (example):  
   `xentz-agent restore dump <snapshot_id> /full/path/inside/backup --output ./restored-file`
3. Restore a folder or full snapshot:  
   `xentz-agent restore <snapshot_id> --target /path/to/empty-folder`  
   Optionally limit paths with `--path` (see [RESTORE.md](RESTORE.md)).

For copy/paste-ready end-user scripts (latest file, folder to Desktop, alternate location restore), use:
[NON_TECHNICAL_RESTORE_PLAYBOOK.md](NON_TECHNICAL_RESTORE_PLAYBOOK.md).

**Security:** Restores use the same encrypted repository as backups. Do not share repository passwords or recovery tokens in chat or email; use a secure channel.

## Re-enrollment (recovery token)

If the user reinstalls Windows/macOS or deletes agent config but backups should continue under the same identity:

1. Admin: control plane portal → device → backup principal → **Generate recovery token** (one-time, short-lived).
2. User: run exactly:  
   `xentz-agent recover --server https://<your-control-plane> --recovery-token <paste-once>`  
3. Old principal API key is rotated; previous key stops working.

Details: control plane `docs/RECOVERY.md` (recovery token flow).

## Recording drills (operators/admin)

For release sign-off, record successful drills in the go-live checklist API:

- `GET /admin/v1/go-live/` and `PUT /admin/v1/go-live/`  
- See `docs/REPORTING_WORKFLOWS.md` in the control plane repository.

Include at least one **file-level** and one **folder or full snapshot** restore in evidence per control plane `docs/GO_NO_GO.md`.

## Escalation

If `restore` or `recover` fails: collect `xentz-agent diagnostics --out /tmp/diag.zip` (paths may vary by OS) and contact support with device id and timeframe; do **not** attach secrets.

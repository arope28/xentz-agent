# Local UI restore MVP (phase 3)

This defines the first local restore workflow contract for `xentz-agent local-ui`.

## Goals

- Reuse the same restore backend path as CLI (`restic dump` / `restic restore`).
- Keep localhost-only + local token model.
- Provide a simple 3-step API flow for UI:
  1) list snapshots
  2) preview/validate restore request
  3) run restore

## Security model

- Endpoints are served only by `local-ui` on local bind address.
- Same auth as existing local UI endpoints:
  - `X-Local-Token` header, or
  - `?token=` query param for browser navigation.
- No control-plane-facing restore endpoint in this MVP.

## Endpoints

### `GET /restore/snapshots`

Returns restic snapshots JSON (pass-through parsed JSON array).

Example:

```json
[
  {
    "id": "7f7a1b6f...",
    "short_id": "7f7a1b6f",
    "time": "2026-03-29T15:20:03.123456789Z"
  }
]
```

### `POST /restore/plan`

Validates request and reports whether overwrite confirmation is required.

Request:

```json
{
  "type": "file",
  "snapshot_id": "latest",
  "path": "/Users/alice/Documents/file.txt",
  "target": "/Users/alice/Desktop/xentz-restore/file.txt"
}
```

Response:

```json
{
  "ok": true,
  "confirm_required": false,
  "errors": [],
  "request": {
    "type": "file",
    "snapshot_id": "latest",
    "path": "/Users/alice/Documents/file.txt",
    "target": "/Users/alice/Desktop/xentz-restore/file.txt"
  }
}
```

Validation rules:

- `type` must be `file`, `folder`, or `snapshot`.
- `target` is required.
- `target` must be an absolute path.
- `path` is required for `file` and `folder`.
- Dangerous targets are rejected: `/`, `/System`, `/Library`, `/usr`.
- `confirm_required=true` when target exists and could be overwritten/merged.

### `POST /restore/run`

Runs restore synchronously.

Request:

```json
{
  "type": "folder",
  "snapshot_id": "latest",
  "path": "/Users/alice/Documents/project",
  "target": "/Users/alice/Desktop/xentz-restore",
  "confirm_overwrite": true
}
```

Response:

```json
{
  "ok": true,
  "type": "folder",
  "snapshot_id": "latest",
  "path": "/Users/alice/Documents/project",
  "target": "/Users/alice/Desktop/xentz-restore",
  "duration_ms": 512,
  "open_hint": "open \"/Users/alice/Desktop/xentz-restore\""
}
```

Error behavior:

- `400` for invalid request payload.
- `409` when `confirm_required=true` but `confirm_overwrite` is not set.
- `500` for restic/config/runtime failures.
- Restore execution uses a bounded timeout (currently 20 minutes).

## Frontend sequence (proposed)

1. Call `GET /restore/snapshots`.
2. User selects restore type + snapshot + path + target.
3. Call `POST /restore/plan`.
4. If `confirm_required`, show warning and require explicit confirmation.
5. Call `POST /restore/run`.
6. Show restored target path + copyable open command.

## Not in MVP

- Async job queue/progress streaming.
- File-tree browsing inside snapshots.
- Cancel running restore.
- Full browser wizard UI (API is ready first).

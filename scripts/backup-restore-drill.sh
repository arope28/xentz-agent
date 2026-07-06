#!/usr/bin/env bash
# Run an end-to-end fixture backup/restore drill on an enrolled agent.
#
# The drill creates a unique file, runs a backup, restores that file from the
# latest snapshot, and byte-compares the result.
#
# Usage:
#   scripts/backup-restore-drill.sh --fixture-dir /path/already/included
#   scripts/backup-restore-drill.sh --fixture-dir /tmp/xentz-drill --update-config

set -euo pipefail

AGENT_BIN="${AGENT_BIN:-xentz-agent}"
FIXTURE_DIR=""
CONFIG_PATH=""
UPDATE_CONFIG=false
RUN_RESTIC_CHECK=false
KEEP_FIXTURE=false
FIXTURE_FILE=""
RESTORE_DIR=""

usage() {
  cat >&2 <<'EOF'
usage: backup-restore-drill.sh --fixture-dir DIR [options]

Options:
  --agent-bin PATH       Agent binary to use (default: AGENT_BIN or xentz-agent)
  --config PATH          Agent config path override
  --update-config        Add fixture dir to agent include paths before backup
  --restic-check         Run xentz-agent restore check after byte-compare
  --keep-fixture         Do not delete the fixture dir after the drill
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --agent-bin)
      AGENT_BIN="${2:-}"
      shift 2
      ;;
    --config)
      CONFIG_PATH="${2:-}"
      shift 2
      ;;
    --fixture-dir)
      FIXTURE_DIR="${2:-}"
      shift 2
      ;;
    --update-config)
      UPDATE_CONFIG=true
      shift
      ;;
    --restic-check)
      RUN_RESTIC_CHECK=true
      shift
      ;;
    --keep-fixture)
      KEEP_FIXTURE=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "$FIXTURE_DIR" ]]; then
  echo "missing --fixture-dir" >&2
  usage
  exit 2
fi

if [[ ! -x "$AGENT_BIN" ]] && ! command -v "$AGENT_BIN" >/dev/null 2>&1; then
  echo "agent binary not found or not executable: $AGENT_BIN" >&2
  exit 1
fi

CONFIG_ARGS=()
if [[ -n "$CONFIG_PATH" ]]; then
  CONFIG_ARGS=(--config "$CONFIG_PATH")
fi

mkdir -p "$FIXTURE_DIR"
FIXTURE_DIR="$(cd "$FIXTURE_DIR" && pwd)"

cleanup() {
  if [[ "$KEEP_FIXTURE" != true && -n "$FIXTURE_FILE" ]]; then
    rm -f "$FIXTURE_FILE"
  fi
  if [[ -n "$RESTORE_DIR" ]]; then
    rm -rf "$RESTORE_DIR"
  fi
}
trap cleanup EXIT

if [[ "$UPDATE_CONFIG" == true ]]; then
  "$AGENT_BIN" config "${CONFIG_ARGS[@]}" --add-include "$FIXTURE_DIR"
fi

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FIXTURE_FILE="$FIXTURE_DIR/xentz-restore-drill-$STAMP.txt"
RESTORE_DIR="$(mktemp -d)"
RESTORED_FILE="$RESTORE_DIR/restored-fixture.txt"

cat > "$FIXTURE_FILE" <<EOF
xentz restore drill
timestamp=$STAMP
host=$(hostname)
random=$(openssl rand -hex 16 2>/dev/null || date +%s%N)
EOF

echo "Created fixture: $FIXTURE_FILE"
echo "Running backup..."
"$AGENT_BIN" backup "${CONFIG_ARGS[@]}" --auto-init

echo "Restoring fixture from latest snapshot..."
"$AGENT_BIN" restore "${CONFIG_ARGS[@]}" dump latest "$FIXTURE_FILE" --output "$RESTORED_FILE"

if ! cmp -s "$FIXTURE_FILE" "$RESTORED_FILE"; then
  echo "restore drill failed: restored file differs" >&2
  echo "fixture:  $FIXTURE_FILE" >&2
  echo "restored: $RESTORED_FILE" >&2
  exit 1
fi

if [[ "$RUN_RESTIC_CHECK" == true ]]; then
  echo "Running repository check..."
  "$AGENT_BIN" restore "${CONFIG_ARGS[@]}" check
fi

echo "Backup/restore drill passed"
echo "fixture=$FIXTURE_FILE"
echo "restored=$RESTORED_FILE"

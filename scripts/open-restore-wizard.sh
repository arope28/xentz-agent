#!/usr/bin/env bash
set -euo pipefail

# Opens xentz-agent restore wizard in browser.
# - Starts local-ui if not already running.
# - Reads token from local-ui.token.
# - Opens /restore with token query.
#
# Usage:
#   scripts/open-restore-wizard.sh [--mode user|system] [--addr 127.0.0.1:9800]

MODE="user"
ADDR="127.0.0.1:9800"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      if [[ $# -lt 2 ]]; then
        echo "missing value for --mode" >&2
        exit 2
      fi
      MODE="${2:-}"
      shift 2
      ;;
    --addr)
      if [[ $# -lt 2 ]]; then
        echo "missing value for --addr" >&2
        exit 2
      fi
      ADDR="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      echo "usage: $0 [--mode user|system] [--addr 127.0.0.1:9800]" >&2
      exit 2
      ;;
  esac
done

if [[ "${MODE}" != "user" && "${MODE}" != "system" ]]; then
  echo "invalid --mode: ${MODE} (expected user|system)" >&2
  exit 2
fi

# Prefer local repo binary when present so launcher and source stay in sync.
if [[ -x "./xentz-agent" ]]; then
  AGENT_BIN="./xentz-agent"
elif command -v xentz-agent >/dev/null 2>&1; then
  AGENT_BIN="xentz-agent"
else
  echo "xentz-agent not found (neither ./xentz-agent nor PATH)" >&2
  echo "build it first: go build -o xentz-agent ./cmd/xentz-agent" >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found in PATH" >&2
  exit 1
fi

OS="$(uname -s)"
case "${OS}" in
  Darwin)
    if [[ "${MODE}" == "system" ]]; then
      CONFIG_DIR="/Library/Application Support/XentzAgent/config"
    else
      CONFIG_DIR="${HOME}/Library/Application Support/XentzAgent/config"
    fi
    OPEN_CMD="open"
    ;;
  Linux)
    if [[ "${MODE}" == "system" ]]; then
      CONFIG_DIR="/etc/xentz-agent"
    else
      CONFIG_DIR="${HOME}/.config/xentz-agent"
    fi
    OPEN_CMD="xdg-open"
    ;;
  *)
    echo "unsupported OS for this launcher: ${OS}" >&2
    exit 1
    ;;
esac
if ! command -v "${OPEN_CMD}" >/dev/null 2>&1; then
  echo "${OPEN_CMD} not found in PATH" >&2
  exit 1
fi

# Strictly parse and validate --addr to prevent URL authority injection.
if [[ "${ADDR}" =~ ^(localhost|127\.0\.0\.1):([0-9]{1,5})$ ]]; then
  ADDR_HOST="${BASH_REMATCH[1]}"
  ADDR_PORT="${BASH_REMATCH[2]}"
elif [[ "${ADDR}" =~ ^\[(::1)\]:([0-9]{1,5})$ ]]; then
  ADDR_HOST="::1"
  ADDR_PORT="${BASH_REMATCH[2]}"
else
  echo "invalid --addr format: ${ADDR}" >&2
  echo "expected one of: localhost:9800, 127.0.0.1:9800, [::1]:9800" >&2
  exit 2
fi
if (( ADDR_PORT < 1 || ADDR_PORT > 65535 )); then
  echo "invalid --addr port: ${ADDR_PORT}" >&2
  exit 2
fi

if [[ "${MODE}" == "system" && "${EUID}" -ne 0 ]]; then
  echo "--mode system requires elevated context." >&2
  echo "re-run with: sudo $0 --mode system --addr ${ADDR}" >&2
  exit 1
fi

TOKEN_FILE="${CONFIG_DIR}/local-ui.token"
if [[ "${ADDR_HOST}" == "::1" ]]; then
  LOCAL_UI_URL="http://[::1]:${ADDR_PORT}"
else
  LOCAL_UI_URL="http://${ADDR_HOST}:${ADDR_PORT}"
fi
UMASK_OLD="$(umask)"
umask 077
# macOS/BSD mktemp requires XXXXXX at the end of the template (not ".XXXXXX.log").
LOG_FILE="$(mktemp "${TMPDIR:-/tmp}/xentz-agent-local-ui.XXXXXX")"
umask "${UMASK_OLD}"

verify_local_ui_identity() {
  local body
  body="$(curl -fsS --max-time 1 "${LOCAL_UI_URL}/" 2>/dev/null || true)"
  [[ "${body}" == *"xentz-agent Local UI"* ]]
}

verify_local_ui_with_token() {
  local token_file="$1"
  [[ -s "${token_file}" ]] || return 1
  local token
  token="$(tr -d '\r\n' < "${token_file}")"
  [[ -n "${token}" ]] || return 1
  local status
  status="$(curl -fsS --max-time 2 "${LOCAL_UI_URL}/status?token=${token}" 2>/dev/null || true)"
  [[ "${status}" == *"last_backup"* ]] && [[ "${status}" == *"last_retention"* ]]
}

started_local_ui=0
if curl -fsS --max-time 1 "${LOCAL_UI_URL}/" >/dev/null 2>&1; then
  if verify_local_ui_identity && verify_local_ui_with_token "${TOKEN_FILE}"; then
    echo "reusing existing verified local-ui on ${ADDR}..."
  else
    echo "address already in use: ${ADDR}" >&2
    echo "existing listener could not be verified as xentz-agent local-ui." >&2
    echo "stop existing process or choose a different --addr." >&2
    exit 1
  fi
else
  echo "starting local-ui on ${ADDR}..."
  "${AGENT_BIN}" local-ui --addr "${ADDR}" >"${LOG_FILE}" 2>&1 &
  started_local_ui=1
fi

# Wait for token file and endpoint to be ready.
for _ in $(seq 1 40); do
  if [[ -s "${TOKEN_FILE}" ]] && verify_local_ui_identity; then
    break
  fi
  sleep 0.25
done

if [[ ! -s "${TOKEN_FILE}" ]]; then
  echo "token file not found: ${TOKEN_FILE}" >&2
  echo "run '${AGENT_BIN} local-ui --addr ${ADDR}' once and retry." >&2
  if [[ "${started_local_ui}" -eq 1 ]]; then
    echo "local-ui log: ${LOG_FILE}" >&2
  fi
  exit 1
fi
if ! verify_local_ui_identity; then
  echo "local-ui identity check failed on ${ADDR}" >&2
  echo "aborting to avoid token exposure." >&2
  exit 1
fi

TOKEN="$(tr -d '\r\n' < "${TOKEN_FILE}")"
if [[ -z "${TOKEN}" ]]; then
  echo "token file is empty: ${TOKEN_FILE}" >&2
  exit 1
fi

RESTORE_URL="${LOCAL_UI_URL}/restore?token=${TOKEN}"
echo "opening restore wizard..."
"${OPEN_CMD}" "${RESTORE_URL}"

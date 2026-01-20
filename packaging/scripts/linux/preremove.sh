#!/bin/sh
set -e

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop xentz-agent.timer || true
  systemctl disable xentz-agent.timer || true
fi

exit 0

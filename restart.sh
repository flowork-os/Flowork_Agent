#!/usr/bin/env bash
# restart.sh — stop bersih → re-build → start. Forward flag --pause ke start.

set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"

PAUSE_ARG=()
if [ "${1:-}" = "--pause" ]; then
  PAUSE_ARG=(--pause)
fi

# stop ngga perlu pause; start yang nahan terminal (kalau --pause aktif).
"$ROOT/stop.sh"
sleep 0.3
exec "$ROOT/start.sh" "${PAUSE_ARG[@]}"

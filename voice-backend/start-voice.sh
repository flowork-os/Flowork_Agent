#!/usr/bin/env bash
# start-voice.sh — Flowork sovereign voice backend launcher.
#
# Starts two small local services the flow_router voice providers proxy to:
#   - TTS  on :5050  (edge-tts, free, no key)        → tts_server.py
#   - STT  on :5060  (faster-whisper, local/offline) → stt_server.py
#
# Idempotent: skips a service whose port is already listening. Logs + PIDs live
# beside this script. Reversible: `kill $(cat *.pid)` and delete this folder.
set -euo pipefail

VB="$(cd "$(dirname "$0")" && pwd)"
PY="$VB/venv/bin/python"
TTS_PORT="${TTS_PORT:-5050}"
STT_PORT="${STT_PORT:-5060}"

if [ ! -x "$PY" ]; then
  echo "ERROR: venv not found at $PY — create it: python3 -m venv $VB/venv && $VB/venv/bin/pip install edge-tts faster-whisper aiohttp" >&2
  exit 1
fi

port_busy() { ss -tlnH "sport = :$1" 2>/dev/null | grep -q .; }

start_one() {
  local name="$1" script="$2" port="$3"
  if port_busy "$port"; then
    echo "[$name] port $port already in use — skip"
    return 0
  fi
  TTS_PORT="$TTS_PORT" STT_PORT="$STT_PORT" setsid "$PY" "$VB/$script" \
    >"$VB/$name.log" 2>&1 < /dev/null &
  echo $! > "$VB/$name.pid"
  echo "[$name] started (pid $(cat "$VB/$name.pid"), port $port, log $VB/$name.log)"
}

start_one tts tts_server.py "$TTS_PORT"
start_one stt stt_server.py "$STT_PORT"
echo "voice backend up — TTS :$TTS_PORT · STT :$STT_PORT"

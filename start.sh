#!/usr/bin/env bash
# start.sh — bangun (kalau perlu) dan jalanin flowork-gui di background.
#
# Layout:
#   bin/flowork-gui    binary hasil build (auto re-build kalau source lebih baru)
#   /tmp/flowork-gui.pid   PID proses jalan
#   /tmp/flowork-gui.log   stdout + stderr
#
# Convention:
#   exit 0   sukses (sudah jalan atau baru di-start)
#   exit 1   build gagal / start gagal
#   exit 2   sudah jalan, ngga di-restart

set -euo pipefail

# --pause = tunggu Enter di akhir (dipakai oleh .desktop biar terminal
# ngga langsung nutup setelah script kelar).
PAUSE=0
if [ "${1:-}" = "--pause" ]; then
  PAUSE=1
  shift
fi
trap '[ "$PAUSE" = "1" ] && { printf "\n[Enter untuk tutup] "; read -r _; }' EXIT

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

BIN="$ROOT/bin/flowork-gui"
PID_FILE="/tmp/flowork-gui.pid"
LOG_FILE="/tmp/flowork-gui.log"
ADDR="${FLOWORK_ADDR:-127.0.0.1:1987}"

# .desktop launchers run dengan PATH minim — Go SDK custom (mis. di
# ~/go-sdk/bin) sering ngga ke-include. Source rc files + augment PATH
# dari lokasi standar.
for rc in "$HOME/.profile" "$HOME/.bashrc"; do
  # .bashrc biasanya guard against non-interactive — set PS1 dummy biar lewat.
  [ -f "$rc" ] && PS1='${PS1-x}' . "$rc" >/dev/null 2>&1 || true
done
for d in \
  "$HOME/go-sdk/bin" \
  /usr/local/go/bin \
  "$HOME/go/bin" \
  "$HOME/.local/go/bin" \
  /snap/bin \
  /opt/go/bin; do
  [ -d "$d" ] && case ":$PATH:" in *":$d:"*) :;; *) PATH="$d:$PATH";; esac
done
export PATH

# Warna log
c_ok()   { printf '\e[32m%s\e[0m\n' "$*"; }
c_warn() { printf '\e[33m%s\e[0m\n' "$*"; }
c_err()  { printf '\e[31m%s\e[0m\n' "$*"; }
c_info() { printf '\e[36m%s\e[0m\n' "$*"; }

# Cek sudah jalan?
if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  PID="$(cat "$PID_FILE")"
  c_warn "flowork-gui sudah jalan (PID $PID, http://$ADDR)"
  c_info "Pakai ./stop.sh atau ./restart.sh"
  exit 2
fi

# Cek port masih dipake proses orphan?
if ss -tlnp 2>/dev/null | grep -q ":${ADDR##*:} "; then
  c_err "Port ${ADDR##*:} masih dipake proses lain. Cek:"
  ss -tlnp | grep ":${ADDR##*:} " || true
  c_info "Jalankan ./stop.sh dulu, atau set FLOWORK_ADDR=127.0.0.1:PORT_LAIN"
  exit 1
fi

# Build kalau binary belum ada atau source lebih baru.
NEED_BUILD=0
if [ ! -x "$BIN" ]; then
  NEED_BUILD=1
else
  # Rebuild kalau ada .go / web/ lebih baru dari binary
  if [ -n "$(find . -newer "$BIN" \( -name '*.go' -o -path './web/*' \) -not -path './bin/*' 2>/dev/null | head -1)" ]; then
    NEED_BUILD=1
  fi
fi

if [ "$NEED_BUILD" = "1" ]; then
  if ! command -v go >/dev/null 2>&1; then
    c_err "Go compiler ngga ketemu di PATH."
    c_info "Cek install: which go || ls /usr/local/go/bin/go ~/go/bin/go 2>/dev/null"
    c_info "PATH sekarang: $PATH"
    c_info "Set GOROOT/GOPATH di ~/.profile biar .desktop launcher kebawa."
    exit 1
  fi
  c_info "Build flowork-gui ($(go version 2>&1))..."
  mkdir -p "$ROOT/bin"
  if ! go build -o "$BIN" . ; then
    c_err "Build gagal"
    exit 1
  fi
  c_ok "Build OK ($(stat -c%s "$BIN") bytes)"
fi

# Jalanin di background
c_info "Start flowork-gui di http://$ADDR..."
nohup "$BIN" -addr "$ADDR" >"$LOG_FILE" 2>&1 &
PID=$!
echo "$PID" > "$PID_FILE"

# Tunggu sebentar + cek listen
sleep 0.7
if ! kill -0 "$PID" 2>/dev/null; then
  c_err "Proses crash saat boot. Cek log:"
  tail -n 20 "$LOG_FILE"
  rm -f "$PID_FILE"
  exit 1
fi

c_ok "flowork-gui jalan — PID $PID"
c_info "URL : http://$ADDR"
c_info "Log : $LOG_FILE"
c_info "PID : $PID_FILE"

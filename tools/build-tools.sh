#!/usr/bin/env bash
# build-tools.sh — compile tiap TOOL SIDECAR (tools/<name>/) jadi binary native sendiri.
#
# Prinsip (owner 2026-06-23): tiap tool = MODUL Go SENDIRI (own go.mod + dependency di folder
# sendiri, NOL shared lib) → build TERISOLASI (GOWORK=off). Itu yang bikin plug-and-play + agnostic.
# Host (toolsidecar.go) discover + register binary ini sbg tool dinamis, di-exec sbg proses terpisah.
#
# Pakai: ./tools/build-tools.sh            (host OS)
#        GOOS=android GOARCH=arm64 ./tools/build-tools.sh   (cross-compile mobile — sidecar prebuilt)
set -uo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
EXT=""; [ "${GOOS:-}" = "windows" ] && EXT=".exe"
built=0; failed=0; skipped=0
for d in "$ROOT"/*/; do
  [ -f "${d}tool.json" ] || { continue; }
  if [ ! -f "${d}go.mod" ]; then
    echo "  ⚠ $(basename "$d"): GA ADA go.mod — tiap tool WAJIB modul sendiri (isolasi). skip."
    skipped=$((skipped+1)); continue
  fi
  name="$(python3 -c "import json;print(json.load(open('${d}tool.json'))['name'])" 2>/dev/null)"
  [ -n "$name" ] || { echo "  ⚠ $(basename "$d"): tool.json invalid. skip."; skipped=$((skipped+1)); continue; }
  echo "  building $name …"
  if ( cd "$d" && GOWORK=off go build -o "${name}${EXT}" . 2>&1 | sed 's/^/    /' ); then
    built=$((built+1))
  else
    echo "    ✗ $name build GAGAL"; failed=$((failed+1))
  fi
done
echo "✅ sidecar tools: built=$built failed=$failed skipped=$skipped"
[ "$failed" -gt 0 ] && exit 1 || exit 0

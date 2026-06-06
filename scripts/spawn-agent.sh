#!/usr/bin/env bash
# spawn-agent.sh — bikin agent baru dari TEMPLATE (default: mr-flow).
#
# Doktrin Fase 2 (roadmap): Mr.Flow = engine template. Agent baru = COPAS folder
# → ganti id + persona + tool subscription. 1 engine, banyak warga, footprint
# diatur per-copy. Engine (main.go) generic; yang beda cuma config.
#
# Pakai:
#   ./scripts/spawn-agent.sh analisa-fundamental
#   ./scripts/spawn-agent.sh crypto-scout --from mr-flow
#   ./scripts/spawn-agent.sh dummy --no-build      # cuma scaffold, ga compile
#
# Yang DI-COPY: main.go (engine), go.mod, manifest.json.
# Yang DI-SKIP: workspace/ (runtime), *.db / state.db (data terisolasi — warga
#   baru bikin DB-nya sendiri saat run). Persona + tool subscription DIATUR via
#   GUI popup (FLOWORK_AGENT_CONFIG dari SQLite), BUKAN di source.
#
# Setelah spawn: buka popup agent baru → set persona + centang tools yang perlu
# (sedikit = prompt kecil = anti over-prompt). Engine-nya identik Mr.Flow.

set -euo pipefail

# ── Args ────────────────────────────────────────────────────────────────────
NEW_ID=""
FROM="mr-flow"
DO_BUILD=1

while [ $# -gt 0 ]; do
  case "$1" in
    --from) FROM="${2:-}"; shift 2 ;;
    --no-build) DO_BUILD=0; shift ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; exit 1 ;;
    *)
      if [ -z "$NEW_ID" ]; then NEW_ID="$1"; else
        echo "unexpected arg: $1" >&2; exit 1
      fi
      shift ;;
  esac
done

if [ -z "$NEW_ID" ]; then
  echo "Usage: $0 <new-agent-id> [--from <template-id>] [--no-build]" >&2
  exit 1
fi

# ── Resolve repo root (script ada di <root>/scripts/) ───────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

# ── Validate id (id ini = FLOWORK_AGENT_ID = dipake di URL self-API) ─────────
if ! printf '%s' "$NEW_ID" | grep -qE '^[a-z][a-z0-9-]*$'; then
  echo "id invalid: '$NEW_ID' — harus lowercase, mulai huruf, cuma [a-z0-9-]." >&2
  exit 1
fi

SRC="agents/$FROM"
DST="agents/$NEW_ID"

if [ ! -d "$SRC" ]; then
  echo "template ngga ada: $SRC" >&2
  exit 1
fi
if [ ! -f "$SRC/manifest.json" ] || [ ! -f "$SRC/main.go" ]; then
  echo "template ngga lengkap (butuh main.go + manifest.json): $SRC" >&2
  exit 1
fi
if [ -e "$DST" ]; then
  echo "agent '$NEW_ID' udah ada: $DST — pilih id lain atau hapus dulu." >&2
  exit 1
fi

echo "→ Template : $SRC"
echo "→ Agent baru: $DST"
echo

# ── Scaffold: copy engine + config, SKIP runtime/data ───────────────────────
mkdir -p "$DST"
cp "$SRC/main.go" "$DST/main.go"
cp "$SRC/go.mod"  "$DST/go.mod"
cp "$SRC/manifest.json" "$DST/manifest.json"
echo "✓ copy engine (main.go) + go.mod + manifest.json"
echo "  (skip: workspace/, *.db — warga baru terisolasi, bikin DB sendiri saat run)"

# go.mod module name → id baru (rapi; build single-file ga strict tapi konsisten)
sed -i "s/^module .*/module $NEW_ID/" "$DST/go.mod"

# manifest.json: set id + display_name + description generic. Engine-generic
# fields (capabilities/exposes_rpc/ui_schema) DIBIARKAN — itu bagian engine.
DISPLAY="$(printf '%s' "$NEW_ID" | sed -e 's/-/ /g' -e 's/\b\(.\)/\u\1/g')"
NEW_ID="$NEW_ID" DISPLAY="$DISPLAY" python3 - "$DST/manifest.json" <<'PY'
import json, os, sys
path = sys.argv[1]
with open(path) as f:
    m = json.load(f)
m["id"] = os.environ["NEW_ID"]
m["display_name"] = os.environ["DISPLAY"]
m["description"] = ("Warga Flowork (spawn dari template mr-flow). Engine sama "
                    "Mr.Flow; persona + tool subscription diatur per-agent via "
                    "popup. Footprint kecil = subscribe tools seperlunya.")
with open(path, "w") as f:
    json.dump(m, f, indent=2, ensure_ascii=False)
    f.write("\n")
PY
echo "✓ manifest: id='$NEW_ID' display_name='$DISPLAY' + description generic"

# ── Build (opsional) ─────────────────────────────────────────────────────────
if [ "$DO_BUILD" -eq 1 ]; then
  echo
  echo "── build wasm ──"
  ./scripts/build-agent.sh "$NEW_ID"
else
  echo
  echo "ⓘ skip build (--no-build). Build manual: ./scripts/build-agent.sh $NEW_ID"
fi

cat <<EOF

✅ Agent '$NEW_ID' kelar di-scaffold.

Langkah berikut (config per-agent, BUKAN di code):
  1. Restart host (./restart.sh) biar warga baru ke-load.
  2. Buka GUI → popup agent '$NEW_ID':
       • set PERSONA (prompt) — ini yang bikin dia beda dari Mr.Flow.
       • centang TOOLS yang dibutuhin doang (sedikit = prompt kecil).
       • isi credentials kalau perlu (mis. TELEGRAM_BOT_TOKEN).
  Engine-nya identik Mr.Flow (tool-loop, sanitize, prune, friendly-error).
EOF

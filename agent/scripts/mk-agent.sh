#!/usr/bin/env bash
# mk-agent.sh — bikin agent warga baru dari agent-template (dumb-agent generik, single-file).
# Pengganti spawn-agent.sh --from mr-flow (rusak: mr-flow multi-file). Full: scaffold+build+config.
#
# Pakai: ./scripts/mk-agent.sh <id> <model> <persona> [tools-csv] [extra-cap-csv]
#   model: claude-opus-4-8 | claude-haiku-4-5 | ...
set -uo pipefail
ID="${1:?id}"; MODEL="${2:?model}"; PERSONA="${3:?persona}"; TOOLS="${4:-}"; EXTRACAPS="${5:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; ROOT="$(dirname "$SCRIPT_DIR")"; cd "$ROOT"

echo "→ mk-agent $ID (model=$MODEL)"
rm -rf "agents/$ID" "$HOME/.flowork/agents/$ID.fwagent"
mkdir -p "agents/$ID"
cp templates/agent-template/main.go templates/agent-template/go.mod templates/agent-template/manifest.json "agents/$ID/"
sed -i "s/^module .*/module $ID/" "agents/$ID/go.mod"
sed -i "s/^go 1\.[0-9]*.*/go 1.23/" "agents/$ID/go.mod"

ID="$ID" EXTRACAPS="$EXTRACAPS" python3 - "agents/$ID/manifest.json" <<'PY'
import json,os,sys
p=sys.argv[1]; m=json.load(open(p)); i=os.environ["ID"]
m["id"]=i; m["display_name"]=i.replace("-"," ").title()
m["description"]=f"Squad member {i} (koloni: 1 agen 1 tugas)."
for k in list(m):
    if k.startswith("_comment") or k in ("i18n_keys","ui_schema"): m.pop(k,None)
for c in [x for x in os.environ.get("EXTRACAPS","").split(",") if x.strip()]:
    if c not in m["capabilities_required"]: m["capabilities_required"].append(c)
json.dump(m,open(p,"w"),indent=2,ensure_ascii=False); open(p,"a").write("\n")
PY

echo "── build wasm ──"
GOWORK=off GOTOOLCHAIN=local GOROOT="$HOME/sdk/go1.23.4" PATH="$HOME/sdk/go1.23.4/bin:$PATH" \
  TINYGO="$HOME/.local/share/tinygo/bin/tinygo" ./scripts/build-agent.sh "$ID" >/dev/null 2>&1 \
  && echo "  wasm OK ($(stat -c%s "$HOME/.flowork/agents/$ID.fwagent/agent.wasm")b)" || { echo "  BUILD GAGAL"; exit 1; }

# trigger host load (watcher = DIR event → mv keluar-masuk)
( cd "$HOME/.flowork/agents"; mv "$ID.fwagent" "/tmp/$ID.stage"; sleep 2; mv "/tmp/$ID.stage" "$ID.fwagent" )
echo "  triggered load — nunggu provision..."
DB="agents/$ID/workspace/state.db"
for i in $(seq 1 15); do [ -f "$DB" ] && break; sleep 1; done
[ -f "$DB" ] || { echo "  ⚠️ state.db ga kebuat (cek load)"; exit 1; }

GOWORK=off go run ./cmd/agent-config "$DB" "$PERSONA" "$TOOLS" 2>&1 | tail -1 | sed 's/^/  /'
sqlite3 "$DB" "INSERT INTO kv(k,v) VALUES('router_model','$MODEL') ON CONFLICT(k) DO UPDATE SET v=excluded.v;"
# RELOAD WAJIB: persona (kv prompt) + model dibaca pas BOOT → reload biar config baru kepake.
( cd "$HOME/.flowork/agents"; mv "$ID.fwagent" "/tmp/$ID.reload"; sleep 2; mv "/tmp/$ID.reload" "$ID.fwagent" ); sleep 4
echo "  ✓ $ID siap (reloaded): model=$(sqlite3 "$DB" "SELECT v FROM kv WHERE k='router_model'") persona=$(sqlite3 "$DB" "SELECT length(v) FROM kv WHERE k='prompt'")ch"

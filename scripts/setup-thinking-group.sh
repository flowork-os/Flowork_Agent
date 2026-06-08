#!/usr/bin/env bash
# setup-thinking-group.sh — deploy the SEQUENTIAL `thinking` group orchestrator and
# wire it so Mr.Flow can delegate to it. Run AFTER scripts/setup-thinking.sh (the
# 4 member agents) + scripts/ingest-thinking-brains.sh (their brains).
#
# Pipeline (owner's way of thinking): questioner → lenses (answer subject+questions)
# → synthesizer. The orchestration is in the wasm; the roster is a transparent
# workspace/roster.json (no hardcoding). Reproducible + plug-and-play.
#
# This DOES create a group (an owner action). The owner explicitly directed it.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
GID="thinking"
WASM="$ROOT/templates/thinking-group/agent.wasm"
DST="$AGENTS/$GID.fwagent"

[ -f "$WASM" ] || { echo "missing $WASM (build: cd templates/thinking-group && GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .)"; exit 1; }

echo "→ deploy group $GID"
mkdir -p "$DST/workspace"
cp "$WASM" "$DST/agent.wasm"
python3 - "$GID" "$DST/manifest.json" <<'PY'
import json,sys
gid,out=sys.argv[1:3]
json.dump({
 "id":gid,"version":"1.0.0","kind":"agent","display_name":"Thinking",
 "description":"Sequential thinking colony: frame the questions, view through each grounded lens, synthesize one decision. Domain-agnostic.",
 "min_kernel_version":"0.1.0","abi_version":1,"author":"@flowork-os","license":"MIT",
 "entry":"agent.wasm","memory_max_mb":16,"timeout_call_ms":120000,
 "capabilities_required":[
   "net:fetch:http://127.0.0.1:1987/api/kernel/call",
   "state:read","state:write","time:read"],
 "exposes_rpc":[{"name":"handle_message","description":"Run the thinking pipeline on one subject.","input_schema":{"type":"object","properties":{}}}]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY

# Mark it a GROUP + write the roster into its OWN loket store — the SAME store the
# Group Colony menu reads/writes, so `thinking` shows up in the colony AND its
# roster is menu-editable. "members" = the editable LENSES; questioner +
# synthesizer are their own keys (pipeline roles). (Same approach as the menu's
# create+config; kv table created if missing, like the operator group script.)
GLOKET="$DST/workspace/loket.db"
set_kv() { sqlite3 "$1" "PRAGMA busy_timeout=30000;
  CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL DEFAULT '');
  INSERT INTO kv(k,v) VALUES('$2','$3') ON CONFLICT(k) DO UPDATE SET v=excluded.v;" >/dev/null; }
set_kv "$GLOKET" group "1"
set_kv "$GLOKET" display_name "Thinking"
set_kv "$GLOKET" members "thinking-strategy,thinking-improvement"
set_kv "$GLOKET" questioner "thinking-questions"
set_kv "$GLOKET" how_agent "thinking-how"
set_kv "$GLOKET" synthesizer "thinking-synthesis"
set_kv "$GLOKET" task "Rumuskan pertanyaan kunci, tinjau lewat tiap lensa, lalu sintesis jadi satu keputusan."
echo "→ marked group=1 + roster in loket store (menu-visible + menu-editable)"

# Register the group in Mr.Flow's OWN allowlist (kv "groups" = "id|command|desc;…").
# The command is the auto Telegram slash command (/thinking); mr-flow routes /<command>
# to this group, and telegram-channel auto-registers it in the slash menu on boot.
MFLOKET="$AGENTS/mr-flow-next.fwagent/workspace/loket.db"
COMMAND="thinking"
DESC="Mikir bareng tim thinking (strategi, perbaikan, persuasi) — multi-turn"
ENTRY="$GID|$COMMAND|$DESC"
if [ -f "$MFLOKET" ]; then
  cur=$(sqlite3 "$MFLOKET" "SELECT v FROM kv WHERE k='groups';" 2>/dev/null || true)
  case "$cur" in
    *"$GID|"*|*"$GID:"*) echo "→ mr-flow already knows group $GID";;
    *) sqlite3 "$MFLOKET" "CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL DEFAULT '');
                           INSERT INTO kv(k,v) VALUES('groups','${cur:+$cur; }$ENTRY')
                           ON CONFLICT(k) DO UPDATE SET v=excluded.v;"
       echo "→ registered group $GID (/$COMMAND) in mr-flow allowlist";;
  esac
else
  echo "ⓘ mr-flow loket store belum ada — boot mr-flow dulu lalu ulangi."
fi

echo "✅ group $GID deployed + wired. Test: rpc handle_message {text:...} ke '$GID', atau chat ke mr-flow."

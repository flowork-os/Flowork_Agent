#!/usr/bin/env bash
# setup-operator-group.sh — wire the "operasi-komputer" GROUP so Mr.Flow can drive
# the computer from a chat: mr-flow → ask_group(operasi-komputer-grup) → member
# operator-komputer (executor) → system_power / app_open.
#
# Prereq: ./scripts/setup-operator.sh (creates the operator-komputer executor) and
# at least one ./restart.sh (so the group's loket store exists). Reproducible.
#
# What it does (idempotent):
#   1. scaffold the GROUP agent operasi-komputer-grup from templates/group-template
#   2. set its roster in its OWN loket store (loket.db kv): members=operator-komputer
#   3. register the group in mr-flow-next's loket store kv "groups" (so ask_group sees it)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
GID="operasi-komputer-grup"
TPL="$ROOT/templates/group-template"
DST="$AGENTS/$GID.fwagent"

echo "→ scaffold GROUP $GID"
mkdir -p "$DST/workspace"
cp "$TPL/agent.wasm" "$DST/agent.wasm"
# manifest: clone the group-template manifest, set id/display
python3 - "$TPL/manifest.json" "$DST/manifest.json" "$GID" <<'PY'
import json,sys
src,dst,gid=sys.argv[1],sys.argv[2],sys.argv[3]
m=json.load(open(src)); m["id"]=gid; m["display_name"]="Group Operasi Komputer"
m["description"]="GROUP kontrol komputer: teruskan perintah (buka app/shutdown/restart/timer) ke anggota operator-komputer (executor), relay hasilnya."
json.dump(m,open(dst,"w"),indent=2,ensure_ascii=False); open(dst,"a").write("\n")
PY
[ -f "$TPL/loket.json" ] && python3 - "$TPL/loket.json" "$DST/loket.json" "$GID" <<'PY'
import json,sys
src,dst,gid=sys.argv[1],sys.argv[2],sys.argv[3]
m=json.load(open(src)); m["id"]=gid
json.dump(m,open(dst,"w"),indent=2,ensure_ascii=False)
PY

# helper: set a kv row in a loket store (create table if missing)
set_kv() { # $1=db $2=key $3=val
  sqlite3 "$1" "CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL DEFAULT '');
                INSERT INTO kv(k,v) VALUES('$2','$3') ON CONFLICT(k) DO UPDATE SET v=excluded.v;"
}

GLOKET="$DST/workspace/loket.db"
echo "→ roster (loket store) members=operator-komputer"
set_kv "$GLOKET" members "operator-komputer"
set_kv "$GLOKET" synthesizer ""
set_kv "$GLOKET" task "Teruskan perintah kontrol komputer ke anggota apa adanya; anggota (operator-komputer) yg eksekusi via system_power/app_open. Relay hasil eksekusinya."

MFLOKET="$AGENTS/mr-flow-next.fwagent/workspace/loket.db"
if [ -f "$MFLOKET" ]; then
  cur=$(sqlite3 "$MFLOKET" "SELECT v FROM kv WHERE k='groups';" 2>/dev/null || true)
  case "$cur" in
    *"$GID"*) echo "→ mr-flow already knows the group";;
    *) set_kv "$MFLOKET" groups "${cur:+$cur; }$GID:Kontrol komputer host — buka chrome/vscode, shutdown, restart, suspend, lock, timer"
       echo "→ registered group in mr-flow kv groups";;
  esac
else
  echo "ⓘ mr-flow loket store belum ada — jalanin ./restart.sh dulu lalu ulangi script ini."
fi

echo "✅ GROUP operasi-komputer-grup siap. Jalanin ./restart.sh, lalu chat: 'matiin pc 30 menit' / 'buka chrome'."

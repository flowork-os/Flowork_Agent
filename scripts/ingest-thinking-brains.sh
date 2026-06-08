#!/usr/bin/env bash
# ingest-thinking-brains.sh — seed the grounded lens agents' brains from their own
# workspace/seed.jsonl (white-label patterns). Replicates the kernel's BrainAdd
# exactly (sha256[:16] id, dedup by content_hash, brain_drawers + brain_fts), so a
# direct seed is byte-identical to what store.brain.add would have written.
#
# Run AFTER the agents have booted once (loket.db + brain tables exist). Idempotent:
# dedup by content hash, so re-running adds nothing new. Reversible: delete the
# agent folder or DELETE FROM brain_drawers WHERE wing='doctrine'.
set -euo pipefail
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"

ingest() { # $1=agent-id
  local id="$1" db="$AGENTS/$1.fwagent/workspace/loket.db" seed="$AGENTS/$1.fwagent/workspace/seed.jsonl"
  [ -f "$seed" ] || { echo "  $id: no seed.jsonl — skip"; return; }
  [ -f "$db" ]   || { echo "  $id: no loket.db (boot the agent once first) — skip"; return; }
  python3 - "$db" "$seed" "$id" <<'PY'
import json,sqlite3,hashlib,sys,datetime
db,seed,agent=sys.argv[1:4]
con=sqlite3.connect(db,timeout=30); con.execute("PRAGMA busy_timeout=30000")
cur=con.cursor()
now=datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
added=dup=0
for line in open(seed,encoding="utf-8"):
    line=line.strip()
    if not line: continue
    rec=json.loads(line)
    content=(rec.get("pattern") or "").strip()
    if not content: continue
    h=hashlib.sha256(content.encode()).hexdigest(); cid=h[:16]
    if cur.execute("SELECT 1 FROM brain_drawers WHERE content_hash=? LIMIT 1",(h,)).fetchone():
        dup+=1; continue
    cur.execute("INSERT OR IGNORE INTO brain_drawers(id,content,wing,room,content_hash,created_at) VALUES(?,?,?,?,?,?)",
                (cid,content,"doctrine","",h,now))
    cur.execute("INSERT INTO brain_fts(drawer_id,content,wing,room) VALUES(?,?,?,?)",(cid,content,"doctrine",""))
    added+=1
con.commit()
total=cur.execute("SELECT count(*) FROM brain_drawers WHERE wing='doctrine'").fetchone()[0]
con.close()
print(f"  {agent}: +{added} new, {dup} dup → {total} doctrine drawers")
PY
}

echo "→ ingest grounded lenses"
ingest thinking-strategy
ingest thinking-improvement
ingest thinking-influence
echo "✅ ingest done"

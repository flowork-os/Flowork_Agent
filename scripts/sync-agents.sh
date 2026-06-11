#!/usr/bin/env bash
# sync-agents.sh — mirror the agent DEFINITIONS from the live runtime dir
# (~/.flowork/agents, or $FLOWORK_AGENTS_DIR) into this repo's agents/ folder so
# every agent + group you create can be committed to GitHub.
#
# It copies ONLY the definition files (manifest.json, loket.json, prompt.md,
# doktrin.md, main.go, go.mod, .gitignore) and DELIBERATELY EXCLUDES runtime +
# secret data: workspace/ (state.db with bot tokens), shared/ output, *.db, and
# *.wasm (built artifacts — .gitignore drops these too). Nothing secret leaves.
#
# Usage:  ./scripts/sync-agents.sh           # sync, then review `git status`
#         ./scripts/sync-agents.sh --push    # sync + commit + push
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
DST="$REPO_DIR/agents"

if [ ! -d "$SRC" ]; then
  echo "✗ runtime agents dir not found: $SRC" >&2
  exit 1
fi

echo "→ syncing agent definitions"
echo "  from: $SRC"
echo "  to:   $DST"

rsync -a \
  --exclude 'workspace/' \
  --exclude 'shared/' \
  --exclude 'state.db' --exclude '*.db' --exclude '*.db-shm' --exclude '*.db-wal' \
  --exclude '*.sqlite*' \
  --exclude 'config.json.migrated' \
  "$SRC"/ "$DST"/

count=$(find "$DST" -maxdepth 1 -type d -name '*.fwagent' | wc -l)
echo "✓ synced — $count *.fwagent folders now in repo/agents/"

# Safety net: never let a wasm/db/secret slip into the index.
staged_bad=$(cd "$REPO_DIR" && git add -An agents/ 2>/dev/null | grep -ciE '\.wasm|\.db|workspace/|/shared/' || true)
if [ "$staged_bad" != "0" ]; then
  echo "✗ ABORT: $staged_bad runtime/secret files would be staged — check .gitignore" >&2
  exit 1
fi

if [ "${1:-}" = "--push" ]; then
  cd "$REPO_DIR"
  git add agents/
  git commit -q -m "agents: sync agent/group definitions from runtime" || { echo "nothing to commit"; exit 0; }
  git push origin main
  echo "✓ committed + pushed"
else
  echo "  review with: git -C \"$REPO_DIR\" status agents/   (then commit/push)"
fi

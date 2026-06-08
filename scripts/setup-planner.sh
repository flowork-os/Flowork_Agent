#!/usr/bin/env bash
# setup-planner.sh — deploy the reusable `thinking-planner` module (ROADMAP_THINKING.md
# item 4-5): the PLAN half of disciplined execution for DYNAMIC colonies.
#
# Split-by-design: the planner (LLM) drafts steps; the LIST lives in a deterministic
# loket store (kv), never in the model — so progress can't be hallucinated. Functions:
#   plan {goal} · status {} · done {n}.
#
# Reproducible, plug-and-play (own folder). Build the wasm first:
#   cd templates/planner-template && GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
WASM="$ROOT/templates/planner-template/agent.wasm"
ID="thinking-planner"
DST="$AGENTS/$ID.fwagent"

[ -f "$WASM" ] || { echo "missing $WASM (build: cd templates/planner-template && GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .)"; exit 1; }

echo "→ deploy $ID"
mkdir -p "$DST/workspace"
cp "$WASM" "$DST/agent.wasm"
python3 - "$ID" "$DST/manifest.json" <<'PY'
import json,sys
id,out=sys.argv[1:3]
json.dump({
 "id":id,"version":"1.0.0","kind":"agent","display_name":"Thinking — Planner",
 "description":"Drafts a step plan (LLM) and stores it in a deterministic kv list; status is mechanical truth, not model memory. Functions: plan/status/done.",
 "min_kernel_version":"0.1.0","abi_version":1,"author":"@flowork-os","license":"MIT",
 "entry":"agent.wasm","memory_max_mb":16,"timeout_call_ms":120000,
 "capabilities_required":[
   "net:fetch:http://127.0.0.1:1987/api/kernel/call",
   "state:read","state:write","time:read"],
 "exposes_rpc":[
   {"name":"plan","description":"Draft a step plan for a goal and store it.","input_schema":{"type":"object","properties":{}}},
   {"name":"status","description":"Read the stored plan + step statuses (ground truth).","input_schema":{"type":"object","properties":{}}},
   {"name":"done","description":"Mark step n done (mechanical).","input_schema":{"type":"object","properties":{}}}]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY
printf '%s' 'You are a PLANNER. Given a goal, produce a numbered plan of 3 to 6 concrete, ACTIONABLE, testable steps to reach it. Frame with 5W+1H, generate with "how". Each step one line, no preamble. Output ONLY the numbered steps. Respond in the same language as the goal.' > "$DST/workspace/prompt.md"
printf '%s' 'SACRED RULE (anti-hallucination): the steps must be real and testable — never invent fake milestones. You only DRAFT the plan; you do NOT track progress (the store does). Respond in the same language as the goal.' > "$DST/workspace/doktrin.md"
echo "✅ $ID deployed. Boot it, then: plan {goal} → status {} → done {n}."

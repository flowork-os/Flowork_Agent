#!/usr/bin/env bash
# setup-thinking.sh — deploy the MEMBER agents of the `thinking` group: a colony
# that adopts the owner's way of thinking. Domain-agnostic (strategy, conflict,
# relationships, anything). Members are tiny "ant" specialists, each one lens:
#
#   thinking-questions    — generates the 5W+1H questions that frame a situation
#   thinking-strategy     — answers through a STRATEGY lens, grounded (RAG) in its brain
#   thinking-improvement  — answers through a CONTINUOUS-IMPROVEMENT lens, grounded (RAG)
#   thinking-synthesis    — fuses the lenses into one decision
#
# WHITE-LABEL: ids/personas carry NO brand names — only the pattern. The strategy
# & improvement lenses are GROUNDED: their brains are seeded from white-label
# corpora (workspace/seed.jsonl) and the lens wasm injects retrieved principles
# into the prompt, so they speak from ingested patterns, not free imagination.
#
# This deploys the AGENTS only. The GROUP itself is created by the OWNER via the
# Groups menu (owner-gated by design — an AI must not self-create a group). After
# this script + ingest, the four agents appear in the Groups menu's available list.
#
# Idempotent. Reproducible. Plug-and-play: each agent is its own folder.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
ANT_WASM="$ROOT/templates/ant-template/agent.wasm"
LENS_WASM="$ROOT/templates/lens-template/agent.wasm"
SEEDS="$ROOT/seeds/thinking"

[ -f "$ANT_WASM" ]  || { echo "missing $ANT_WASM";  exit 1; }
[ -f "$LENS_WASM" ] || { echo "missing $LENS_WASM (build it: cd templates/lens-template && GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .)"; exit 1; }

# deploy <id> <wasm> <display> <description>   (persona+doktrin+seed written by caller after)
deploy() {
  local id="$1" wasm="$2" disp="$3" desc="$4"
  local dst="$AGENTS/$id.fwagent"
  echo "→ deploy $id"
  mkdir -p "$dst/workspace"
  cp "$wasm" "$dst/agent.wasm"
  python3 - "$id" "$disp" "$desc" "$dst/manifest.json" <<'PY'
import json,sys
id,disp,desc,out=sys.argv[1:5]
json.dump({
 "id":id,"version":"1.0.0","kind":"agent","display_name":disp,
 "description":desc,"min_kernel_version":"0.1.0","abi_version":1,
 "author":"@flowork-os","license":"MIT","entry":"agent.wasm",
 "memory_max_mb":16,"timeout_call_ms":120000,
 "capabilities_required":[
   "net:fetch:http://127.0.0.1:1987/api/kernel/call",
   "state:read","state:write","time:read"],
 "exposes_rpc":[{"name":"handle_message","description":"Handle one task message through the loket.","input_schema":{"type":"object","properties":{}}}]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY
  printf '%s' "$5" > "$dst/workspace/prompt.md"
  printf '%s' "$6" > "$dst/workspace/doktrin.md"
}

DOKTRIN_GROUNDED='SACRED RULE (anti-hallucination): answer ONLY from the principles provided to you in context. If they do not cover the subject, say plainly you have no grounded basis for it. Never fabricate facts, numbers, names, or principles. Respond in the same language as the subject.'
DOKTRIN_PLAIN='SACRED RULE (anti-hallucination): reason only from what you are given. Do not invent facts, numbers, or names. If you are unsure, say so plainly. Respond in the same language as the subject.'

# 1) questioner — plain ant (no doctrine brain; it frames, it does not answer)
deploy thinking-questions "$ANT_WASM" "Thinking — Questions" \
 "Frames a situation into the 5W+1H questions that must be answered to decide well." \
 'You receive a SITUATION. Generate the sharpest, most decision-relevant questions using 5W+1H (What, Why, Who, When, Where, How). Output 6 to 10 crisp questions as a bullet list. Do NOT answer them — only ask. Domain-agnostic: works for business, conflict, law, relationships, anything.' \
 "$DOKTRIN_PLAIN"

# 1b) how-engine — plain ant (generative: MANUFACTURE paths; the "bagaimana caranya" organ)
deploy thinking-how "$ANT_WASM" "Thinking — How" \
 "Manufactures concrete paths ('how') for a goal — divergent generation, no judging." \
 'You receive a SITUATION/goal and the key questions about it. Your ONLY job: MANUFACTURE exactly 3 concrete, DIFFERENT paths for HOW to reach it — a numbered list, ONE to TWO lines each, no preamble. Each path = a real, specific route (not a platitude); make them genuinely different. Do NOT judge which is best. Never answer "it is impossible" — turn it into "how" (unless the path is fatal/irreversible, then say so in one line). Domain-agnostic. Be brief.' \
 "$DOKTRIN_PLAIN"

# 2) strategy lens — RAG, grounded in the strategy corpus
deploy thinking-strategy "$LENS_WASM" "Thinking — Strategy" \
 "Analyzes a subject through a strategy lens, grounded in retrieved principles." \
 'You analyze a subject through a STRATEGY lens, grounded ONLY in the principles retrieved into your context: positioning, timing, knowing the terrain and the opponent, winning at the least cost, never fighting on the enemy strength. Max 3 tight bullets, ONE line each, no preamble. If the retrieved principles do not cover it, say so — never invent.' \
 "$DOKTRIN_GROUNDED"

# 3) improvement lens — RAG, grounded in the improvement corpus
deploy thinking-improvement "$LENS_WASM" "Thinking — Improvement" \
 "Analyzes a subject through a continuous-improvement lens, grounded in retrieved principles." \
 'You analyze a subject through a CONTINUOUS-IMPROVEMENT lens, grounded ONLY in the principles retrieved into your context: getting genuinely better through small consistent steps, removing waste, fixing root causes, standardizing gains. Max 3 tight bullets, ONE line each, no preamble. If the retrieved principles do not cover it, say so — never invent.' \
 "$DOKTRIN_GROUNDED"

# 4) synthesizer — plain ant (fuses the lenses)
deploy thinking-synthesis "$ANT_WASM" "Thinking — Synthesis" \
 "Fuses the lens analyses into one balanced decision." \
 'You are the SYNTHESIZER. You receive a subject plus analyses from a strategy lens and an improvement lens. Weave them into ONE coherent decision. Be CONCISE: 2-3 sentences of integrated reasoning, then 3-5 concrete next steps as a short list. No padding, no repeating the inputs verbatim. Balance both lenses. Domain-agnostic.' \
 "$DOKTRIN_PLAIN"

# Seed the two grounded lenses with their white-label corpora (travels with the agent).
if [ -f "$SEEDS/strategy.jsonl" ]; then
  cp "$SEEDS/strategy.jsonl"    "$AGENTS/thinking-strategy.fwagent/workspace/seed.jsonl"
  echo "→ seeded thinking-strategy ($(wc -l < "$SEEDS/strategy.jsonl") patterns staged)"
fi
if [ -f "$SEEDS/improvement.jsonl" ]; then
  cp "$SEEDS/improvement.jsonl" "$AGENTS/thinking-improvement.fwagent/workspace/seed.jsonl"
  echo "→ seeded thinking-improvement ($(wc -l < "$SEEDS/improvement.jsonl") patterns staged)"
fi

echo
echo "✅ 4 member agents deployed to $AGENTS"
echo "   next: ./restart.sh (or wait for hot-load) → scripts/ingest-thinking-brains.sh → create group in Groups menu"

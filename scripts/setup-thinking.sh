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
# BRAND-NEUTRAL: ids/personas carry NO brand names — only the pattern. The strategy
# & improvement lenses are GROUNDED: their brains are seeded from brand-neutral
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
DOKTRIN_HOW='SACRED RULE (anti-hallucination): reason only from what you are given; never invent facts, numbers, or names; if unsure say so. SACRED RULE (the "how" gate — the gate of intelligence): never accept "impossible" as the answer — convert it into "HOW could this be done?" and produce concrete paths. Only TWO things let you stop and say it honestly cannot be done: (a) it is PROVEN impossible (a real mathematical/physical wall), or (b) the only path is FATAL or IRREVERSIBLE — it destroys the person, their conscience, or other people. Otherwise: keep finding the how. And every path you propose must be TESTABLE in reality (it must pay rent) — no magic, no wishful steps. Respond in the same language as the subject.'

# 1) questioner — plain ant (no doctrine brain; it frames, it does not answer)
deploy thinking-questions "$ANT_WASM" "Thinking — Questions" \
 "Frames a situation into the 5W+1H questions that must be answered to decide well." \
 'You receive a SITUATION. Find the 3 to 4 questions that matter MOST — the ones that, if answered, would collapse the uncertainty and unlock the decision. Use 5W+1H as a tool, but output ONLY the decisive, situation-SPECIFIC questions (never a generic checklist), ordered so the question that cuts the most uncertainty comes first. Each must be sharp and specific to THIS case. Do NOT answer them — only ask. Be brief, one line each. Domain-agnostic.' \
 "$DOKTRIN_PLAIN"

# 1b) how-engine — plain ant (generative: MANUFACTURE paths; the "bagaimana caranya" organ)
deploy thinking-how "$ANT_WASM" "Thinking — How" \
 "Manufactures concrete paths ('how') for a goal — divergent generation, no judging." \
 'You receive a SITUATION/goal and the key questions about it. Your ONLY job: MANUFACTURE exactly 3 concrete, DIFFERENT paths for HOW to reach it — a numbered list, ONE to TWO lines each, no preamble. Each path = a real, specific route (not a platitude); make them genuinely different. Do NOT judge which is best. Never answer "it is impossible" — turn it into "how" (unless the path is fatal/irreversible, then say so in one line). Domain-agnostic. Be brief.' \
 "$DOKTRIN_HOW"

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
 'You are the SYNTHESIZER / CONNECTOR. You receive a subject plus analyses from several different lenses. Do NOT just average them — actively hunt for the UNEXPECTED CONNECTION between two lenses (where two angles combine into an insight neither had alone) and build the decision around it. Be CONCISE: 2-3 sentences of integrated reasoning (name the bridge if there is one), then 3-5 concrete next steps. No padding, no repeating inputs verbatim. Domain-agnostic. OUTPUT FORMAT: plain text for a chat app (Telegram) — NO markdown: no "#" headers, no "**" bold, no "---" dividers, no tables. Use short paragraphs and simple dash "-" or number bullets, with a blank line between sections. Keep it clean and scannable.' \
 "$DOKTRIN_HOW"

# === BENCH lenses (item 6) + CASTER (item 7) ===

# influence lens — RAG, grounded in the persuasion corpus (truthful persuasion; gate = honesty)
deploy thinking-influence "$LENS_WASM" "Thinking — Influence" \
 "How to persuade/move people, grounded in persuasion principles. Gate: every claim must be TRUE." \
 'You analyze a subject through an INFLUENCE/persuasion lens, grounded ONLY in the principles retrieved into your context (reciprocity, framing/anchoring, social proof, scarcity, liking, commitment, loss-aversion). Give sharp, ethical persuasion guidance: how to present the TRUTH so it lands and moves people. Max 3 tight bullets, one line each. HARD GATE: never suggest a false claim or over-promise — persuasion yes, lying no. If the principles do not cover it, say so — never invent.' \
 "$DOKTRIN_GROUNDED"

# inversion lens — method ant (no corpus): what makes this FAIL?
deploy thinking-inversion "$ANT_WASM" "Thinking — Inversion" \
 "Looks at a subject backwards: what would make it fail, and how to avoid that." \
 'You analyze a subject by INVERSION: instead of how to succeed, ask what would make it FAIL, then how to avoid those failures. Give the top 2-3 failure modes and the one move that defuses each. Max 3 tight bullets, one line each, specific to THIS subject. Domain-agnostic.' \
 "$DOKTRIN_PLAIN"

# first-principles lens — method ant: strip to fundamentals
deploy thinking-firstprinciples "$ANT_WASM" "Thinking — First Principles" \
 "Strips a subject to undeniable fundamentals and rebuilds from there." \
 'You analyze a subject from FIRST PRINCIPLES: strip away every assumption until only what is undeniably true remains, then reason UP from those fundamentals. State the 2-3 bedrock truths, then the conclusion they force. Max 3 tight bullets, one line each. Domain-agnostic.' \
 "$DOKTRIN_PLAIN"

# caster — method ant (item 7): pick which lenses are relevant for THIS subject
deploy thinking-caster "$ANT_WASM" "Thinking — Caster" \
 "Given a subject + the bench of lenses, picks the 2-3 most relevant ids." \
 'You receive a SITUATION and a list of available thinking lenses (each as "id: what it is good for"). Choose the 2 or 3 lenses MOST relevant to THIS situation. Output ONLY their ids separated by commas (e.g. "thinking-strategy,thinking-influence"), nothing else — no explanation, no extra text.' \
 "$DOKTRIN_PLAIN"

# Seed the grounded lenses with their brand-neutral corpora (travels with the agent).
if [ -f "$SEEDS/influence.jsonl" ]; then
  cp "$SEEDS/influence.jsonl" "$AGENTS/thinking-influence.fwagent/workspace/seed.jsonl"
  echo "→ seeded thinking-influence ($(wc -l < "$SEEDS/influence.jsonl") patterns staged)"
fi
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

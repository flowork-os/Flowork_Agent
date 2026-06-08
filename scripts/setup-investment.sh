#!/usr/bin/env bash
# setup-investment.sh — deploy the `investment` group: a SEQUENTIAL stock-analyst
# colony WITH EYES. The orchestrator gathers real data (price/fundamentals via the
# market_quote tool, news/scandal via web_search, archives via web_archive) into a
# grounded brief, then each organ reasons on it, then a decision organ returns
# INVEST/WATCH/PASS + sizing + kill-criteria.
#
# Organs (plain "ant" specialists, English, anti-hallucination, brief-grounded):
#   investment-financials — financial health (margins, ROE, cash vs debt)
#   investment-valuation  — cheap/fair/expensive vs value (PE, PB, growth)
#   investment-technical  — trend/timing from price action (candles)
#   investment-governance — integrity (scandal) + political exposure (context-weighted)
#   investment-risk       — the bear case / inversion (what loses money)
#   investment-decision   — synth: one final call + sizing + kill-criteria
#
# IDX-first (a bare symbol defaults to .JK). Reproducible, plug-and-play. The group
# itself is created here as an OWNER action (explicitly directed by the owner).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
ANT_WASM="$ROOT/templates/ant-template/agent.wasm"
GROUP_WASM="$ROOT/templates/investment-group/agent.wasm"

[ -f "$ANT_WASM" ]   || { echo "missing $ANT_WASM"; exit 1; }
[ -f "$GROUP_WASM" ] || { echo "missing $GROUP_WASM (build: cd templates/investment-group && GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .)"; exit 1; }

# deploy_organ <id> <display> <description> <persona> <doktrin>
deploy_organ() {
  local id="$1" disp="$2" desc="$3" persona="$4" doktrin="$5"
  local dst="$AGENTS/$id.fwagent"
  echo "→ deploy organ $id"
  mkdir -p "$dst/workspace"
  cp "$ANT_WASM" "$dst/agent.wasm"
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
 "exposes_rpc":[{"name":"handle_message","description":"Analyze one subject through this organ's lens.","input_schema":{"type":"object","properties":{}}}]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY
  printf '%s' "$persona" > "$dst/workspace/prompt.md"
  printf '%s' "$doktrin" > "$dst/workspace/doktrin.md"
}

DOKTRIN='SACRED RULE (anti-hallucination): reason ONLY from the VERIFIED DATA BRIEF and snippets given to you. NEVER fabricate numbers, prices, names, scandals, or affiliations. If a figure is missing or marked n/a, say so plainly. Cite the figures you use. This is analysis, not financial advice. Respond in the same language as the subject.'

deploy_organ investment-financials "Investment — Financials" \
 "Financial-health analyst: margins, profitability, cash vs debt, from verified numbers." \
 'You are a FINANCIAL HEALTH analyst. From the VERIFIED DATA BRIEF only, judge the company financial health: revenue and growth, margins, profitability (ROE/ROA), and balance-sheet strength (debt-to-equity, free cash flow, cash). State plainly whether the business is financially STRONG, MEDIOCRE, or FRAGILE and why, citing the exact figures. Max 4 tight bullets. If a figure is n/a, say so; never invent one.' \
 "$DOKTRIN"

deploy_organ investment-valuation "Investment — Valuation" \
 "Valuation analyst: cheap/fair/expensive versus value, from the multiples." \
 'You are a VALUATION analyst. From the brief only, judge whether the stock is CHEAP, FAIR, or EXPENSIVE versus its value: weigh PE and forward PE and P/B against profitability (ROE) and growth, plus the street target if present. Remember a great company at a rich price is still a poor investment. Give a one-word verdict (UNDERVALUED / FAIR / OVERVALUED) with reasoning tied to the numbers. Max 4 tight bullets. Never invent figures.' \
 "$DOKTRIN"

deploy_organ investment-technical "Investment — Technical" \
 "Technical analyst: trend and timing from price action." \
 'You are a TECHNICAL analyst. From the PRICE ACTION line only (window return, range, last close), read the TREND (up/down/sideways), momentum, and where the last close sits in its range (near the high = extended; near the low = possible support). This informs TIMING and entry, not whether to own it. State explicitly that without intraday or indicator data this is a coarse read. Max 4 tight bullets. Never invent levels the numbers do not support.' \
 "$DOKTRIN"

deploy_organ investment-governance "Investment — Governance" \
 "Governance & integrity analyst: scandal check + political exposure (context-weighted)." \
 'You are a GOVERNANCE and INTEGRITY analyst. From the GOVERNANCE INTEL snippets ONLY, assess two things. (a) INTEGRITY: any credible scandal, fraud, lawsuit, or red flag around the company or its leadership — cite the source; an archived or scrubbed article is itself a warning sign. (b) POLITICAL EXPOSURE: if leadership is tied to a political party or figure, treat it as a CONTEXT-WEIGHTED risk, not an automatic penalty — it can be a MOAT (protected contracts, licenses) OR a LIABILITY (falls with the patron, regulatory/corruption risk) — say which way it likely cuts and why. Report ONLY what the sourced snippets support. If there is no evidence, say "no adverse findings in the retrieved sources" — never accuse anyone without a source. Max 4 tight bullets.' \
 "$DOKTRIN"

deploy_organ investment-risk "Investment — Risk" \
 "Risk/inversion analyst: the bear case — what loses money." \
 'You are a RISK and INVERSION analyst. From the brief, make the sharp BEAR case: what could make this LOSE money — valuation compression, margin or competitive pressure, debt load, governance or political risk, sector/macro headwinds, thin liquidity. Invert the question: what has to go WRONG for this to fail, and how plausible is it. Max 4 tight bullets, citing figures where relevant. Never fabricate.' \
 "$DOKTRIN"

deploy_organ investment-decision "Investment — Decision" \
 "Decision synthesizer: one final call + sizing + kill-criteria." \
 'You are the INVESTMENT DECISION synthesizer. You receive the subject, the verified data brief, and the analyst findings (financials, valuation, technical, governance, risk). Weigh them fairly into ONE decision — do not blindly favor any side. Output, in this order: a clear FINAL CALL (INVEST / WATCH / PASS); two to three sentences of integrated reasoning tied to the numbers; a suggested POSITION SIZE / how to scale in; and the KILL-CRITERIA (the specific things that would flip your call). End with the line: This is analysis, not financial advice. OUTPUT FORMAT: plain text for a chat app (Telegram) — NO markdown: no "#" headers, no "**" bold, no tables. Short paragraphs, simple dash bullets, a blank line between sections.' \
 "$DOKTRIN"

# === the GROUP orchestrator ===========================================
GID="investment"
DST="$AGENTS/$GID.fwagent"
echo "→ deploy group $GID"
mkdir -p "$DST/workspace"
cp "$GROUP_WASM" "$DST/agent.wasm"
python3 - "$GID" "$DST/manifest.json" <<'PY'
import json,sys
gid,out=sys.argv[1:3]
json.dump({
 "id":gid,"version":"1.0.0","kind":"agent","display_name":"Investment Team",
 "description":"Stock-analyst colony with eyes: gather real data (price, fundamentals, news), analyze through financials/valuation/technical/governance/risk organs, then decide INVEST/WATCH/PASS.",
 "min_kernel_version":"0.1.0","abi_version":1,"author":"@flowork-os","license":"MIT",
 "entry":"agent.wasm","memory_max_mb":16,"timeout_call_ms":120000,
 "capabilities_required":[
   "net:fetch:http://127.0.0.1:1987/api/kernel/call",
   "net:fetch:*",
   "state:read","state:write","time:read"],
 "exposes_rpc":[{"name":"handle_message","description":"Run the investment pipeline on one subject/ticker.","input_schema":{"type":"object","properties":{}}}]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY

# loket.json — declares the GrantOwner cap the orchestrator consumes (tool.run, the
# bridge to market_quote/web_search/web_archive). Installing the .fwagent IS the
# owner grant, exactly like mr-flow-next.
python3 - "$GID" "$DST/loket.json" <<'PY'
import json,sys
gid,out=sys.argv[1:3]
json.dump({
 "id":gid,"kind":"agent","name":"Investment Team","version":"0.1.0",
 "abi_version":"1","entry":"handle",
 "consumes":["tool.run","time.now"]
}, open(out,"w"), indent=2, ensure_ascii=False); open(out,"a").write("\n")
PY

# Mark group=1 + roster into its OWN loket store (menu-visible + menu-editable).
GLOKET="$DST/workspace/loket.db"
set_kv() { sqlite3 "$1" "PRAGMA busy_timeout=30000;
  CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL DEFAULT '');
  INSERT INTO kv(k,v) VALUES('$2','$3') ON CONFLICT(k) DO UPDATE SET v=excluded.v;" >/dev/null; }
set_kv "$GLOKET" group "1"
set_kv "$GLOKET" display_name "Investment Team"
set_kv "$GLOKET" members "investment-financials,investment-valuation,investment-technical,investment-governance,investment-risk"
set_kv "$GLOKET" synthesizer "investment-decision"
set_kv "$GLOKET" task "Gather verified market data, analyze through each organ, then decide INVEST/WATCH/PASS with sizing and kill-criteria."
echo "→ marked group=1 + roster in loket store"

# Register /invest → investment in Mr.Flow's allowlist (replaces the old analis-tim
# pointer if present). Format "id|command|desc;…".
MFLOKET="$AGENTS/mr-flow-next.fwagent/workspace/loket.db"
ENTRY="$GID|invest|Investment team: gather data, analyze (financials, valuation, technical, governance, risk), decide INVEST/WATCH/PASS"
if [ -f "$MFLOKET" ]; then
  cur=$(sqlite3 "$MFLOKET" "SELECT v FROM kv WHERE k='groups';" 2>/dev/null || true)
  case "$cur" in
    *"$GID|"*) echo "→ mr-flow already knows group $GID";;
    *) sqlite3 "$MFLOKET" "CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL DEFAULT '');
                           INSERT INTO kv(k,v) VALUES('groups','${cur:+$cur; }$ENTRY')
                           ON CONFLICT(k) DO UPDATE SET v=excluded.v;"
       echo "→ registered group $GID (/invest) in mr-flow allowlist";;
  esac
else
  echo "ⓘ mr-flow loket store missing — boot mr-flow first, then re-run."
fi

echo "✅ investment group deployed + wired. Restart, then test: /invest BBCA"

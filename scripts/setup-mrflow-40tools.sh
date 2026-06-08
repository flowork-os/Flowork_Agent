#!/usr/bin/env bash
# setup-mrflow-40tools.sh — make mr-flow a CAPABLE single agent: subscribe it to the
# curated 40 first-class tools (owner principle 2026-06-08: mr-flow holds 40 mature
# tools, groups only for special pasukan-semut tasks). ISOLATED: touches ONLY
# mr-flow's own state.db (tool_subscriptions). Idempotent. Re-run to restore.
# No hardcoded paths — resolves the agents dir from env / HOME.
set -euo pipefail
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
DB="$AGENTS/mr-flow-next.fwagent/workspace/state.db"
[ -f "$DB" ] || { echo "mr-flow state.db not found at $DB — boot mr-flow first"; exit 1; }
TOOLS=(
  file_read file_write edit file_list glob grep codemap_search
  shell git system_power app_open system_health
  web_search webfetch web_archive html_extract pdf_read
  brain_search brain_add memory_get memory_set fact_recall fact_write kv_get kv_set
  todo plan_read plan_write goal_done scheduler_schedule_add scheduler_list
  skill skill_search tool_search capabilities_list
  market_quote scanner_quick_scan code_scan
  telegram_send askuser
)
sqlite3 "$DB" "CREATE TABLE IF NOT EXISTS tool_subscriptions(tool_name TEXT PRIMARY KEY, subscribed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, source TEXT NOT NULL DEFAULT 'manual', config TEXT NOT NULL DEFAULT '{}');" >/dev/null
for t in "${TOOLS[@]}"; do
  sqlite3 "$DB" "INSERT INTO tool_subscriptions(tool_name,source,config) VALUES('$t','curated40','{}') ON CONFLICT(tool_name) DO UPDATE SET source='curated40';" >/dev/null
done
echo "subscribed ${#TOOLS[@]} tools to mr-flow (source=curated40)"
sqlite3 "$DB" "SELECT count(*) FROM tool_subscriptions WHERE source='curated40';"

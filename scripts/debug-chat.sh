#!/usr/bin/env bash
# debug-chat.sh — send a message to mr-flow through the EXACT path Telegram uses
# (the agent's handle_message), so a debug reply is identical to what a real user
# sees. Use this for QC instead of curling tool endpoints directly (a direct tool
# call bypasses the agent pipeline and gives misleading results).
#   usage: scripts/debug-chat.sh "your message here" [chat_id]
# No hardcoded host: override with FLOWORK_GUI_URL (default 127.0.0.1:1987).
set -euo pipefail
URL="${FLOWORK_GUI_URL:-http://127.0.0.1:1987}"
MSG="${1:?usage: debug-chat.sh \"message\" [chat_id]}"
CHAT="${2:-debug}"
BODY=$(python3 -c 'import json,sys;print(json.dumps({"plugin":"mr-flow-next","function":"handle_message","args":{"text":sys.argv[1],"chat_id":sys.argv[2]}}))' "$MSG" "$CHAT")
curl -s -m 115 -X POST "$URL/api/kernel/rpc" -H 'Content-Type: application/json' -d "$BODY" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);r=d.get("result",d);print(r.get("reply") or json.dumps(r,ensure_ascii=False))'

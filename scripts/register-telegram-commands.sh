#!/usr/bin/env bash
# register-telegram-commands.sh — register the bot's slash-command menu on Telegram
# (setMyCommands), so commands show up in the "/" menu with autocomplete. One-time:
# Telegram stores it bot-side. Re-run after changing the command list.
#
# Reads the bot token from the telegram-channel agent's own loket store (never hardcoded).
set -euo pipefail
AGENTS="${FLOWORK_AGENTS_DIR:-$HOME/.flowork/agents}"
DB="$AGENTS/telegram-channel.fwagent/workspace/state.db"
[ -f "$DB" ] || { echo "telegram-channel store not found ($DB) — boot it first"; exit 1; }
TOK="$(sqlite3 "$DB" "SELECT v FROM secrets WHERE lower(k) LIKE '%token%' OR lower(k) LIKE '%tg_bot%';" 2>/dev/null | head -1)"
[ -n "$TOK" ] || { echo "no bot token in telegram-channel store"; exit 1; }

CMDS='{"commands":[
  {"command":"thinking","description":"Mikir bareng tim thinking (strategi, perbaikan, persuasi, dll) — multi-turn"}
]}'
code=$(curl -s --max-time 20 -X POST "https://api.telegram.org/bot${TOK}/setMyCommands" \
  -H 'Content-Type: application/json' -d "$CMDS" -o /tmp/_tgcmd.json -w "%{http_code}")
echo "setMyCommands HTTP $code"
grep -q '"ok":true' /tmp/_tgcmd.json && echo "✅ commands registered" || { echo "❌ failed:"; cat /tmp/_tgcmd.json; exit 1; }
rm -f /tmp/_tgcmd.json
echo "ⓘ users now see /thinking in Telegram. Usage: /thinking <masalah lo>"

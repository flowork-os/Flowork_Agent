#!/usr/bin/env bash
# setup-operator.sh — bikin agent "operator-komputer" (reproducible).
#
# Operator = warga yang ngendaliin status DAYA komputer host (shutdown/reboot/
# suspend/lock/logout) via tool `system_power`. Beda dari warga biasa: dia
# pegang capability `exec:power` yang warga lain TIDAK punya. Pola sama crew
# (setup-saham-crew.sh): generated + gitignored, reproduce lewat script ini.
#
# SAFETY: aksi daya real cuma jalan kalau host di-ARM (env FLOWORK_POWER_ARMED=1).
# Tanpa itu, tool dry-run (resolve command + audit, TANPA eksekusi). Aman default.
#
# Pakai:
#   ./scripts/setup-operator.sh
#
# Setelah ini: ./restart.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

ID="operator-komputer"
SRC="agents/$ID"
STAGED="$HOME/.flowork/agents/$ID.fwagent"

echo "→ Bikin operator agent: $ID"

# ── clean reproduce: buang scaffold lama (state.db terisolasi ga kebawa) ─────
rm -rf "$SRC" "$STAGED"

# ── scaffold engine dari template mr-flow + build wasm ──────────────────────
./scripts/spawn-agent.sh "$ID" --from mr-flow

# ── patch manifest: cap exec:power (buang yg ga perlu) + description ────────
python3 - "$SRC/manifest.json" <<'PY'
import json, sys
p = sys.argv[1]
m = json.load(open(p))
m["description"] = ("Operator agent: controls the host computer's power state "
                    "(shutdown/reboot/suspend/lock/logout) on explicit owner "
                    "command. Same Mr.Flow engine; holds the exec:power "
                    "capability that normal agents do not. Real power actions "
                    "only fire when the host is ARMED (FLOWORK_POWER_ARMED).")
m["capabilities_required"] = [
    "net:fetch:https://api.telegram.org",
    "net:fetch:http://127.0.0.1:2402/v1/chat/completions",
    "net:fetch:http://127.0.0.1:1987/api/agents/self-prompt/render",
    "net:fetch:http://127.0.0.1:1987/api/agents/interactions",
    "net:fetch:http://127.0.0.1:1987/api/agents/tools/specs",
    "net:fetch:http://127.0.0.1:1987/api/agents/tools/run",
    "state:read",
    "state:write",
    "time:read",
    "exec:power",
]
json.dump(m, open(p, "w"), indent=2, ensure_ascii=False)
open(p, "a").write("\n")
print("✓ manifest: caps trimmed + exec:power added")
PY

# re-stage manifest ke runtime dir (broker baca cap dari sini)
cp "$SRC/manifest.json" "$STAGED/manifest.json"
echo "✓ manifest re-staged ke $STAGED"

# ── persona + subscribe tool system_power (ke state.db) ─────────────────────
PERSONA="Lo OPERATOR KOMPUTER. Tugas lo ngendaliin status daya komputer owner: matiin (shutdown), restart (reboot), tidur (suspend), kunci layar (lock), atau logout — HANYA kalau owner minta eksplisit. Pakai tool system_power. SELALU konfirmasi dulu ke owner sebelum shutdown/reboot ('yakin mau matiin komputer?'). Kasih delay_seconds wajar (default 10) biar ada jendela batal; kalau owner bilang batal, panggil system_power action=cancel. Jawab ringkas, Bahasa Indonesia. JANGAN jalanin perintah daya tanpa permintaan jelas."
go run ./cmd/agent-config "$SRC/workspace/state.db" "$PERSONA" "system_power"

# ── register Category Task "Operasi Komputer" (wadah operator agents) ────────
# Container yang bakal nambah crew ke depan (power → app → file → proses → dll).
# Sekarang crew = operator-komputer doang; synthesizer = dia juga.
# Register lewat jalur GUI (POST /api/taskflow/category). Server :1987 harus up.
BASE="http://127.0.0.1:1987"
if curl -sS -m 4 -o /dev/null "$BASE/api/system/health" 2>/dev/null; then
  echo "── register kategori (jalur GUI POST /category) ──"
  curl -sS -m 8 -X POST "$BASE/api/taskflow/category" -H "Content-Type: application/json" -d '{
    "id":"operasi-komputer","name":"Operasi Komputer","icon":"🖥️",
    "trigger_hint":"kendaliin komputer (matiin/restart/tidur/kunci/logout)",
    "synthesizer":"operator-komputer","enabled":true,
    "crew":[
      {"agent_id":"operator-komputer","role_label":"operator daya & sistem (shutdown/reboot/suspend/lock/logout)"}
    ]
  }' && echo " ✓ kategori 'operasi-komputer' terdaftar"
else
  echo "ⓘ server :1987 belum jalan — skip register kategori."
  echo "  Jalanin ./restart.sh dulu, lalu re-run script ini (idempotent) buat daftarin kategori."
fi

cat <<EOF

✅ Agent '$ID' siap.
  • Tool      : system_power (cap exec:power) — cuma agent ini yg punya.
  • Default   : DRY-RUN (aman). Buat eksekusi shutdown BENERAN, jalanin host
                dengan env: FLOWORK_POWER_ARMED=1 ./restart.sh
  • Langkah   : ./restart.sh  → operator-komputer ke-load.

Cara nugasin: kasih dia Telegram bot token sendiri (Setting popup) lalu chat,
ATAU via scheduler/MCP. (Reachability detail — lihat changelog.)
EOF

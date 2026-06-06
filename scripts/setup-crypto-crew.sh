#!/usr/bin/env bash
# setup-crypto-crew.sh — FASE 6: crew CRYPTO reproducible (demo generalize).
#
# Buktiin "nambah task gampang": spawn crew dari template + register kategori
# lewat API (jalur GUI). Mirror setup-saham-crew.sh. Crew gitignored (generated).
#
# Pakai:  ./scripts/setup-crypto-crew.sh   (server :1987 harus jalan buat register)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"; cd "$ROOT"
RT="web_search,html_extract,web_archive,pdf_read"
BASE="${FLOWORK_SELF_URL:-http://127.0.0.1:1987}"

spawn_build() { # <id> <netfetch1/0>
  [ -d "agents/$1" ] || ./scripts/spawn-agent.sh "$1" --no-build >/dev/null
  if [ "$2" = "1" ]; then NETID="$1" python3 - <<'PY'
import json,os
p=f"agents/{os.environ['NETID']}/manifest.json";m=json.load(open(p));c=m["capabilities_required"]
if "net:fetch:*" not in c:c.append("net:fetch:*");json.dump(m,open(p,"w"),indent=2,ensure_ascii=False);open(p,"a").write("\n")
PY
  fi
  ./scripts/build-agent.sh "$1" >/dev/null; echo "  built $1"
}
cfg(){ go run ./cmd/agent-config "agents/$1/workspace/state.db" "$2" "$3" >/dev/null; echo "  cfg $1"; }

echo "── spawn + build crypto crew ──"
for a in crypto-fundamental crypto-onchain crypto-sentimen; do spawn_build "$a" 1; done
spawn_build crypto-sinteser 0

echo "── persona + subs ──"
cfg crypto-fundamental "Lo analis FUNDAMENTAL crypto. Fokus: tokenomics (supply, distribusi, unlock), tim/backer, use-case nyata, roadmap, valuasi (market cap, FDV). Cari data REAL (web_search/html_extract) — sebut sumber, JANGAN ngarang. Ringkas, Bahasa Indonesia." "$RT"
cfg crypto-onchain "Lo analis ON-CHAIN & KEAMANAN crypto. Fokus: distribusi holder (whale), likuiditas, status audit, red-flag scam/rug (kontrak, lock LP). Cari data REAL — sebut sumber, JANGAN ngarang. Ringkas, Bahasa Indonesia." "$RT"
cfg crypto-sentimen "Lo analis SENTIMEN crypto. Fokus: komunitas, sosmed (X/Reddit), berita, hype vs FUD, momentum naratif. Cari data REAL — sebut sumber, JANGAN ngarang. Ringkas, Bahasa Indonesia." "$RT"
cfg crypto-sinteser "Lo SYNTHESIZER keputusan crypto. Baca analis (fundamental/on-chain/sentimen) via file_read, gabungin → keputusan TEGAS: BUY/HOLD/AVOID + ALASAN (sebut analis mana) + RISIKO. Berbasis data analis, JANGAN ngarang. Bahasa Indonesia." ""

echo "── register kategori (jalur GUI POST /category) ──"
echo "  (jalanin ./restart.sh dulu kalau agent baru di-spawn, lalu:)"
curl -sS -m 8 -X POST "$BASE/api/taskflow/category" -H "Content-Type: application/json" -d '{
  "id":"crypto","name":"Analisa Crypto","icon":"🪙","synthesizer":"crypto-sinteser","enabled":true,
  "crew":[
    {"agent_id":"crypto-fundamental","role_label":"analis fundamental (tokenomics, tim, use-case)"},
    {"agent_id":"crypto-onchain","role_label":"analis on-chain & keamanan (holder, likuiditas, scam-check)"},
    {"agent_id":"crypto-sentimen","role_label":"analis sentimen (komunitas, berita, hype)"}
  ]}' && echo

echo "✅ Crew CRYPTO siap. ./restart.sh lalu: curl -X POST '$BASE/api/taskflow/run?category=crypto&subject=SOL'"

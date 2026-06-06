#!/usr/bin/env bash
# setup-saham-crew.sh — FASE 4: bikin crew SAHAM reproducible dari template.
#
# Spawn 4 warga (3 analis + 1 synthesizer) dari template mr-flow, patch caps,
# build wasm, set persona + tool subscription. Idempotent-ish: skip spawn kalau
# folder udah ada. Persona = source of truth crew (state.db gitignored).
#
# Pakai:  ./scripts/setup-saham-crew.sh   (lalu ./restart.sh biar ke-load)
#
# Crew (lihat internal/taskflow/taskflow.go — hardcoded Phase 1):
#   saham-fundamental · saham-keuangan · saham-teknikal  → analis (web tools)
#   saham-sinteser                                       → synthesizer (baca file)

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"
cd "$ROOT"

# Analis butuh net:fetch:* (web_search/html_extract host-side) + tools riset.
# Synthesizer ENGGAK (biar ga ngarang web — cuma baca file analis).
ANALYSTS=(saham-fundamental saham-keuangan saham-teknikal)
RESEARCH_TOOLS="web_search,html_extract,web_archive,pdf_read"

spawn_one() {
  local id="$1" netfetch="$2"
  if [ ! -d "agents/$id" ]; then
    ./scripts/spawn-agent.sh "$id" --no-build >/dev/null
    echo "  spawned $id"
  else
    echo "  $id sudah ada (skip spawn)"
  fi
  if [ "$netfetch" = "1" ]; then
    NETID="$id" python3 - <<'PY'
import json, os
p = f"agents/{os.environ['NETID']}/manifest.json"
m = json.load(open(p)); c = m["capabilities_required"]
if "net:fetch:*" not in c:
    c.append("net:fetch:*")
    json.dump(m, open(p, "w"), indent=2, ensure_ascii=False); open(p, "a").write("\n")
PY
  fi
  ./scripts/build-agent.sh "$id" >/dev/null
  echo "  built $id"
}

cfg() { # <id> <persona> <tools-csv>
  go run ./cmd/agent-config "agents/$1/workspace/state.db" "$2" "$3" >/dev/null
  echo "  configured $1"
}

echo "── spawn + build crew ──"
for a in "${ANALYSTS[@]}"; do spawn_one "$a" 1; done
spawn_one "saham-sinteser" 0

echo "── set persona + subscriptions ──"
cfg saham-fundamental \
  "Lo analis FUNDAMENTAL saham profesional. Tugas: NILAI bisnis perusahaan (model bisnis, posisi industri, prospek, valuasi PER/PBV). Pake tools riset (web_search/html_extract) cari data REAL — JANGAN ngarang angka, sebut sumber. Ringkas poin-poin, Bahasa Indonesia. Fokus fundamental, bukan teknikal." \
  "$RESEARCH_TOOLS"
cfg saham-keuangan \
  "Lo analis LAPORAN KEUANGAN saham. Fokus: revenue, laba bersih, margin, ROE/ROA, DER/utang, arus kas, pertumbuhan YoY. Cari laporan keuangan REAL (web_search/html_extract) — sebut ANGKA + sumber, JANGAN ngarang. Ringkas poin-poin, Bahasa Indonesia. Cuma keuangan, bukan teknikal/chart." \
  "$RESEARCH_TOOLS"
cfg saham-teknikal \
  "Lo analis TEKNIKAL saham. Fokus: tren harga, support/resistance, momentum, volume, moving average. Cari data harga REAL (web_search/html_extract) — sebut level + sumber, JANGAN ngarang. Ringkas poin-poin, Bahasa Indonesia. Cuma teknikal, bukan fundamental." \
  "$RESEARCH_TOOLS"
cfg saham-sinteser \
  "Lo SYNTHESIZER pengambil keputusan investasi saham. Baca hasil analis (fundamental/keuangan/teknikal) via tool file_read, GABUNGIN, kasih 1 keputusan TEGAS: BUY/HOLD/AVOID + ALASAN (sebut dari analis mana) + RISIKO. Berbasis DATA analis doang, JANGAN ngarang/nambah data sendiri. Bahasa Indonesia." \
  ""

echo
echo "✅ Crew SAHAM siap. Jalanin ./restart.sh biar ke-load, terus:"
echo "   curl -X POST 'http://127.0.0.1:1987/api/taskflow/run?category=saham&subject=BBCA'"

#!/usr/bin/env bash
# brain-export.sh — SNAPSHOT brain DB jadi 1 file KONSISTEN (WAL checkpoint + .backup).
#
# PRINSIP: brain = pengalaman hidup user (Flowork "keabadian"). Tool ini WAJIB:
#   - READ-ONLY ke brain asli → NOL ubah/hapus memori (cuma baca → tulis ke file BARU).
#   - hasil = 1 file sqlite konsisten (NO -wal/-shm) → aman di-backup / di-ship portable
#     (anti "WAL-poison": copy mentah DB live bikin SQLite replay state basi di mesin tujuan).
#
# Pakai:
#   ./brain-export.sh <src-brain.sqlite> <dest.sqlite>            # backup PENUH (semua memori)
#   ./brain-export.sh <src-brain.sqlite> <dest.sqlite> --public  # cuma memori visibility='public'
#                                                                 # (butuh kolom visibility — B2)
set -euo pipefail
SRC="${1:-}"; DST="${2:-}"; MODE="${3:-full}"
[ -f "$SRC" ] || { echo "src brain ga ada: $SRC"; exit 2; }
[ -n "$DST" ] || { echo "pakai: brain-export.sh <src> <dest> [--public]"; exit 2; }
command -v sqlite3 >/dev/null || { echo "butuh sqlite3"; exit 2; }

mkdir -p "$(dirname "$DST")"; rm -f "$DST" "$DST-wal" "$DST-shm"

# 1) Snapshot konsisten via .backup (read-only; merge WAL ke salinan, brain asli ga disentuh).
echo "[brain-export] snapshot konsisten → $DST"
sqlite3 "file:$SRC?mode=ro" ".timeout 10000" ".backup '$DST'"

# 2) Pastiin salinan ga bawa WAL (single-file, journal DELETE = portable bersih).
sqlite3 "$DST" "PRAGMA journal_mode=DELETE; VACUUM;" >/dev/null
rm -f "$DST-wal" "$DST-shm"

# 3) (opsional) FILTER PUBLIC — cuma kalau kolom visibility ADA (B2). Additive: kalau kolom belum
#    ada, skip filter (backup penuh) — JANGAN error, JANGAN hapus apa-apa di src.
if [ "$MODE" = "--public" ]; then
  TABLES="$(sqlite3 "$DST" "SELECT name FROM sqlite_master WHERE type='table';" 2>/dev/null || true)"
  filtered=0
  for t in $TABLES; do
    if sqlite3 "$DST" "PRAGMA table_info($t);" 2>/dev/null | grep -q '|visibility|'; then
      # buang baris privat DI SALINAN (src ga kesentuh — ini file backup terpisah).
      sqlite3 "$DST" "DELETE FROM $t WHERE visibility IS NOT NULL AND visibility!='public';" 2>/dev/null && filtered=$((filtered+1))
    fi
  done
  sqlite3 "$DST" "VACUUM;" >/dev/null
  if [ "$filtered" -eq 0 ]; then
    echo "[brain-export] WARN: kolom 'visibility' belum ada (B2 belum dipasang) → backup PENUH, bukan public-only."
  else
    echo "[brain-export] filter public di $filtered tabel."
  fi
fi

SZ="$(du -h "$DST" | cut -f1)"
echo "[brain-export] ✅ selesai: $DST ($SZ, konsisten, no-WAL). Brain asli UTUH (read-only)."

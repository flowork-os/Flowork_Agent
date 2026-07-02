# lines-changed — metrik "cost" baris-diubah per-sesi

Lacak baris ditambah/dihapus (added/removed) tiap agent ngedit file → kartu
aktivitas di GUI (kayak +X/−Y Claude Code). Roadmap GUI "Cost lines-changed".

## Arsitektur (self-contained agent-side, NOL frozen)
- `agent/internal/tools/builtins/lines_changed_ext.go` — SIBLING deletable. Interceptor
  (pola file_checkpoint) `Before` file_write/edit → hitung diff → akumulasi in-memory
  per-agent (`editStats`). Rekonstruksi isi baru: file_write=`content`, edit=apply
  replace ke isi lama. Diff added/removed via **LCS** (akurat; fallback net-count
  file >4000 baris). Best-effort NON-blocking (metrik ga boleh ganggu tulisan).
- `agent/feature_edits_stats.go` — SIBLING feature, endpoint `/api/edits/stats`
  → `{total:{added,removed,edits,files}, byAgent, scope:"since-boot"}`.
- GUI: `web/tabs/codemap.js` toolbar pill (`#cm-edits`) fetch endpoint → "✏️ edits
  this session: +A / −R · N files". i18n `codemap.edits_*` (en+id).

## Scope
- "per-sesi" = since-boot (in-memory, reset tiap restart). Cukup buat indikator
  aktivitas; bukan billing. notebook_edit SENGAJA ga dihitung (JSON per-cell, bukan LOC).

## Freeze
FROZEN 2026-07-02 (seizin owner, stabil+live). Behavior stabil dikunci; hapus file butuh unlock.

## QC
build agent hijau · vet hijau · unit test (LCS diff 6 kasus akurat, akumulasi
interceptor +files, non-edit tool diabaikan) PASS · full builtins nol regresi ·
TestKernelFreeze utuh · GUI pill verified via chrome-headless screenshot.

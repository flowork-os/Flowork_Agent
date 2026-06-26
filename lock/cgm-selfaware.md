# CGM SELF-AWARE — codemap → graph + orphan backfill (agent Cognitive Graph)

> Tambahan ke CGM agent (lihat `lock/CognitiveGraph.md` buat dasar CGM). Owner: Mr.Dev. 2026-06-26.
> Tujuan: agent SADAR peta kode-dirinya + graph nyambung penuh (no node ngambang).

## M2 — codemap STRUKTUR → CGM
`graph_autosync.go` (cycle CGM, ticker 1mnt) panggil `store.LinkCodemapToGraph(scope)` → projeksi
`codemap_files` + `codemap_file_edges` jadi node type `code`/`code_layer` + edge `depends_on`/`part_of`.
Hasil: agent punya peta kode-dirinya di CGM (246 code-node terverifikasi). Idempotent (upsert).
- Switch GUI: **`FLOWORK_CGM_CODEMAP`** (default ON). OFF = skip.
- Sumber struktur: codemap deterministik (bukan LLM). Makna enrich (summary/domain/role) ada di
  tabel `codemap_semantic` (lihat `lock/codemap-enrich.md`); attach makna ke node = SISA opsional.

## Bagian 4 — orphan backfill → hub brain-root
Node projeksi sumber (skill/constitution/edu/knowledge/code) di-`put` sbg NODE tanpa edge = ORPHAN
(ngambang di viz). `BackfillOrphansToHub(scope)` (file `internal/agentdb/cognitive_orphan.go`):
pastikan hub `<scope>/concept/brain-root` ada → link semua orphan via `member_of` (strength 0.5).
Dipanggil di `graph_autosync` cycle. Verified: orphan **208 → 0** (total 1256 node nyambung).
- Switch GUI: **`FLOWORK_CGM_ORPHAN_BACKFILL`** (default ON). OFF = biarin ngambang.
- Pola sama `lock/CognitiveGraph.md` §2 FIX-2 (hub), tapi otomatis tiap cycle (bukan one-time manual).

## FILE & FREEZE
- `agent/graph_autosync.go` (FROZEN, re-hash tiap edit) — wiring M2 + orphan di cycle.
- `agent/internal/agentdb/cognitive_orphan.go` (FROZEN) — BackfillOrphansToHub.
- `LinkCodemapToGraph` ada di `cognitive_codemap.go` (FROZEN brain-core, cuma DIPANGGIL).
- Switch di `registry.go` (non-frozen extension point).

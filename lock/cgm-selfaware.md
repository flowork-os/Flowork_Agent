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

## #2 — enrich (makna) → code-node (2026-06-26)
`AttachCodemapSemanticToGraph(scope)` (`internal/agentdb/cognitive_codemap_semantic.go`, FROZEN):
join `codemap_semantic`+`codemap_files` → re-upsert code-node `<scope>/codemap/<path>` dgn
`Why`=summary + domain/role di properties. Dipanggil di graph_autosync (switch FLOWORK_CGM_CODEMAP).
Hasil: `graph_recall` nyurfacing "file ini ngapain". Verified: code-node-with-summary 0→63 (= enrich rows).

## #4 — dead-letter task → graph (2026-06-26)
`SyncDeadLettersToGraph(scope,limit)` (`internal/agentdb/cognitive_deadletter.go`, FROZEN): task
gagal permanen (`agent_runs.state='error'`) → node type `dead_letter` + Why=error + edge member_of
→ brain-root. Switch FLOWORK_CGM_DEADLETTER. Agent jadi SADAR kegagalan & bisa graph_recall/belajar.
Verified: inject error-row uji → node `dead_letter` ke-projeksi (label+why+edge), lalu artefak uji dibersihin.

## EXTENSION SEAM — RegisterGraphProjection (2026-06-26, nutup gap audit)
**Akar:** 3 proyeksi di atas (codemap/dead-letter/orphan) dipanggil INLINE di `SyncSourcesToGraph`
(FROZEN). Nambah SUMBER proyeksi BARU = kepaksa buka file frozen (langgar prinsip evolusi). Skill
(`RegisterSkillProvider`) & instinct (`RegisterInstinctSelector`) udah punya registry-seam; graph
projection BELUM → ditutup.
**Fix (cabut-akar):** file NON-frozen `agent/graph_autosync_ext.go` = registry `RegisterGraphProjection`
(switch-aware + fails-open). Dispatcher frozen di-hook 1 baris (`runExtraGraphProjections(ctx,store,scope)`
di akhir `SyncSourcesToGraph`). Sekali hook, selamanya plug-and-play.
**Nambah proyeksi baru (zero edit frozen):** bikin file sibling `agent/graph_proj_xxx.go` →
`func init(){ RegisterGraphProjection(GraphProjection{Name,Switch:"FLOWORK_CGM_XXX",Run:func(ctx,store,scope)(int,error)}) }`
\+ tambah entri switch di `internal/fwswitch/registry.go` → muncul GUI. Run WAJIB idempotent + fails-open.
**Test:** `TestRegisterGraphProjection` / `*SwitchGate` / `*FailsOpen` (package main) PASS. Re-freeze
`graph_autosync.go` PASS (`TestKernelFreeze`).

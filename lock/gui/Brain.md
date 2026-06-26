# Brain

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com
> Dok tab GUI Flowork Router (:2402). Standar freeze: lock/frozen-core.md.

## Fungsi
Pusat **ingatan abadi** Flowork — knowledge base semantik yang membuat agent berevolusi tanpa lupa konteks. Tab ini punya 9 sub-tab: Overview (status), Search (cari by-makna), Add Knowledge (drawer + bulk), Constitution (aturan sakral + governance amandemen), Typed Memory (memory berkategori), Personas, Registry (skill komunitas), Instincts (pola perilaku FLowork), Knowledge Graph (DreamGraph: node/edge editor + cognitive digestion).

## Endpoint (router/routes.go, ~35 route /api/brain*)
- **Status/Config**: `GET /api/brain/status`, `GET|PUT /api/brain/config`, `POST /api/brain/test` (uji semantic search) → handlers_brain.go.
- **Drawer/Search**: `POST|PUT|DELETE /api/brain/drawer`, `GET /api/brain/search-drawers`, `/by-type`, `/personas`, `/init` → handlers_brain_views.go.
- **Wing / Knowledge Graph**: `GET /api/brain/wing` (paginasi drawer per-wing; `wing=cognitive_graph_node|edge` POST/DELETE = editor graph manual; `cognitive_graph_all|stats` baca graph) → **handlers_brain_wing.go**.
- **Graph sync**: `POST /api/brain/graph/sync` → `dreamGraphSyncHandler` (dreamgraph_autosync.go) — mirror instinct/knowledge → cognitive graph (additive, mirror-only). Tombol GUI **"Run Dream Mode"** memanggil endpoint INI.
- **Constitution & governance** (editionGate): `/constitution`, `/constitution/propose|proposals|vote`, `/constitution/amend|amendments|amend/vote` → handlers_brain.go, handlers_brain_proposals.go, handlers_brain_amend.go.
- **Ingest/Learning**: `/ingest/run|submit|batch`, `/contributions`, `/contributions/ingest` → handlers_brain_ingest.go.
- **Typed memory**: `/by-type`, `/mem-types` → handlers_brain_memtypes.go.
- **Skills**: `/skills/list`, `/skills/get` → handlers_brain_skills.go.
- **Tool patterns**: `/tool-patterns/learn`, `/tool-patterns` → handlers_brain_tools.go.
- **Models**: `/models`, `/models/get` → handlers_brain_models.go.
- **Safety**: `/quality/check`, `/pii/strip`, `/injection/check`, `/rescore` → handlers_brain_quality.go, _pii.go, _injection.go, _rescore.go.
- **Tracker keamanan**: `/immune/add`, `/pentest/add|delete|list` → **handlers_pentest.go** (router = single-writer brain; proses luar/scanner sync ke sini, bukan tembak DB langsung).
- **Instincts/Mistakes**: `/instincts` (handlers_brain_instincts.go), `/mistakes` (handlers_brain_mistakes.go).

## Logic / Alur
- **Semantic recall**: query → embed bge-m3 (provider lokal) → cosine similarity di `vecindex.Index`; fallback **FTS5** kalau index kosong. Threshold default 0.45 (`FLOWORK_SEARCH_MINSCORE`, dikelola fwswitch).
- **Drawer**: dedup content-hash; soft-delete; mem_type berkategori (Phase 2 typed system).
- **Knowledge Graph**: tabel `cognitive_nodes` + `cognitive_edges`. Edit manual node/edge via `wing` auto-panggil `SyncGraphToRAG()`. `dreamGraphSyncHandler` = jalur AKTIF & SEHAT (live: ~326 node / ~325 edge).
- **Governance**: constitution = aturan sakral; ubah hanya lewat propose→vote→amend (editionGate: FREE read-only, CORPORATE full-edit).
- **Single-writer**: router pemilik brain DB (`brain.OpenRW`, WAL). Proses luar (flowork-gui scanner) WAJIB lewat endpoint loopback, tak boleh tulis DB langsung (anti-korup/lock).

## File yang dilewati
- `router/routes.go`; `router/handlers_brain*.go` (16 file); `router/handlers_pentest.go`; `router/dreamgraph_autosync.go`.
- `router/internal/brain/` — brain.go, init.go (schema), crud.go, retrieve.go, semantic.go, explore.go, dream_cycle.go, graph_extras.go, instincts.go, mistakes.go, rescore.go, mem_type_registry.go, skills.go, seed_doctrine.go, seed_instinct.go.
- `router/internal/store/` (kv, migrasi); `router/web/static/index.html` (`data-tab="brain"`, ~baris 385-691).
- DB: `~/.flow_router/brain/flowork-brain.sqlite`.

## Teknologi
- SQLite WAL (single-writer), FTS5 fallback.
- Embedding bge-m3 lokal + `vecindex` (cosine).
- Knowledge graph relasional (nodes/edges) + auto-sync ke RAG drawer.
- Governance amandemen (propose/vote) + editionGate (`FLOWORK_EDITION`).

## SWITCH / seam evolusi
- Threshold & perilaku recall via fwswitch (`FLOWORK_SEARCH_MINSCORE`, instinct switches).
- Knowledge/persona/constitution/instinct = **DATA** (baris DB), bukan kode → tumbuh tanpa buka frozen.
- Mem-type baru = registry entry; endpoint brain baru = `*_ext.go` + `RegisterExtraRoute`.

## Catatan jujur (bukan bug)
- `internal/brain/dream_cycle.go` (pipeline dream LEGACY) **dimatikan default** (butuh `FLOWORK_LEGACY_DREAM=1`) karena bug data-loss lama; ini **dead-code di balik flag, TIDAK dipanggil GUI**. Jalur graph aktif = `/api/brain/graph/sync` (sehat, terverifikasi live). Reclassify LLM = Phase 3 (future), tak memengaruhi jalur aktif.

## Status freeze (QC 2026-06-26)
- Live GUI: status/search/drawer/constitution/typed/personas/instincts/graph-sync JALAN, 0 mock di jalur aktif.
- FROZEN: semua handlers_brain*.go + handlers_pentest.go + dreamgraph_autosync.go + internal/brain/*.
- NON-FROZEN (sengaja): `index.html` (GUI), `internal/fwswitch/registry.go` (switch), `routes_ext.go` (seam).

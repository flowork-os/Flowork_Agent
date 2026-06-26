# DREAMGRAPH (router Knowledge Graph) — auto-populate + auto-update

> Router cognitive graph yang tampil di dashboard `:2402` tab Brain → "FLowork Knowledge Graph
> (DreamGraph)". BEDA dari CGM agent (`lock/CognitiveGraph.md`, per-agent `state.db`). Owner: Mr.Dev.

## APA INI
Graph relasi entitas-inti Flowork (node `cognitive_nodes` + edge `cognitive_edges` di
`router/brain/flowork-brain.sqlite`), cermin dari sumber: **constitution + persona + skill + agent**
→ kesadaran-diri sistem (aturan, identitas, kemampuan).

## MASALAH (sebelum, akar)
Graph KOSONG (0 node/0 edge) → canvas blank. Akar: tabel `cognitive_nodes/edges` cuma keisi via
`SyncGraphToRAG` (kepanggil pas CRUD node/edge manual doang) atau `RunDreamCycle` (disabled: mock +
data-loss). GAK ADA trigger boot/terjadwal → selamanya kosong.

## FIX (cabut-akar)
File baru NON-frozen `router/dreamgraph_autosync.go`:
- **Boot-populate**: `startDreamGraphAutoSync(ctx)` dipanggil di `router/main.go` (deket ticker
  fresh-index) → sekali sync saat boot → graph langsung keisi.
- **Auto-update**: loop poll 30 dtk, sync tiap interval (GUI switch, default 5 menit) → graph
  selalu cermin sumber tanpa aksi manual. Loop RE-BACA switch tiap siklus → ganti di GUI langsung
  kepakai (gak perlu restart).
- **Manual**: `POST /api/brain/graph/sync` (tombol "Sync Now") → balas `{ok,nodes,edges}`.
- Mekanisme = `brain.SyncGraphToRAG` (idempotent, MIRROR-only: constitution/persona/skill/agent →
  graph; TIDAK hapus memory). Serialized via mutex (anti SQLite-lock bentrok ticker vs manual).

## SWITCH GUI (kebenaran di GUI, bukan hardcode)
Di `agent/internal/fwswitch/registry.go` (kategori "Brain / Graph"), muncul di tab "🎛️ Switch Fitur":
- `FLOWORK_DREAMGRAPH_AUTOSYNC` (bool, default `true`) — ON/OFF auto-sync.
- `FLOWORK_DREAMGRAPH_SYNC_MIN` (int, default `5`) — interval menit.
Lintas-proses via `~/.flowork/flowork_settings.json` → router baca live (≤3 dtk).

## CAKUPAN SUMBER (file `graph_extras.go` — SyncGraphExtended)
DreamGraph mirror dari: **constitution + persona + skill + agent** (core, `syncCoreEntitiesToGraph`)
\+ **INSTINCTS** (drawers room `instinct_*` → node type `instinct`) + **KNOWLEDGE** (hub per-WING,
BUKAN node-per-drawer biar 860k gak meledak). Semua spoke → hub `flowork` (anti orphan). Idempotent
(cleanup by-source dulu), MIRROR-only (gak hapus sumber/memory).

## EXTENSION SEAM — RegisterGraphProjection (2026-06-26, nutup gap audit)
**Akar:** INSTINCTS + KNOWLEDGE dipanggil INLINE di `SyncGraphExtended` (FROZEN). Nambah proyeksi
BARU = kepaksa buka file frozen. Skill udah punya `RegisterSkillProvider`; graph projection BELUM → ditutup.
**Fix (cabut-akar):** file NON-frozen `router/internal/brain/graph_extras_ext.go` = registry
`RegisterGraphProjection` (switch-aware via `extraSwitchOn` + fails-open). `SyncGraphExtended` (frozen)
di-hook 1 baris (`runExtraGraphProjectionsTx(ctx,tx)` sebelum RAG-mirror) → proyeksi jalan dalam tx yg sama.
**Nambah proyeksi baru (zero edit frozen):** bikin file sibling di `internal/brain` →
`func init(){ RegisterGraphProjection(GraphProjection{Name,Switch:"FLOWORK_DREAMGRAPH_XXX",Run:func(ctx,tx)(int,error)}) }`
\+ tambah switch di `agent/internal/fwswitch/registry.go` → muncul GUI. Run WAJIB idempotent + mirror-only.
**Test:** `TestRegisterGraphProjectionRouter` / `TestRouterProjectionSwitch` (pkg brain) PASS. Re-freeze
`graph_extras.go` PASS (`TestKernelFreeze`).

## VERIFIKASI (2026-06-26)
- Boot log: `dreamgraph: boot sync OK`.
- `cognitive_graph_stats` 0/0 → **325 node / 324 edge**: 285 instinct + 24 knowledge-wing + 14
  constitution + 2 (agent+persona). Semua nyambung ke `flowork` (no orphan by-design).
- Switch GUI (4): `FLOWORK_DREAMGRAPH_AUTOSYNC/_SYNC_MIN/_INSTINCTS/_KNOWLEDGE` — kebukti muncul di
  tab "🎛️ Switch Fitur".
- `POST /api/brain/graph/sync` works. Build+vet+TestKernelFreeze PASS. Idempotent.

## GUI CANVAS (router dashboard, `web/static/index.html`)
Node banyak (325) bikin 3 masalah → fix (verified via headless screenshot):
- Label numpuk/smear → label cuma render pas HOVER / node HUB kalau graph padat (>40). Full di panel.
- Node ke-fling ke luar layar → `resetGraphView` = FIT-TO-BOUNDS (contek codemap `fitGraph`) +
  auto-fit setelah settle + clamp velocity.
- Gerak terus (bikin pusing) → `graphCool`: physics jalan ~240 frame nyusun, lalu BEKU. Idup lagi
  cuma pas drag.

## KEPUTUSAN FREEZE
File-file ini = **orchestration/extension seam + switch-protected**, SENGAJA non-frozen:
- `main.go`/`routes.go` → harus tetap terbuka buat nambah route/boot-hook (freeze = mateng evolusi).
- `registry.go` → extension-point switch (by-design non-frozen).
- `dreamgraph_autosync.go` → switch (FLOWORK_DREAMGRAPH_AUTOSYNC) = pengaman evolusi; tuning lewat
  GUI, bukan edit kode. Logika inti graph (`SyncGraphToRAG`) ada di `dream_cycle.go` (soft-lock).
Alasan: prinsip "freeze CORE, biarin seam" → switch sudah cukup lindungi dari AI lain ngerusak.

## MEMORY (6th source) — temuan 2026-06-26
Tabel `memories` router = KOSONG (router gak simpan memory episodik by-design). Memory episodik =
AGENT-side (`interactions`, 1200 row) dan **SUDAH ke-graph** di CGM agent (#cognitive) via dream-cycle
aktif: 424 node memory-derived (concept/fact/person/event/trait/preference/memory). Jadi rebuild
dream-cycle LLM (data-loss-prone) = MUBAZIR (jalan di tabel kosong) + BAHAYA → TIDAK dibangun.
Router DreamGraph = 5 sumber router-scoped (constitution/persona/skill/instinct/knowledge); memory
lengkap di CGM agent. 6 sumber komplit lintas dua graph. Instincts ✅ + Knowledge-hub ✅ DONE.

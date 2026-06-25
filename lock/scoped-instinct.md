# SCOPED INSTINCT (RI-5) — insting per-peran via X-Agent-ID

> Owner: Aola Sahidin (Mr.Dev). Update: 2026-06-25. Koloni BERLAPIS: tiap semut cuma dapet
> insting domain-nya (coder ≠ bisnis) + baseline universal/tool. Token-efisiensi + anti-noise.

## AKAR yang dicabut
Insting lintas-domain ke-inject ke SIAPA AJA → boros + noise. Router DULU **buta** siapa
pemanggil (`store.APIKey` no agent-id, model `flowork-brain` shared, agent ga kirim identitas).

## RANTAI (end-to-end)
```
agent (selfID) ──X-Agent-ID header──▶ router auth-middleware ──ctx──▶ instinct selector ──filter by Room──▶ inject
```
1. **Agent kirim** `X-Agent-ID: selfID()` HANYA pada call `/v1/chat/completions` — di `fetch()`:
   `agent/agentkit/agentkit.go` (FROZEN — nutup SEMUA worker + agent-template lewat delegasi
   `agentkit.Main()`) + `agents/mr-flow/main.go` (FROZEN, loop sendiri). No-op kalau selfID kosong.
2. **Router parse** header → ctx: `handlers_apikey_auth.go` (sebelum cabang auth → kena keyed+keyless)
   + helper `internal/router/agentctx_ext.go` (`WithAgentID`/`AgentIDFromContext`, sibling authctx LOCKED).
3. **Selector ctx-aware**: `instinctenrich.go` (FROZEN) punya hook ke-2 `instinctSelectHookCtx`
   (`RegisterInstinctSelectorCtx`), call-site PREFER ctx-hook. Logika scoping di
   `internal/router/instinctenrich_ext2.go` (NON-frozen): filter `all` → baseline ∪ domain-peran,
   lalu rank pakai `semanticInstinctSelector` (reuse RI-1 vindex).

## SWITCH (Rule 7 — evolusi tanpa buka freeze)
| ENV | Default | Guna |
|---|---|---|
| `FLOWORK_INSTINCT_SCOPED` | **off** | master switch scoping. off = perilaku PROVEN (semantic, no scope) |
| `FLOWORK_INSTINCT_SCOPE_MAP` | — | role-map RUNTIME (no rebuild): `agent:room1,room2;agent2:room3` |

Role-map default (compiled, `instinctenrich_ext2.go` `roleDomains`): `mr-flow` → semua domain
(generalis, no-op aman). Tambah agent lain di map / ENV buat aktifin scoping-nya.

## GUI — "Agent Brain" panel (per-agent, GUI = SUMBER KEBENARAN)
Tab tool-catalog (subscribe/unsubscribe, vestigial pasca all-tools) di-REPURPOSE jadi panel per-agent:
- **File:** `~/.flowork/agent_brain_config.json` (data user, gitignored) — `{ "<agentId>": {"instinct_domains":[...],
  "defer_tools":bool|null, "expose_all":bool|null} }`. Ditulis GUI, DIBACA dua proses.
- **Backend host** `agent/brain_config_ext.go` (NON-frozen): endpoint **`/api/agents/brain-config`** (GET/POST,
  AUTH-GATED = GUI cookie) + `RegisterDeferPolicy` per-agent (defer/all-tools dari file; fallback ENV PERSIS
  `FLOWORK_DEFER_TOOLS`/`FLOWORK_EXPOSE_ALL_TOOLS` → byte-identik pas agent ga ke-set).
- **Router** `instinctenrich_ext2.go` `scopeFromBrainConfig`: baca file → instinct_domains per-agent.
- **Prioritas scope:** file brain-config (GUI) **>** ENV `FLOWORK_INSTINCT_SCOPE_MAP` **>** compiled `roleDomains`.
- **Frontend** `web/tabs/agents_tool_catalog.js` (export `renderToolCatalog` dipertahankan): centang domain insting
  (baseline universal/tool locked) + tri-state defer/all-tools + Simpan. **+ Form "➕ Tambah insting"** → POST
  `/api/brain/ingest/submit` (content WHEN→THEN + domain=room + importance) → brain SHARED, auto-index ≤2 menit.
- **Verified live:** set file `mr-flow→[instinct_bisnis]` → `instinct-scope: agent="mr-flow" domains=[bisnis]
  284→184` (override compiled 4-domain). defer fallback no-regression (env defer/expose=1 → mr-flow tools=22 tetap).

## KURASI PENUH via GUI — Brain tab router (#6 SELESAI 2026-06-26)
Selain panel per-agent di atas, **Brain tab** (`router/web/static/index.html`, di-embed router :2402)
= kurasi DOKTRIN + INSTING penuh, semua NON-frozen (web) di atas handler yg udah ada:
- **Insting CRUD** (room LIKE `instinct%`): ADD (`openInstinctAdd`→`/api/brain/ingest/submit`),
  **EDIT** (`openInstinctEdit`→`PUT /api/brain/drawer`) — bisa **pindahin domain** (room editable,
  normalisasi "coding"→"instinct_coding"; **penting buat scoping** krn injeksi room-based), **HAPUS**
  (`confirmInstinctDelete`→`DELETE /api/brain/drawer` soft-delete). Backend: `brain.AddDrawer/UpdateDrawer/
  SoftDeleteDrawer` (`internal/brain/crud.go`) sinkron FTS; fresh-index re-embed ≤2 mnt; content selalu
  fresh dari tabel `drawers` saat retrieve (vektor cuma buat NEMU). mem_type ga relevan ke insting
  (injeksi room-based) → default 'project' aman, ga perlu buka frozen instincts.go.
- **Doktrin CRUD**: add/edit/del (`/api/brain/constitution` GET/POST/PUT/DELETE) + **amendments
  governance-gated** (`loadBrainAmendments`/`voteAmendment` → `/api/brain/constitution/amendments` +
  `/amend/vote`) — edit aturan sakral butuh approve owner, ga langsung apply.
- **Verified live (Rule-9):** ADD/EDIT/DELETE insting 200; edit domain `universal→crypto` readback OK.

## EXTERNAL SCOPE (#11 brain-as-service) — anti-halu caller LUAR
Agent LUAR (ber-jiwa-Aola via `:2402/v1`, ga punya tool internal Flowork) kalau dikasih insting
`instinct_tool` ("WHEN butuh X → panggil tool Y") bakal **HALU** nyoba manggil tool yg ga ada.
- **`externalScopedSelector`** (`instinctenrich_ext2.go`, NON-frozen): caller external (X-Agent-ID
  KOSONG) → buang `instinct_tool`, sisanya (universal + reasoning per-domain: coding/security/
  crypto/bisnis/kehidupan) TETAP lolos (pengetahuan murni, aman). Fails-open kalau ngosongin semua.
- **Switch `FLOWORK_BRAIN_EXTERNAL_SCOPE`** (default **OFF**): id-kosong AMBIGU (agent template-lama
  belum-rebuild juga kosong tapi BUTUH instinct_tool) → owner nyalain pas expose brain ke klien luar
  (saat itu semua agent internal udah kirim X-Agent-ID). **Independen** dari master per-agent switch
  `FLOWORK_INSTINCT_SCOPED` → bisa amanin brain-luar tanpa scoping internal.
- Universal DOKTRIN buat external lewat `maybeInjectConstitution` (terpisah, `InjectConstitution`
  setting). Mode `augment` (jangan clobber persona external) + `AlwaysOn` udah ADA di `BrainConfig`.
- **Verified:** unit `TestExternalScope_{DropsToolInstinct,InternalUnaffected,OffKeepsToolForLegacy}`
  (3 PASS). Live `/v1` external (no X-Agent-ID) → HTTP 200 jawaban ber-jiwa-Aola.
- **Residual:** constitution-scoping buat external (cuma UNIVERSAL doktrin, skip doktrin-internal-Flowork)
  → butuh tag universal di tabel constitution + nyentuh `brain_constitution.go` (FROZEN). Belum.

## FAILS-OPEN (anti-rusak) — di TIAP titik balik ke `semanticInstinctSelector` (perilaku lama):
switch off · agent-id kosong (external / agent belum di-rebuild kirim header) · agent belum di-map ·
hasil filter kosong. **Baseline `instinct_universal` + `instinct_tool` SELALU lolos** → ga ada agent
"buta tool" gara2 scoping.

## VERIFIKASI
- Unit: `go test ./internal/router/ -run TestScoped` (5 PASS: filter, fails-open ×3, baseline).
- Live (Rule-9): `grep "instinct-scope:" /tmp/flowork-watchdog.log` →
  `agent="mr-flow" domains=[coding security crypto bisnis] 284→280 kandidat` + reply koheren (no regression).

## CATATAN
- **Template TAK diubah**: agent-template/worker = `func main(){ agentkit.Main() }` (delegasi) →
  fix agentkit cukup. group/connector-template ga call LLM-router.
- **5 worker agentkit** (browse-surfer/-reporter, fbspecial, fb-writer, fb-repofinder) DI-REBUILD 2026-06-26 →
  kirim X-Agent-ID (verified live: browse-surfer `domains=[coding] 284→212`). Agent template-lama deploy-only (~24,
  no source) BELUM → fails-open (aman); rebuild pas perlu. Rebuild = `bash scripts/build-agent.sh <id>` (wasm derived).
- Sumber role belum dari manifest (manifest no field role) → role-map = sumber kebenaran (router-side, editable).

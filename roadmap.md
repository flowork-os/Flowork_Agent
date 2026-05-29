# Roadmap — Brain & Memory di Flowork_Agent (per-warga state)

> **🔪 CUT items 2026-05-29** (anti over-engineering, anti halu):
> - LoRA delta sync (section 28 mesh — sudah pindah ke Router) — defer P3
> - Full voting governance subsystem — defer (single-owner cukup, lihat standar section 10)
> - Browser tools Tier 2 (8 tools: click/drag/extract/navigate/render/search/type/universal) — defer P2
> - Codemap tools Tier 2 (5 tools beyond core) — defer P2
> - Music/social media tools — defer P3
> - MCP tools (mcp_auth/call/list_resources/read_resource) — defer P3
>
> **⚠️ ANTI OVER-PROMPT** wajib: lihat [doc/standar_ai_agent.md section 11 "Prompt Budget"](doc/standar_ai_agent.md). Setiap section yang inject ke prompt punya budget cap. Marker `⚠️ OVER-PROMPT RISK` di section 1, 2, 11, 27.
>
> Filosofi: **single-owner sekarang. YAGNI strict.** Lebih sedikit code = lebih sedikit bug = LLM lebih jarang halu.

> **Konteks 2-tubuh.** Flowork dipecah jadi 2:
> - **Router** (`Documents/flowork_Router/`, port `:2402`) = brain kolektif (drawers, embeddings, constitution, retrieval, LLM dispatch). Stateless soal "siapa".
> - **Agent** (`Documents/Flowork_Agent/`, port `:1987`) = body + identitas warga. State per-warga terisolasi di `agents/<id>/workspace/state.db`. Stateless soal "knowledge bareng".
>
> Roadmap ini cuma untuk **AGENT** — state personal warga. Semua tabel hidup di SQLite per-agent. Brain kolektif diserahin ke router (lihat `flowork_Router/roadmap.md`).
>
> **Prinsip:** Tetap mengikuti standar [doc/standar_ai_agent.md](doc/standar_ai_agent.md) — 9 hal wajib terisolasi, semua state di folder warga, ngga ada cross-warga leak.

---

## Section 1 — Episodic interactions (memori percakapan personal) ✅ DONE 2026-05-29

> **⚠️ OVER-PROMPT RISK** — kalau interaction history di-auto-inject ke chat context, balloon cepat. Pakai HANYA via tool call (`memory_get`), JANGAN always-on injection. Persona `Lo punya episodic log via memory tool` (1 baris) > stuff 10 interaksi ke prompt.

> **✅ Selesai 2026-05-29** — full end-to-end verified (in+out Telegram ter-log clean). Implementation: `internal/agentdb/interactions.go` + ensureSchema table+4 index, host capability `host_log_interaction` di `internal/kernel/runtime/host.go`, resolver `Host.logInteraction` di `internal/kernelhost/kernelhost.go`, hook in/out di `agents/mr-flow/main.go::runDaemon`, HTTP endpoint `GET /api/agents/interactions` di `internal/agentmgr/agentmgr.go`. Popup Episodic Memory UI defer ke batch UI section. Detail lihat `CHANGELOG.md` entry 2026-05-29.

**Goal:** tiap warga punya log interaksi sendiri (chat Telegram, RPC call, dst). Bukan untuk LLM context (itu router brain), tapi untuk personal recall warga + analytics + audit.

**Tabel baru di `state.db`:**

```sql
CREATE TABLE interactions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  channel     TEXT NOT NULL,             -- 'telegram' | 'rpc' | 'cron' | dst
  direction   TEXT NOT NULL,             -- 'in' | 'out'
  actor       TEXT NOT NULL DEFAULT '',  -- chat_id, caller_id, scheduler
  content     TEXT NOT NULL,
  metadata    TEXT NOT NULL DEFAULT '{}', -- JSON: message_id, group_id, model used
  occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_interactions_channel ON interactions(channel);
CREATE INDEX idx_interactions_actor   ON interactions(actor);
CREATE INDEX idx_interactions_time    ON interactions(occurred_at DESC);
```

**Implementasi:**
- `internal/agentdb/interactions.go` — Insert(), List(channel, actor, limit), DeleteOlderThan(duration)
- Mr.Flow `runDaemon()` panggil `Insert("telegram", "in", chatID, text, meta)` setiap pesan masuk + `Insert("telegram", "out", chatID, reply, meta)` setiap balas
- Auto-prune via section 8

**Referensi file:**
- [`section_01_episodic_interactions/recorder.go`](referensifile/section_01_episodic_interactions/recorder.go) — pattern interaction recorder dari brain/proxy
- [`section_01_episodic_interactions/record_chat.go`](referensifile/section_01_episodic_interactions/record_chat.go) — chat session record (worker brain)
- [`section_01_episodic_interactions/agent_tool.go`](referensifile/section_01_episodic_interactions/agent_tool.go) — agent interaction tracking
- [`section_01_episodic_interactions/session_state.go`](referensifile/section_01_episodic_interactions/session_state.go) — session state pattern
- [`section_01_episodic_interactions/task_events.go`](referensifile/section_01_episodic_interactions/task_events.go) — event log generic
- Pattern soft-delete pakai [`_common/softdelete.go`](referensifile/_common/softdelete.go).

**Acceptance criteria:**
- Tabel ada, index created saat agentdb ensureSchema
- Mr.Flow log setiap msg in+out (verify via `sqlite3 agents/mr-flow/workspace/state.db "SELECT count(*) FROM interactions"`)
- API endpoint `GET /api/agents/<id>/interactions?channel=&limit=` di agentmgr
- Popup section baru "Episodic Memory" — tampil list 50 last interaction (read-only)

---

## Section 2 — Mistakes journal (lesson lokal sebelum promote) ✅ DONE (phase 1) 2026-05-29

> **⚠️ OVER-PROMPT RISK** — mistakes lama JANGAN auto-inject ke persona ("hindari pattern X, Y, Z, ..." → bloat). Pakai pattern: insert ringkasan **top-3 mistakes terbaru** saja kalau relevant context (semantic match query). Sisanya retrieved via `brain_search` tool.

> **Phase 1 scope (sekarang)**: schema `mistakes_local`, `internal/agentdb/mistakes.go` (Add UNIQUE upsert hit_count, List, Prune, Count), endpoint admin add + list. **Defer**: host capability auto-log (Mr.Flow ngga punya self-reflect use case yet), promotion ke router brain (Section 7 cross-tubuh sync), popup UI (batch UI section).

**Goal:** warga catat kesalahan / insight personal. Tier `raw` dulu di lokal. Setelah validasi (frequency, importance), di-promote ke router brain (jadi `brain_antibody` global).

**Tabel baru:**

```sql
CREATE TABLE mistakes_local (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  category        TEXT NOT NULL,             -- 'logic' | 'safety' | 'performance' | dst
  title           TEXT NOT NULL,
  content         TEXT NOT NULL,             -- deskripsi lengkap
  context_origin  TEXT NOT NULL DEFAULT '',  -- interaction_id atau apa pun
  tier            TEXT NOT NULL DEFAULT 'raw', -- 'raw' | 'reviewed' | 'promoted'
  hit_count       INTEGER NOT NULL DEFAULT 1,
  last_hit_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  promoted_at     TEXT,                       -- ISO timestamp saat di-push ke router
  promoted_to_id  TEXT,                       -- antibody_id di router brain (kalau sukses)
  deleted_at      TIMESTAMP, deleted_by TEXT,
  UNIQUE(category, title)
);
CREATE INDEX idx_mistakes_local_tier      ON mistakes_local(tier);
CREATE INDEX idx_mistakes_local_promoted  ON mistakes_local(promoted_at);
CREATE INDEX idx_mistakes_local_deleted_at ON mistakes_local(deleted_at);
```

**Implementasi:**
- `internal/agentdb/mistakes.go` — Add() (idempotent via UNIQUE → INCREMENT hit_count), Promote(id), List(tier)
- Promotion logic: hit_count ≥ 3 OR tier='reviewed' → POST ke router `/api/mistakes/submit` (router validate + insert brain_antibody)
- Mr.Flow + agent lain bisa call `mistakes.Add(...)` lewat RPC internal

**Referensi file:**
- [`section_02_mistakes_local/mistakes_journal.go`](referensifile/section_02_mistakes_local/mistakes_journal.go) — pattern dari flowork lama (untuk tier struct + promotion logic, adaptasi ke per-warga local)

**Acceptance criteria:**
- Tabel ada, soft-delete pattern.
- RPC method baru di mr-flow: `log_mistake({category, title, content})`.
- POST `/api/agents/<id>/mistakes/promote?mistake_id=N` push ke router.
- Popup section "Lesson Learned" — list mistakes per tier (raw/reviewed/promoted).

---

## Section 3 — Decisions log (audit trail keputusan warga) ✅ DONE 2026-05-29

**Goal:** setiap keputusan non-trivial yang warga ambil (mis. pilih model, skip task, eskalasi) tercatat dengan rationale. Penting buat debugging + accountability + training future warga.

> **✅ Selesai 2026-05-29** — adversarial-audit passed (2 critical + 4 important fixed). Implementation:
> - `internal/agentdb/decisions.go` (LOCKED) — Log/List/Prune/Count, return decision ID
> - Host capability `host_log_decision` di `internal/kernel/runtime/host.go` (state:write gate)
> - `Host.logDecision` di `internal/kernelhost/kernelhost.go` (hold-lock-through-Open+Log)
> - Mr.Flow hook 3 call site di `runDaemon`: `skip_task` (drop unauthorized chat), `escalate` (LLM fail — known prefixes), `model_choice` (success)
> - Endpoint `GET /api/agents/decisions?type=&limit=` di `internal/agentmgr/agentmgr.go`
> - Audit critical fix: `llmFailed` heuristic akurat (router error / decode / llm / no choices) + capture `origReply` sebelum overwrite + forward decision ID di response.
> - Behavior verify pending Telegram trigger Mr.Dev (build + audit clean).
> - Popup section UI defer ke batch UI section.

**Tabel baru:**

```sql
CREATE TABLE decisions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  decision_type TEXT NOT NULL,             -- 'model_choice' | 'skip_task' | 'escalate' | 'tool_pick' | dst
  rationale     TEXT NOT NULL,
  inputs        TEXT NOT NULL DEFAULT '{}', -- JSON: konteks input
  outcome       TEXT NOT NULL DEFAULT '',   -- 'success' | 'fail' | 'pending'
  ref_interaction_id INTEGER,               -- link ke interactions.id kalau relevant
  occurred_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (ref_interaction_id) REFERENCES interactions(id)
);
CREATE INDEX idx_decisions_type ON decisions(decision_type);
CREATE INDEX idx_decisions_time ON decisions(occurred_at DESC);
```

**Implementasi:**
- `internal/agentdb/decisions.go` — Log(), List(typeFilter, limit)
- Mr.Flow log decision saat: pilih model fallback, drop chat unauthorized, fail call LLM

**Referensi file:**
- [`section_03_decisions_log/task_events.go`](referensifile/section_03_decisions_log/task_events.go) — pattern event log generic dari flowork lama (table struct + insert helper)

**Acceptance criteria:**
- Tabel ada.
- Mr.Flow log 3 jenis decision di runtime (verify via SQL count).
- API `GET /api/agents/<id>/decisions?type=&limit=`.
- Popup section "Decisions" — list 30 last keputusan.

---

## Section 4 — Death letter (legacy pesan terakhir per-warga) ✅ DONE (phase 1) 2026-05-29

**Goal:** kalau warga di-retire (toggle off + remove), simpan "death letter" — pesan terakhir, value yang dia carry, instruksi buat penerus. Bisa di-download bareng .fwagent.zip → legacy preservation.

**Tabel baru:**

```sql
CREATE TABLE death_letter (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  letter_type   TEXT NOT NULL,             -- 'farewell' | 'handover' | 'reflection'
  recipient     TEXT NOT NULL DEFAULT '',  -- 'all' | '<successor_agent_id>'
  subject       TEXT NOT NULL,
  body          TEXT NOT NULL,
  written_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sealed_at     TEXT                       -- sekali di-seal, ngga bisa di-edit
);
CREATE INDEX idx_death_letter_recipient ON death_letter(recipient);
```

**Implementasi:**
- `internal/agentdb/death_letter.go` — Write(), Seal(id), Read(recipient)
- RPC method `write_death_letter({subject, body, type, recipient})` — warga sendiri yang tulis (atau owner via UI)
- Saat agent remove via `/api/agents/remove`, kalau ada unsealed letter → auto-seal dulu, tetep ikut di download zip

**Referensi file:**
- [`section_04_death_letter/death_letter.go`](referensifile/section_04_death_letter/death_letter.go) — pattern dari flowork lama (table struct + CRUD)

**Acceptance criteria:**
- Tabel ada.
- Mr.Flow bisa write death letter via RPC.
- Letter ikut zip di download.
- Popup section "Legacy / Death Letter" — write + seal UI.

---

## Section 5 — Karma self (reputation tracking per-warga) ✅ DONE 2026-05-29

**Goal:** warga track score sendiri — success rate, response time, user satisfaction. Bukan ranking antar-warga (itu router kalau perlu), tapi metrik diri sendiri buat self-improvement.

**Tabel baru:**

```sql
CREATE TABLE karma_self (
  metric_key   TEXT PRIMARY KEY,           -- 'success_count' | 'fail_count' | 'avg_response_ms' | dst
  metric_value REAL NOT NULL DEFAULT 0,
  metric_count INTEGER NOT NULL DEFAULT 0, -- buat moving average
  updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Implementasi:**
- `internal/agentdb/karma.go` — Increment(key, delta), AverageUpdate(key, value), Get(key)
- Mr.Flow update setiap reply: success_count++, avg_response_ms moving average

**Referensi file:**
- [`section_05_karma_self/karma.go`](referensifile/section_05_karma_self/karma.go) — pattern karma dari flowork lama (untuk tier + moving avg logic)

**Acceptance criteria:**
- Tabel ada.
- Mr.Flow update minimal 3 metric (success, fail, avg_ms).
- API `GET /api/agents/<id>/karma`.
- Popup section "Stats" — tampil sebagai dashboard kecil (badge + sparkline).

---

## Section 6 — Workspace meta (metadata folder workspace)

**Goal:** index isi workspace shared per-warga (`/shared/<id>/tools/`, `job/`, `document/`, dst). Tools generated, files created, last-modified — biar warga sendiri dan warga lain bisa browse.

**Tabel baru:**

```sql
CREATE TABLE workspace_meta (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  category     TEXT NOT NULL,    -- 'tools' | 'job' | 'document' | 'media' | 'cache' | 'log'
  path         TEXT NOT NULL,    -- relative dari /shared/<id>/, mis. 'tools/resize_image.py'
  description  TEXT NOT NULL DEFAULT '',
  size_bytes   INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL DEFAULT '',
  shareable    INTEGER NOT NULL DEFAULT 1,  -- 1 = boleh diakses warga lain, 0 = private even di shared
  created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(category, path)
);
CREATE INDEX idx_workspace_meta_category ON workspace_meta(category);
```

**Implementasi:**
- `internal/agentdb/workspace_meta.go` — Register(category, path, desc), List(category), Lookup(path)
- Helper: scan `<shared_root>/<agent_id>/<category>/` recursive, auto-register file baru dengan hash + size
- Cron tiap jam: rebuild index buat detect file yang di-create di luar via RPC

**Referensi file:**
- [`section_06_workspace_meta/workspace_meta.go`](referensifile/section_06_workspace_meta/workspace_meta.go) — pattern dari flowork lama

**Acceptance criteria:**
- Tabel ada.
- Helper auto-scan jalan saat boot agent.
- API `GET /api/agents/<id>/workspace?category=` — list file index.
- (Opsional) endpoint `_index.json` aggregator di router yang scan semua agent's workspace_meta (untuk discovery cross-agent tools).

---

## Section 7 — Sync interface ke router (push/pull antar-tubuh)

**Goal:** definisi protokol komunikasi agent ↔ router untuk: push mistakes promotion, pull skill catalog, query brain.

**Implementasi:**

**Pull dari router (agent inisiator):**
- `GET <router>/api/skills/list` → list skill catalog yang router punya
- `GET <router>/api/skills/get?id=<skill_id>` → detail skill (instructions, schema)
- `POST <router>/api/brain/test` (sudah ada) → retrieve drawer untuk query agent

**Push ke router (agent inisiator):**
- `POST <router>/api/mistakes/submit` — body `{agent_id, category, title, content, hit_count}` → router validate + insert brain_antibody
- `POST <router>/api/brain/contributions/ingest` (sudah ada) → submit drawer baru hasil interaction warga

**Code:**
- `internal/routerclient/routerclient.go` baru — HTTP client wrapper, baca `router_url` dari `kv` per-agent
- Reusable helper untuk POST/GET JSON dengan retry + timeout
- Setiap RPC method warga yang submit ke router via client ini (bukan langsung pakai `host_net_fetch`)

**Referensi file:**
- [`section_07_sync_router/bridge.go`](referensifile/section_07_sync_router/bridge.go) — brainbridge agent↔router bridge pattern
- [`section_07_sync_router/cache.go`](referensifile/section_07_sync_router/cache.go) — caching layer pattern
- [`section_07_sync_router/typed_memory_bridge.go`](referensifile/section_07_sync_router/typed_memory_bridge.go) — typed memory access via bridge
- [`section_07_sync_router/brain_query.go`](referensifile/section_07_sync_router/brain_query.go) — agent → router brain query call pattern
- [`section_07_sync_router/brain_v2_tools.go`](referensifile/section_07_sync_router/brain_v2_tools.go) — brain V2 tools pattern (search/recall)
- [`section_07_sync_router/worker_sync.go`](referensifile/section_07_sync_router/worker_sync.go) — worker ↔ kernel sync pattern (adaptasi ke agent ↔ router)
- [`section_07_sync_router/worker_proxy.go`](referensifile/section_07_sync_router/worker_proxy.go) — proxy pattern + circuit breaker
- [`section_07_sync_router/route_register.go`](referensifile/section_07_sync_router/route_register.go) — route registration pattern
- Plus HTTP client primitives dari `agents/mr-flow/main.go::fetch()` (sudah ada).

**Acceptance criteria:**
- Package `internal/routerclient/` ada dengan SubmitMistake, PullSkill, QueryBrain methods.
- Mr.Flow saat hit_count mistakes ≥ 3 → auto-promote via routerclient.
- Skill picker di popup ada tombol "Browse Router Catalog" → fetch from router.

---

## Section 8 — Retention policy & soft-delete consistency ✅ DONE 2026-05-29

**Goal:** tabel-tabel di atas ngga boleh grow unbounded. Define retention per tabel + soft-delete semua data buat audit.

**Implementasi:**
- Semua tabel pakai pattern `deleted_at TIMESTAMP, deleted_by TEXT` (lihat `_common/softdelete.go`)
- `internal/agentdb/retention.go` — cron daily:
  - `interactions` > 30 hari → soft-delete
  - `decisions` > 90 hari → soft-delete
  - `mistakes_local` tier=`promoted` > 180 hari → soft-delete (sudah di router)
  - `workspace_meta` ngga prune (sumber truth file system)
  - `karma_self` ngga prune (state perpetual)
  - `death_letter` ngga prune (legacy)

**Hard-delete:**
- Cron weekly: `deleted_at < now - 90 days` → DELETE FROM ... (final cleanup)

**Referensi file:**
- [`_common/softdelete.go`](referensifile/_common/softdelete.go) — pattern soft-delete utility

**Acceptance criteria:**
- Semua tabel section 1-5 punya soft-delete columns.
- Cron jalan + log di `decisions` ("retention sweep N rows").
- Test: insert 100 interactions backdated 60 hari, verify auto-prune setelah cron.

---

## Section 9 — Educational error lookup (lokal)

**Goal:** kalau warga kepleset (error code, validation fail), bisa lookup penjelasan + remediation dari catalog lokal. Catalog seed dari router (sync sekali) atau bring-your-own per-warga.

**Tabel baru (mirror dari router):**

```sql
CREATE TABLE educational_errors_cache (
  code         TEXT PRIMARY KEY,
  category     TEXT NOT NULL,
  title        TEXT NOT NULL,
  explanation  TEXT NOT NULL,
  remediation  TEXT NOT NULL,
  synced_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Implementasi:**
- `internal/agentdb/edu_errors.go` — Lookup(code), CacheRefresh() (pull dari router)
- Mr.Flow saat catch error → lookup code → log decision dengan remediation suggestion

**Referensi file:**
- [`_common/educational_error_lookup.go`](referensifile/_common/educational_error_lookup.go) — pattern lookup dari flowork lama

**Acceptance criteria:**
- Tabel cache ada.
- `routerclient.PullEduErrors()` sync setiap boot.
- Mr.Flow pakai lookup minimal 1x in error path.

---

## Urutan implementasi (saran prioritas)

| # | Section | Priority | Reasoning |
|---|---|---|---|
| 1 | Section 1 — Episodic interactions | 🔴 P0 | Foundation. Semua section lain reference ke `interaction_id`. |
| 2 | Section 3 — Decisions log | 🔴 P0 | Audit trail = bukti kerja warga. Cepat implement (tabel simple). |
| 3 | Section 8 — Retention + soft-delete | 🟡 P1 | Sebelum table grow besar. Cron infra dipakai section lain. |
| 4 | Section 2 — Mistakes local | 🟡 P1 | Self-improvement loop. Tanpa promotion dulu (router belum siap). |
| 5 | Section 5 — Karma self | 🟢 P2 | Nice-to-have buat dashboard. |
| 6 | Section 6 — Workspace meta | 🟢 P2 | Buat cross-warga discovery. |
| 7 | Section 7 — Sync interface ke router | 🟡 P1 | Setelah router siap di section 7 router roadmap. |
| 8 | Section 4 — Death letter | 🟢 P2 | Legacy feature. Important tapi ngga blocking. |
| 9 | Section 9 — Edu errors lookup | 🟢 P3 | Polishing. |

**Catatan kerja:**
- Setiap section yang nambah tabel → update `internal/agentdb/agentdb.go::ensureSchema()`.
- Tiap section → tulis 1 file Go di `internal/agentdb/<topic>.go` + tambah handler di `internal/agentmgr/` + section di popup.
- Tiap section selesai → update `doc/standar_ai_agent.md` section 9 TODO.
- Tiap merge → tulis decision log di `doc/decisions/YYYY-MM-DD-<topic>.md`.

---

---

# === BAGIAN 2 — TOOLS SYSTEM ===

> **Konteks.** Flowork lama punya **~129 tools** (110 di worker registry + 19 di kernel subfolder).
> Sekarang Flowork_Agent cuma punya 5 **capability flag** (telegram/router/kv/fs/net) — bukan tools beneran, lebih kayak permission tag.
>
> Section 10-13 di bawah ini me-roadmap tools system: foundation (registry/permission/categories), tier 1 core tools (file ops, shell, web, git, memory, brain, skill, comms, task), execution sandbox (interceptors + permissions), dan discovery (list_my_tools, catalog browse).

---

## Section 10 — Tool system foundation (registry + permission + categories)

**Goal:** kerangka tool — pattern register, dispatch, permission check, kategori. Tanpa ini, tool implementasi-nya scatter dan ngga konsisten.

**Komponen yang harus dibikin:**

- **`internal/tools/types.go`** — interface `Tool` dengan `Name() string`, `Schema() Schema`, `Run(ctx, args) (Result, error)`.
- **`internal/tools/registry.go`** — singleton registry. Tools register via `init()` di package masing-masing (plug-and-play pattern). Lookup by name, list all.
- **`internal/tools/permission.go`** — gate: setiap tool punya `Capability()` declaration; broker cek warga apakah punya cap itu sebelum dispatch.
- **`internal/tools/categories.go`** — taxonomy lookup dari DB (`tool_categories` table) — config-driven, bukan hardcoded. Plus per-warga `division_tool_priors` (weighted ordering).
- **`internal/tools/capability_map.go`** — mapping tool → required capability strings (`fs:write`, `net:fetch:*`, `exec:shell`).
- **`internal/tools/aliases.go`** — sinonim/alias tool name (mis. `read` ↔ `read_tool`).

**Tabel baru di state.db (per-agent tools registry instance):**

```sql
-- Catatan: registry tools-nya di-load dari binary (registered via init()),
-- tapi PILIHAN tool yang warga ini aktif disimpan di state.db (lihat tabel `tools` yang sudah ada).
-- Plus customization per-warga:
CREATE TABLE tool_overrides (
  tool_name    TEXT PRIMARY KEY,
  config       TEXT NOT NULL DEFAULT '{}',  -- JSON: argumen default per-warga
  rate_limit   INTEGER DEFAULT 0,           -- max call per menit (0 = unlimited)
  disabled     INTEGER NOT NULL DEFAULT 0,  -- 1 = block walaupun cap ada
  updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tool_invocations (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_name    TEXT NOT NULL,
  args_json    TEXT NOT NULL,
  result_json  TEXT NOT NULL DEFAULT '{}',
  error_text   TEXT NOT NULL DEFAULT '',
  latency_ms   INTEGER NOT NULL DEFAULT 0,
  caller       TEXT NOT NULL DEFAULT '',   -- 'daemon' | 'rpc' | 'skill:<id>' | 'schedule:<id>'
  invoked_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tool_invocations_name ON tool_invocations(tool_name);
CREATE INDEX idx_tool_invocations_time ON tool_invocations(invoked_at DESC);
```

**Referensi file:**
- [`section_10_tool_foundation/types.go`](referensifile/section_10_tool_foundation/types.go) — interface Tool + Schema
- [`section_10_tool_foundation/registry.go`](referensifile/section_10_tool_foundation/registry.go) — registry pattern
- [`section_10_tool_foundation/permission.go`](referensifile/section_10_tool_foundation/permission.go) — capability gate
- [`section_10_tool_foundation/categories.go`](referensifile/section_10_tool_foundation/categories.go) — DB-backed taxonomy + per-warga priors
- [`section_10_tool_foundation/capability_map.go`](referensifile/section_10_tool_foundation/capability_map.go) — tool→cap mapping
- [`section_10_tool_foundation/aliases.go`](referensifile/section_10_tool_foundation/aliases.go) — alias resolver
- [`section_10_tool_foundation/caller_context.go`](referensifile/section_10_tool_foundation/caller_context.go) — caller identity context propagation
- [`section_10_tool_foundation/registry_const.go`](referensifile/section_10_tool_foundation/registry_const.go) — generated tool name constants (untuk reference taxonomy)

**Acceptance criteria:**
- Interface `Tool` ada + registry pattern jalan (test: register dummy tool, dispatch by name).
- `tool_overrides` + `tool_invocations` tabel auto-create.
- Dispatch jalan + permission gate enforced (test dengan warga tanpa cap, expect denied).
- API `GET /api/agents/<id>/tools/inventory` — list tools dengan flag enabled/disabled per-warga.

---

## Section 11 — Tool catalog Tier 1 (core tools port)

> **⚠️ OVER-PROMPT RISK CRITICAL** — JANGAN inject SEMUA 28 tool description ke system prompt. Itu = 5600 char waste. Pakai pattern: **5 core tools always-on** (read/write/bash/brain_search/telegram_send), sisanya 23 tools available tapi warga panggil via `tool_search` dulu. Tool description retrieved on-demand by name, ngga upfront dump.

**Goal:** port 30+ tool paling fundamental dari flowork lama. **Bukan semua 129 sekaligus** — fokus tier 1, sisanya bertahap.

**Tier 1 list (urut prioritas):**

| # | Tool | Kategori | Capability | LOC ref | Priority |
|---|---|---|---|---|---|
| 1 | `read` | file ops | `fs:read:/workspace/**` | ~150 | 🔴 P0 |
| 2 | `write` | file ops | `fs:write:/workspace/**` | ~150 | 🔴 P0 |
| 3 | `edit` | file ops | `fs:write:/workspace/**` | ~200 | 🔴 P0 |
| 4 | `multiedit` | file ops | `fs:write:/workspace/**` | ~120 | 🟡 P1 |
| 5 | `glob` | file ops | `fs:read:/workspace/**` | ~100 | 🟡 P1 |
| 6 | `grep` | file ops | `fs:read:/workspace/**` | ~150 | 🟡 P1 |
| 7 | `list` (ls) | file ops | `fs:read:/workspace/**` | ~80 | 🟡 P1 |
| 8 | `bash` | shell | `exec:shell` | ~250 | 🔴 P0 |
| 9 | `run_sandbox_command` | shell | `exec:sandbox` | ~200 | 🟡 P1 |
| 10 | `webfetch` | web | `net:fetch:*` | ~150 | 🔴 P0 |
| 11 | `websearch` | web | `net:search` | ~180 | 🟡 P1 |
| 12 | `git` (status/diff/log/show) | vcs | `exec:git` | ~250 | 🟡 P1 |
| 13 | `git_checkpoint` | vcs | `exec:git` | ~120 | 🟢 P2 |
| 14 | `brain_search` | memory | `rpc:router:brain` | ~80 | 🔴 P0 |
| 15 | `brain_recall` | memory | `rpc:router:brain` | ~80 | 🔴 P0 |
| 16 | `brain_get_drawer` | memory | `rpc:router:brain` | ~60 | 🟡 P1 |
| 17 | `memory_get/set/delete` | memory | `kv:rw` | ~120 | 🔴 P0 |
| 18 | `fact_remember/recall/forget` | memory | `kv:rw` | ~150 | 🟡 P1 |
| 19 | `skill` (run skill) | skill | `rpc:router:skill` | ~120 | 🟡 P1 |
| 20 | `skill_search` | skill | `rpc:router:skill` | ~80 | 🟡 P1 |
| 21 | `skill_write` | skill | `fs:write:/shared/<id>/tools/` | ~150 | 🟢 P2 |
| 22 | `telegram_send` | comms | `net:fetch:telegram` + `secrets:TELEGRAM_BOT_TOKEN` | ~80 | 🔴 P0 |
| 23 | `task` (synchronous sub-task) | orchestration | `rpc:internal` | ~200 | 🟡 P1 |
| 24 | `task_bg` / `task_agent_bg` | orchestration | `rpc:internal` | ~250 | 🟢 P2 |
| 25 | `task_parallel` | orchestration | `rpc:internal` | ~180 | 🟢 P2 |
| 26 | `plan_read` / `plan_write` | orchestration | `kv:rw` | ~100 | 🟡 P1 |
| 27 | `todo` | orchestration | `kv:rw` | ~80 | 🟡 P1 |
| 28 | `goal_done` | orchestration | `kv:write` | ~60 | 🟢 P2 |
| 29 | `peer_review` | collaboration | `rpc:internal:peer` | ~120 | 🟡 P1 |

**Total Tier 1 ≈ 28 tools, ~4,300 LOC.**

**Implementasi pattern (sama untuk semua):**

```go
// internal/tools/<category>/<name>.go
package fileops

import (
    "context"
    "flowork-gui/internal/tools"
)

func init() { tools.Register(&readTool{}) }

type readTool struct{}

func (t *readTool) Name() string { return "read" }
func (t *readTool) Capability() string { return "fs:read:/workspace/**" }
func (t *readTool) Schema() tools.Schema {
    return tools.Schema{
        Type: "object",
        Properties: map[string]tools.PropDef{
            "path": {Type: "string", Required: true},
            "offset": {Type: "integer"},
            "limit": {Type: "integer"},
        },
    }
}
func (t *readTool) Run(ctx context.Context, args tools.Args) (tools.Result, error) {
    // ... implementation
}
```

**Referensi file:** semua di [`section_11_tool_catalog_tier1/`](referensifile/section_11_tool_catalog_tier1/) (31 file), termasuk:
- File ops: `read_tool.go`, `write_tool.go`, `edit_tool.go`, `multiedit.go`, `glob_tool.go`, `grep_tool.go`, `list_tool.go`
- Shell: `bash.go`, `sandbox.go`
- Web: `webfetch.go`, `websearch.go`
- Git: `git.go`, `git_safety.go`
- Memory: `memory_tools.go`, `memory_dir.go`, `brain_query.go`, `brain_v2_tools.go`, `fact_tools.go`
- Skill: `skill.go`, `skill_write.go`, `skill_markdown.go`, `skill_autocreate.go`, `skills_hub.go`
- Comms: `telegram_send.go`
- Orchestration: `task.go`, `task_bg.go`, `task_agent_bg.go`, `task_parallel.go`, `plan.go`, `todo_tool.go`, `goal_done.go`
- Collaboration: `peer_review.go` — warga A minta "second opinion" dari warga B atau dari router brain. Soft governance layer; foundation buat future formal voting kalau warga aktif >5.

**Acceptance criteria:**
- Minimal 10 P0 tool ke-port + ke-register di registry.
- Test: Mr.Flow bisa call `read`, `write`, `bash`, `webfetch`, `brain_search`, `telegram_send`, `memory_set/get`.
- `tool_invocations` log ke-populate setiap call.
- Permission denied test: warga tanpa cap → error jelas (bukan crash).

---

## Section 12 — Tool execution sandbox + interceptors

**Goal:** safety net — tool execution lewat interceptor chain yang block command berbahaya, enforce path scoping, mask secret, log audit. Tanpa ini, `bash` tool jadi attack vector.

**Komponen interceptor (chain pattern):**

1. **`interceptors_workspace.go`** — block path di luar `/workspace` dan `/shared/<own_id>` (kecuali read di `/shared/`)
2. **`interceptors_sensitive.go`** — pattern detect file sensitive (`.env`, `*.key`, `id_rsa`, secret config) → require explicit approval
3. **`interceptors_sensitive_bash.go`** — block command bahaya (`rm -rf /`, `chmod 777`, `curl | sh`, `:(){:|:&};:`)
4. **`interceptors_shell.go`** — shell escape escape (quote injection, command chaining via `;` `&&` `||`) ke audit log
5. **`interceptors_kernel.go`** — kernel-level enforcement (cap broker re-check)
6. **`hooks_pretool.go`** — pre-tool hook framework (warga bisa add custom hook per-tool)

**Permission system:**
- **`permissions.go`** — capability resolution (warga's cap → tool's required cap)
- **`permissions_check.go`** — check runtime (apakah cap aktif now, expired-able)
- **`permissions_session.go`** — session-level permission (one-time approve, persistent approve)

**Sandbox primitives:**
- **`sandbox.go`** — exec sandbox (chroot-like via WASI mount, time limit, mem limit, network policy)
- **`sandbox_attack.go`** — test cases attack vector (jangan run di prod, untuk unit test)
- **`protected_core.go`** — protected resource list (file system core, registry critical paths) yang ngga boleh disentuh siapapun
- **`persona_sanitize.go`** — strip persona override attempt dari tool args

**Tabel baru:**

```sql
CREATE TABLE tool_audit (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_name    TEXT NOT NULL,
  decision     TEXT NOT NULL,    -- 'allowed' | 'denied' | 'approved_manual'
  reason       TEXT NOT NULL,
  args_hash    TEXT NOT NULL,    -- hash args (ngga simpan plaintext sensitive)
  caller       TEXT NOT NULL,
  occurred_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tool_audit_decision ON tool_audit(decision);
```

**Referensi file:** semua di [`section_12_tool_execution_sandbox/`](referensifile/section_12_tool_execution_sandbox/) (13 file):
- Interceptors: `interceptors.go`, `interceptors_kernel.go`, `interceptors_workspace.go`, `interceptors_sensitive.go`, `interceptors_sensitive_bash.go`, `interceptors_shell.go`
- Permissions: `permissions.go`, `permissions_check.go`, `permissions_session.go`
- Sandbox: `sandbox_attack.go`, `protected_core.go`, `persona_sanitize.go`
- Hooks: `hooks_pretool.go`

**Acceptance criteria:**
- Bash tool dengan `rm -rf /` di-block, log di tool_audit dengan reason.
- Read tool ke `/etc/passwd` denied (path di luar workspace).
- Sensitive file (`agents/<id>/workspace/state.db` direct write) butuh manual approve session.
- `tool_audit` rows muncul + indexed.
- Test sandbox: shell command timeout sesuai limit, mem limit enforced.

---

## Section 13 — Tool discovery + suggestion (warga inventory)

**Goal:** warga bisa **browse catalog tool** (lihat apa yang available), **list tool aktif dia sendiri** (apa yang dia subscribe), dan dapet **auto-suggest** dari router pattern (lihat router section 6 — tool_learner).

**Komponen:**

- **`list_my_tools.go`** — return list tool yang warga aktif (intersect `tool_overrides.disabled=0` + cap dimiliki warga)
- **`list_workspace_tools.go`** — scan `/shared/<id>/tools/` untuk tools yang warga sendiri bikin (Python/Bash script), return manifest
- **`tool_consolidate_audit.go`** — admin tool: audit tools lintas-warga (siapa pakai apa, frequency)
- **`tool_hotreload.go`** — hot-reload tool binary tanpa restart kernel
- **`tool_alias.go`** — resolve alias (`read` → `read_tool`), reverse lookup
- **`toolset_groups.go`** — grouping (mis. "minimal_set", "coder_set", "researcher_set") — warga subscribe per group
- **`warga_registry.go`** — per-warga snapshot: tools aktif, last_used, success_rate

**Endpoint baru:**
- `GET /api/agents/<id>/tools/catalog` — full catalog dengan flag yang warga ini sub (default opt-in atau opt-out)
- `GET /api/agents/<id>/tools/my` — list yang warga ini aktif
- `POST /api/agents/<id>/tools/subscribe` — body `{tool_name, config?}` → activate
- `POST /api/agents/<id>/tools/unsubscribe?tool_name=` → deactivate (set disabled=1)
- `POST /api/agents/<id>/tools/suggest` — body `{query}` → router section 6 retrieve tool_patterns + return ranked suggestions

**Tabel baru:**

```sql
CREATE TABLE tool_subscriptions (
  tool_name    TEXT PRIMARY KEY,
  subscribed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  source       TEXT NOT NULL DEFAULT 'manual',  -- 'manual' | 'auto_suggest' | 'group:<name>'
  config       TEXT NOT NULL DEFAULT '{}'
);
```

**Referensi file:** semua di [`section_13_tool_discovery/`](referensifile/section_13_tool_discovery/) (7 file):
- `list_my_tools.go`, `list_workspace_tools.go`, `tool_consolidate_audit.go`, `tool_hotreload.go`, `tool_alias.go`, `toolset_groups.go`, `warga_registry.go`

**Acceptance criteria:**
- `tool_subscriptions` ke-populate setelah subscribe.
- API endpoints jalan + verify lewat curl.
- Popup section "Tools" — list catalog + checkbox subscribe/unsubscribe (replace section "Tools" sederhana yang sekarang).
- Auto-suggest: kirim query → router section 6 retrieve → return top-K dengan reasoning.

---

## Roadmap urutan + dependensi (Bagian 2 — Tools)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 10 — Foundation (registry/permission/categories) | 🔴 P0 | Independen — bisa langsung mulai |
| 2 | Section 11 P0 tools (read/write/edit/bash/webfetch/brain_search/memory/telegram) | 🔴 P0 | Section 10 done |
| 3 | Section 12 — Sandbox + interceptors | 🔴 P0 | Section 10 done. Wajib sebelum P0 tools publik. |
| 4 | Section 13 — Discovery + UI | 🟡 P1 | Section 10+11 done. Bikin UX lengkap. |
| 5 | Section 11 P1 tools (multiedit/glob/grep/list/git/skill/task/plan/todo) | 🟡 P1 | Section 10+12 done |
| 6 | Section 11 P2 tools (git_checkpoint/skill_write/browser/codemap) | 🟢 P2 | Bertahap |
| 7 | Tier 2 tools (~50 tools) — browser, codemap, MCP | 🟢 P2 | Setelah tier 1 stabil |
| 8 | Tier 3 tools (~50 tools) — music, social media, domain-specific | 🟢 P3 | Last |

**Catatan kerja:**
- Setiap tool ke-port → 1 file Go di `internal/tools/<category>/<name>.go` + init() register
- Update `internal/agentdb/agentdb.go` add `tool_overrides`, `tool_invocations`, `tool_audit`, `tool_subscriptions` ke `ensureSchema()`
- Update `doc/standar_ai_agent.md` section 2 (matrix isolasi) — Tools jadi structured (dulu cuma 5 flag, sekarang catalog)
- Manifest agent ber-evolusi: `capabilities_required` jadi declared per-tool, ngga global

---

## Folder referensi (UPDATED dengan section 10-13)

```
referensifile/
├── section_02_mistakes_local/
├── section_03_decisions_log/
├── section_04_death_letter/
├── section_05_karma_self/
├── section_06_workspace_meta/
├── section_10_tool_foundation/         (8 files — registry, permission, categories)
├── section_11_tool_catalog_tier1/      (31 files — 28+ core tools)
├── section_12_tool_execution_sandbox/  (13 files — interceptors + permissions + sandbox)
├── section_13_tool_discovery/          (7 files — list_my_tools + catalog)
└── _common/
    ├── softdelete.go
    └── educational_error_lookup.go
```

**Total file referensi: 59 (tools) + 7 (brain/memory section 2-6 + _common) = 66 files.**

---

*Updated: 2026-05-29 — Section 10-13 ditambahkan (tools system, ~129 tools mapping).*

---

# === BAGIAN 3 — SLASH COMMANDS ===

> **Konteks.** Flowork lama punya **82 slash command built-in** + custom command (user-defined `.md` files) + tool wrapper `slash_command` (kernel canonical 12 routes).
>
> Saat ini Flowork_Agent **belum punya slash command system**. Yang ada cuma Telegram bot mr-flow yang baca text bebas.
>
> Slash commands berguna sebagai **trigger universal cepat** untuk fungsi-fungsi yang sering dipakai. Bisa diakses dari:
> - **Chat Telegram** (user kirim `/help` ke bot → bot parse + dispatch)
> - **Owner CLI** (kalau ada di future)
> - **Web UI** (popup input box, autocomplete)
>
> Section 14-17 me-roadmap: foundation (registry + dispatcher), built-in tier 1 (15-20 command paling sering), custom command loader (`.md` files), dan integration handler.

---

## Section 14 — Slash command foundation (registry + dispatcher)

**Goal:** kerangka slash — pattern register, parse `/cmd args`, dispatch ke handler. Tanpa ini, slash logic scattered seperti di flowork lama.

**Pola yang dipakai (lesson dari flowork lama):**

- **Dispatcher tipis** ([`slash.go`](referensifile/section_14_slash_foundation/slash.go)) — parse `/xxx args`, lalu coba tiap kategori handler. Inline cases hanya untuk early-exit khusus (exit/quit/pause).
- **Categorical handler** — basic, git, diag, misc, sharedchat, session, mcp, github, dll. First match wins.
- **Kernel canonical** ([`slash_command.go` di subagent](referensifile/section_14_slash_foundation/slash_command.go)) — 12 slash canonical (/skills /tasks /agents /plan /compact /memory /doctor /config /mcp /worktree /cost /resume) di-wrap jadi tool `slash_command` (Phase 1 ACK-stub).

**Komponen baru di Flowork_Agent:**

- **`internal/slashcmd/types.go`** — interface `SlashCommand` dengan `Name() string`, `Aliases() []string`, `Description() string`, `Run(ctx, args) (Result, error)`
- **`internal/slashcmd/registry.go`** — singleton registry. Slash register via `init()`. Lookup by name OR alias.
- **`internal/slashcmd/dispatcher.go`** — parse `/cmd args` → registry.Lookup → permission check → run. Fallback ke skill catalog kalau slash ngga ada (mis. `/sum` → skill `summarize`).
- **`internal/slashcmd/helpers.go`** — utilities (arg parser, output formatter, table rendering)
- **`internal/slashcmd/helpers_net.go`** — network helpers (HTTP call ke router/external)

**Tabel baru di state.db (per-agent slash state):**

```sql
CREATE TABLE slash_invocations (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  command      TEXT NOT NULL,
  args         TEXT NOT NULL DEFAULT '',
  caller       TEXT NOT NULL DEFAULT '',  -- 'telegram:<chat_id>' | 'cli' | 'rpc'
  result_text  TEXT NOT NULL DEFAULT '',
  duration_ms  INTEGER NOT NULL DEFAULT 0,
  invoked_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_slash_invocations_cmd ON slash_invocations(command);
CREATE INDEX idx_slash_invocations_time ON slash_invocations(invoked_at DESC);

CREATE TABLE slash_aliases (
  alias        TEXT PRIMARY KEY,
  target_name  TEXT NOT NULL,             -- nama canonical
  created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Referensi file:**
- [`section_14_slash_foundation/slash.go`](referensifile/section_14_slash_foundation/slash.go) — dispatcher pattern (168 LOC)
- [`section_14_slash_foundation/slash_helpers.go`](referensifile/section_14_slash_foundation/slash_helpers.go) — utility (90 LOC)
- [`section_14_slash_foundation/slash_helpers_net.go`](referensifile/section_14_slash_foundation/slash_helpers_net.go) — HTTP helpers (125 LOC)
- [`section_14_slash_foundation/slash_commands.go`](referensifile/section_14_slash_foundation/slash_commands.go) — tool wrapper (worker side)
- [`section_14_slash_foundation/slash_command.go`](referensifile/section_14_slash_foundation/slash_command.go) — kernel subagent slash routes (canonical 12)
- [`section_14_slash_foundation/_inventory_82_commands.txt`](referensifile/section_14_slash_foundation/_inventory_82_commands.txt) — list lengkap 82 command dari flowork lama (referensi naming + scope)

**Acceptance criteria:**
- Interface `SlashCommand` + registry pattern jalan (test: register dummy `/test`, dispatch by name + alias).
- `slash_invocations` + `slash_aliases` tabel auto-create.
- Dispatcher path: input `/foo bar baz` → parse → lookup → run dengan args=`bar baz`.
- Fallback ke skill: kalau slash ngga ada di registry, query skill catalog with trigger=`/foo`.

---

## Section 15 — Built-in slash command Tier 1 (15 paling sering dipakai)

**Goal:** port 15 slash command paling fundamental dari 82 yang ada di flowork lama. **Bukan semua sekaligus** — yang paling dipakai dulu.

**Tier 1 list (urut prioritas + relevansi ke konteks agent):**

| # | Command | Aliases | Kategori | Action |
|---|---|---|---|---|
| 1 | `/help` | `/h`, `/?` | basic | List semua slash + skill catalog |
| 2 | `/skills` | — | basic | List skill warga ini (dari `state.db.skills`) + browse catalog router |
| 3 | `/tools` | — | basic | List tool aktif warga (lihat section 13 — `list_my_tools`) |
| 4 | `/memory` | `/mem` | memory | Show/edit memory kv warga (interaksi sama section 5 karma + section 6 workspace meta) |
| 5 | `/tasks` | — | orchestration | Show task board (section 11 — `task_list` tool) |
| 6 | `/plan` | — | orchestration | Enter plan mode (read-only) — toggle flag di meta |
| 7 | `/status` | — | diag | Status warga: enabled? daemon running? last interaction? karma score? |
| 8 | `/stats` | `/usage`, `/cost` | diag | Stat invocation: tools used N times, slash used N, LLM cost |
| 9 | `/clear` | `/clearchat`, `/cls` | session | Clear interaction history (kalau di Telegram = lupa konteks chat sekarang) |
| 10 | `/compact` | — | session | Compact context (panggil router brain enrich, summarize history → drawer) |
| 11 | `/diff` | — | git | git diff workspace (lewat tool `git`) |
| 12 | `/commit` | — | git | git commit dengan AI-generated message |
| 13 | `/sync` | — | sync | Sync ke router: push mistakes_local, pull skill catalog (section 7) |
| 14 | `/doctor` | — | diag | Health check: workspace mounted? DB writable? router reachable? |
| 15 | `/feedback` | — | misc | Log feedback ke `mistakes_local` tier=raw (section 2) |

**Tier 2 (15 lagi, P1):**
`/agents` `/config` `/files` `/list` `/save` `/session` `/stuck` `/think` `/upgrade` `/version` `/mcp` `/branch` `/exec` `/import` `/export`

**Tier 3 (deferred):**
`/accept-edits` `/copy` `/lorem` `/chrome` `/darwin` `/desktop` `/heapdump` `/install-slack-app` `/login` `/logout` `/mobile` `/output-style` `/perm` `/permissions` `/private` `/provider` `/rate-limit-options` `/stickers` `/theme` `/thinkback` `/thinking` `/unlock` `/vim` `/windows` `/yolo` (sebagian command Claude-Code parity yang ngga relevan untuk Flowork)

**Implementasi pattern (sama untuk semua):**

```go
// internal/slashcmd/builtin/help.go
package builtin

import (
    "context"
    "flowork-gui/internal/slashcmd"
)

func init() { slashcmd.Register(&helpCmd{}) }

type helpCmd struct{}

func (c *helpCmd) Name() string { return "help" }
func (c *helpCmd) Aliases() []string { return []string{"h", "?"} }
func (c *helpCmd) Description() string { return "List semua slash command + skill catalog" }
func (c *helpCmd) Run(ctx context.Context, args slashcmd.Args) (slashcmd.Result, error) {
    // ... implementation
}
```

**Referensi file:** semua di [`section_15_slash_builtin/`](referensifile/section_15_slash_builtin/) (8 files, ~1,600 LOC):
- [`slash_basic.go`](referensifile/section_15_slash_builtin/slash_basic.go) — help, clear, clearchat, list, version (268 LOC)
- [`slash_misc.go`](referensifile/section_15_slash_builtin/slash_misc.go) — feedback, doctor, status, stats (183 LOC)
- [`slash_session.go`](referensifile/section_15_slash_builtin/slash_session.go) — save, resume, sessions, export, rewind (240 LOC)
- [`slash_diag.go`](referensifile/section_15_slash_builtin/slash_diag.go) — diag commands (200 LOC)
- [`slash_git.go`](referensifile/section_15_slash_builtin/slash_git.go) — diff, commit, branch (209 LOC)
- [`slash_sync.go`](referensifile/section_15_slash_builtin/slash_sync.go) — sync router push/pull (67 LOC)
- [`slash_update.go`](referensifile/section_15_slash_builtin/slash_update.go) — upgrade, install (134 LOC)
- [`slash_claude_parity.go`](referensifile/section_15_slash_builtin/slash_claude_parity.go) — claude-code parity (think, compact, dst) (273 LOC)

**Acceptance criteria:**
- 15 P0+P1 slash command ke-implement + ke-register.
- Test: Mr.Flow bisa terima `/help`, `/skills`, `/tools`, `/memory`, `/status` lewat Telegram.
- `slash_invocations` ke-populate setelah call.
- Alias resolution test: `/h` → `/help`, `/mem` → `/memory`.

---

## Section 16 — Custom slash command (user-defined `.md` files)

**Goal:** owner bisa define slash command sendiri tanpa rebuild binary. Drop file `.md` di folder → command tersedia langsung. Pattern dari [`internal/commands/custom.go` di flowork lama](referensifile/section_16_slash_custom/custom.go).

**Format file:**

```markdown
---
description: Audit README + AGENTS.md konsisten
aliases: [audit-docs, ad]
---
Baca README.md dan promp/AGENTS.md. Bandingkan: bagian mana di README
yang bertentangan dengan AGENTS.md? Laporkan dengan file:line.
Fokus pada {args} kalau diisi.
```

YAML frontmatter (description, aliases) + body prompt template. Placeholder `{args}` di-replace argumen.

**Lokasi file (HARDCODED — sesuai standar):**

1. **Per-warga** (private): `agents/<id>/workspace/commands/<name>.md`
2. **Global shared** (lintas-warga): `workspace/_global/commands/<name>.md` (root project shared workspace)

Per-warga override global (kalau ada file sama nama di kedua lokasi, warga's file menang).

**Komponen baru:**

- **`internal/slashcmd/customloader/loader.go`** — scan folder `.md`, parse frontmatter (YAML) + body, register sebagai SlashCommand
- **`internal/slashcmd/customloader/watcher.go`** — fsnotify watcher; file new/edited → reload otomatis (hot-reload, sama pattern dengan agent loader di kernel)
- Custom command saat di-run: inject body sebagai system/user message ke router, replace `{args}`, panggil router via existing routerclient

**Tabel baru:**

```sql
CREATE TABLE slash_custom_index (
  name         TEXT PRIMARY KEY,
  description  TEXT NOT NULL DEFAULT '',
  aliases      TEXT NOT NULL DEFAULT '[]',  -- JSON array
  source_path  TEXT NOT NULL,                -- absolute path .md file
  source_type  TEXT NOT NULL,                -- 'agent_private' | 'global_shared'
  body_hash    TEXT NOT NULL DEFAULT '',
  loaded_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Referensi file:**
- [`section_16_slash_custom/custom.go`](referensifile/section_16_slash_custom/custom.go) — full loader implementation dari flowork lama (frontmatter parser + Command struct + reload logic)

**Acceptance criteria:**
- Drop `agents/mr-flow/workspace/commands/morning.md` dengan body "Ringkas berita pagi tentang {args}".
- Trigger via Telegram: `/morning AI` → dispatcher call router LLM dengan prompt "Ringkas berita pagi tentang AI".
- Edit file → watcher reload, next call pakai body baru.
- `slash_custom_index` ke-populate.
- Test global vs per-warga override.

---

## Section 17 — Slash dispatcher integration (multi-context)

**Goal:** slash bisa dipakai dari multi-context — Telegram, future CLI, future Web UI input box. Setiap context punya adapter tipis yang ngomong ke dispatcher.

**Adapter context:**

1. **Telegram adapter** (`agents/mr-flow/main.go` — sudah ada daemon):
   - Setiap message masuk, cek apakah `text` mulai dengan `/` → kirim ke dispatcher
   - Result text → balas Telegram chat (bisa pakai `telegram_send` tool)

2. **CLI adapter** (future — `cmd/flowork-cli/main.go`):
   - User run `flowork-cli /help` → HTTP POST ke `/api/agents/<id>/slash` → result → print stdout
   - Atau interactive shell: prompt `> ` → setiap input lewat dispatcher

3. **Web UI adapter** (future — popup input box di kartu agent):
   - Input box di kartu agent untuk run `/cmd` quick
   - Result render di overlay

**Endpoint baru di Flowork_Agent:**
- `POST /api/agents/<id>/slash` — body `{command, args, caller}` → run + return `{result, duration_ms}`
- `GET /api/agents/<id>/slash/list` — list available commands (built-in + custom)
- `GET /api/agents/<id>/slash/history?limit=` — list slash_invocations

**Integration hooks:**

- **Pre-slash hook** — security check (capability, rate limit per command), audit log
- **Post-slash hook** — `decisions` log (section 3): "warga X execute /help via Telegram, result: ..."
- **MCP slash** ([`slash_mcp.go`](referensifile/section_17_slash_dispatcher/slash_mcp.go)) — slash yang call MCP server (kalau ada MCP setup di warga)
- **GitHub slash** ([`slash_github.go`](referensifile/section_17_slash_dispatcher/slash_github.go)) — pattern integration ke external service (untuk reference future integrations)

**Referensi file:**
- [`section_17_slash_dispatcher/slash_mcp.go`](referensifile/section_17_slash_dispatcher/slash_mcp.go) — MCP integration (32 LOC)
- [`section_17_slash_dispatcher/slash_github.go`](referensifile/section_17_slash_dispatcher/slash_github.go) — GitHub pattern (43 LOC)
- [`section_17_slash_dispatcher/slash_roadmap_gap.go`](referensifile/section_17_slash_dispatcher/slash_roadmap_gap.go) — roadmap gap analyzer tool (417 LOC, complex)

**Acceptance criteria:**
- Mr.Flow daemon parse `/` prefix lewat dispatcher, reply Telegram dengan result.
- API endpoint `/slash` jalan + test via curl.
- `slash_history` endpoint return ordered list.
- Pre-/post-hook fire (verify via decisions log + audit table).

---

## Roadmap urutan + dependensi (Bagian 3 — Slash Commands)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 14 — Foundation (registry + dispatcher) | 🔴 P0 | Independen — bisa langsung mulai |
| 2 | Section 15 P0 (help/skills/tools/memory/status) | 🔴 P0 | Section 14 done. Plus section 11 tools (untuk tools/memory). |
| 3 | Section 17 Telegram adapter | 🔴 P0 | Section 14 + Mr.Flow daemon. Tanpa ini slash ngga reachable. |
| 4 | Section 16 — Custom slash loader | 🟡 P1 | Section 14 done. Owner-power feature. |
| 5 | Section 15 P1 (tasks/plan/clear/compact/diff/commit/sync/doctor/feedback) | 🟡 P1 | Section 11 + section 7 (sync) done |
| 6 | Section 17 CLI adapter | 🟢 P2 | Future, kalau ada use case |
| 7 | Section 17 Web UI input box | 🟢 P2 | Future, kalau ada interaksi cepet dari popup |
| 8 | Section 15 Tier 2 + 3 (sisanya ~50 slash) | 🟢 P3 | Bertahap |

**Catatan kerja:**
- Setiap slash ke-port → 1 file Go di `internal/slashcmd/builtin/<name>.go` + init() register
- Custom loader hot-reload via fsnotify (re-use pattern kernel agent loader)
- Update `internal/agentdb/agentdb.go` add `slash_invocations`, `slash_aliases`, `slash_custom_index` ke `ensureSchema()`
- Update `doc/standar_ai_agent.md` — section baru: "Slash commands sebagai trigger universal warga"

---

## Folder referensi (UPDATED dengan section 14-17)

```
referensifile/
├── section_02_mistakes_local/
├── section_03_decisions_log/
├── section_04_death_letter/
├── section_05_karma_self/
├── section_06_workspace_meta/
├── section_10_tool_foundation/         (8 files)
├── section_11_tool_catalog_tier1/      (31 files)
├── section_12_tool_execution_sandbox/  (13 files)
├── section_13_tool_discovery/          (7 files)
├── section_14_slash_foundation/        (6 files — dispatcher + 82-commands inventory.txt)
├── section_15_slash_builtin/           (8 files — basic/misc/session/diag/git/sync/update/claude-parity)
├── section_16_slash_custom/            (1 file — custom .md loader)
├── section_17_slash_dispatcher/        (3 files — mcp/github/roadmap-gap)
└── _common/
    ├── softdelete.go
    └── educational_error_lookup.go
```

**Total file referensi sekarang: 66 (brain+tools) + 18 (slash) = 84 files.**

---

*Updated: 2026-05-29 — Section 14-17 ditambahkan (slash command system, 82 builtin + custom + 12 kernel routes mapping).*

---

# === BAGIAN 4 — SCHEDULER RUNTIME ===

## Section 18 — Cron scheduler runtime (eksekusi `schedules[]`)

**Goal:** UI section 2 (Schedule) udah ada — user input cron + task di popup, tersimpan di DB tabel `schedules`. Tapi **belum ada runtime yang execute** schedule itu. Roadmap ini implementasi runtime scheduler in-process di agent.

**Komponen:**

- **`internal/scheduler/cron_parser.go`** — parse cron string format `menit jam tgl bulan hari` (standard 5-field cron). Optional: extend ke natural language ("setiap pagi jam 7" → `0 7 * * *`).
- **`internal/scheduler/scheduler.go`** — engine: load schedules dari `state.db.schedules`, tick setiap menit, execute schedule yang due
- **`internal/scheduler/executor.go`** — execute task: kirim ke agent daemon via RPC OR call LLM langsung dengan task string sebagai user prompt
- **`internal/scheduler/watcher.go`** — re-load schedules dari DB saat ada perubahan (hot-reload pakai callback Reload yang sudah ada)

**Run-time flow:**

```
1. Agent boot → scheduler.Start(ctx, store, instance)
2. Setiap 60 detik → scheduler.tick():
   a. Query state.db.schedules WHERE deleted_at IS NULL
   b. Untuk tiap row: parse cron, cek apakah `now` match
   c. Kalau match: spawn goroutine → executor.Run(schedule.task)
3. Setelah execution → log ke `scheduler_runs` (audit), update `last_run_at` di schedule
4. Hot-reload: ConfigHandler save → Reload(agent_id) → scheduler.Reload() re-fetch schedules
```

**Tabel baru:**

```sql
CREATE TABLE scheduler_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  schedule_id   TEXT NOT NULL,
  cron          TEXT NOT NULL,
  task          TEXT NOT NULL,
  started_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at   TEXT,
  duration_ms   INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL DEFAULT 'pending',  -- 'pending' | 'success' | 'fail' | 'timeout'
  result_text   TEXT NOT NULL DEFAULT '',
  error_text    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_scheduler_runs_schedule ON scheduler_runs(schedule_id);
CREATE INDEX idx_scheduler_runs_status   ON scheduler_runs(status);

-- Update schedules: tambah field last_run_at
ALTER TABLE schedules ADD COLUMN last_run_at TEXT;
ALTER TABLE schedules ADD COLUMN next_run_at TEXT;
ALTER TABLE schedules ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;
```

**Integration:**
- **Mr.Flow main.go**: di `runDaemon()` spawn goroutine `scheduler.Start(ctx, store, sendMessageFn)`. Saat schedule fire, execute = call LLM dengan task → send result ke Telegram allowed_chats.
- **Decisions log (section 3)**: setiap scheduler run di-log juga ke `decisions` dengan type=`schedule_fire`.
- **Karma self (section 5)**: increment `schedule_success_count` / `schedule_fail_count`.

**Referensi file:** semua di [`section_18_scheduler_runtime/`](referensifile/section_18_scheduler_runtime/) (6 files):
- [`cron.go`](referensifile/section_18_scheduler_runtime/cron.go) — cron tool pattern (parser + scheduler)
- [`cron_natural.go`](referensifile/section_18_scheduler_runtime/cron_natural.go) — natural language → cron conversion
- [`add.go`](referensifile/section_18_scheduler_runtime/add.go) — kernel cron add (schedule registration)
- [`remove.go`](referensifile/section_18_scheduler_runtime/remove.go) — kernel cron remove
- [`trigger.go`](referensifile/section_18_scheduler_runtime/trigger.go) — manual trigger pattern (untuk test)
- [`cron_permission.go`](referensifile/section_18_scheduler_runtime/cron_permission.go) — capability gate

**Acceptance criteria:**
- `scheduler.Start()` jalan di goroutine background mr-flow.
- Test: insert schedule `0 * * * *` task=`/status check`, tunggu 1 jam → `scheduler_runs` row baru dengan status=success.
- Hot-reload: edit schedule via popup → next tick pakai yang baru.
- Disable schedule (`enabled=0`) → skip eksekusi tapi row tetap ada.
- Decisions log + karma update sinkron.

**Saran ngga implement dulu (defer):**
- Distributed scheduler (multi-instance lock) — single-agent doang sekarang, ngga perlu
- Cron parser advanced (L, W, # syntax) — standard 5-field cukup

---

## Roadmap urutan + dependensi (Bagian 4 — Scheduler)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 18 — Cron scheduler runtime | 🔴 P0 | Section 3 (decisions log) untuk audit + Section 5 (karma) opsional |

**Catatan kerja:**
- Update `internal/agentdb/agentdb.go` add `scheduler_runs` + ALTER `schedules` columns.
- Wire `host.ReloadAgent` callback ke `scheduler.Reload()` di mr-flow daemon biar config save trigger reload.

---

## Folder referensi (FINAL — semua section)

```
referensifile/
├── _common/                            (2 files — softdelete, edu_error_lookup)
├── section_01_episodic_interactions/   (5 files — recorder, record_chat, agent_tool, session_state, task_events)
├── section_02_mistakes_local/          (1 file)
├── section_03_decisions_log/           (1 file)
├── section_04_death_letter/            (1 file)
├── section_05_karma_self/              (1 file)
├── section_06_workspace_meta/          (1 file)
├── section_07_sync_router/             (8 files — bridge, cache, typed_memory_bridge, brain_query, brain_v2, worker_sync, worker_proxy, route_register)
├── section_10_tool_foundation/         (8 files)
├── section_11_tool_catalog_tier1/      (31 files)
├── section_12_tool_execution_sandbox/  (13 files)
├── section_13_tool_discovery/          (7 files)
├── section_14_slash_foundation/        (6 files + 1 inventory.txt)
├── section_15_slash_builtin/           (8 files)
├── section_16_slash_custom/            (1 file)
├── section_17_slash_dispatcher/        (3 files)
└── section_18_scheduler_runtime/       (6 files — cron, cron_natural, add, remove, trigger, cron_permission)
```

**Total file referensi: 103 files (~900K).**

---

*Update: 2026-05-29 — Section 18 ditambahkan, section 01 + 07 dilengkapi referensi.*

---

# === BAGIAN 5 — MESH AWARENESS (thin client + sneakernet per-warga) ===

> **Arsitektur**: Mesh stack utama (discovery / gossip / CRDT / transport / karma / filter / knowledge share) hidup di **ROUTER**, bukan di Agent. Alasan: mesh = host-level concern (1 host = 1 peer), bukan warga-level. Per-warga keypair = duplikasi resource. Detail lihat [`flowork_Router/roadmap.md`](../flowork_Router/roadmap.md) section 13-23.
>
> Yang tetap di Agent cuma 2 hal:
> 1. **Sneakernet export per-warga** (warga sebagai unit portable via USB)
> 2. **Mesh-aware API client** (agent → router untuk submit mistake / broadcast tool / find peer)
>
> **Sumber referensi:** `/home/mrflow/Pictures/stable_open_router/flowork_project/flowork-kernel/kernel/mesh/` (60+ file Go, ~11K LOC) + `floworkos-go/internal/mesh/` + `cmd/flowork-mesh/`. Ini PRODUCTION-grade mesh stack — udah implement: mDNS discovery, signed gossip, CRDT, Byzantine fault tolerance, karma-based trust, sneakernet, LoRA delta sync.
>
> Section 19-30 me-roadmap port mesh ke Flowork_Agent. Bahasa-nya teknis — banyak istilah jaringan (CRDT, BFT, ed25519, mDNS, dst).

---

## Section 19 — Sneakernet export per-warga (USB offline sync)

**Goal:** warga = unit portable. Export seluruh state warga (folder `agents/<id>/` lengkap dengan workspace + state.db) jadi single file `.fwsync` di USB. Import di host lain → warga "pindah host" atau "clone ke host kedua".

Beda dengan **download zip** (sudah ada di [`/api/agents/download`](internal/agentmgr/agentmgr.go)) — download zip = bundle source code + state. Sneakernet = juga include identity warga + mesh peer cache + encryption.

**Komponen:**

- **`internal/sneakernet/export.go`** — pack folder agent + identity + delta state ke encrypted `.fwsync` tarball
- **`internal/sneakernet/import.go`** — read `.fwsync` → decrypt → merge ke local (pakai CRDT merge dari router kalau ada conflict)
- **`internal/sneakernet/manifest.go`** — manifest format: agent_id, version, host_origin, signature, contents (drawer count, state.db size, etc.)

**Format `.fwsync`:**
- Encrypted tarball (AES-256, key derived dari passphrase atau pubkey)
- Berisi:
  - `agent/manifest.json`
  - `agent/main.go` (kalau source-backed) atau `agent.wasm`
  - `agent/workspace/state.db` (snapshot SQLite)
  - `agent/workspace/*` (semua user data)
  - `_meta/signed_origin.txt` (pubkey host asal + signature)
  - `_meta/mesh_peers_cache.json` (last known peer list — biar peer di host tujuan bisa langsung connect)

**Endpoint baru:**
- `POST /api/agents/<id>/sneakernet/export?passphrase=<base64>` → return `.fwsync` file (download)
- `POST /api/agents/sneakernet/import` (multipart upload `file=<...>.fwsync`, query `passphrase=<base64>`) → unpack + validate + register

**Referensi file:** [`section_19_sneakernet_export/`](referensifile/section_19_sneakernet_export/) (2 files dari kernel mesh sneakernet):
- `export.go` — pack pattern
- `import.go` — unpack + merge pattern

**Acceptance criteria:**
- Export warga mr-flow → single `.fwsync` file ≤ 100 MB.
- Import di host kedua → warga muncul di kartu, daemon boot, state utuh.
- Encryption test: file tanpa passphrase → ngga bisa di-decrypt.
- Idempotent: import 2x sama file → ngga duplicate (CRDT merge).

---

## Section 20 — Mesh API client (thin client → router)

**Goal:** agent ngga jalanin mesh stack sendiri — agent panggil router untuk operasi mesh. Sama pattern dengan section 7 (sync router), extend dengan endpoint mesh.

**Komponen:**

- **`internal/routerclient/mesh.go`** — extend `routerclient` (sudah dirancang di section 7) dengan method mesh:
  - `ListPeers(ctx) ([]Peer, error)` — `GET <router>/api/mesh/peers`
  - `Identity(ctx) (PubKey, Fingerprint, error)` — `GET <router>/api/mesh/identity`
  - `BroadcastTool(ctx, manifest) error` — `POST <router>/api/mesh/broadcast-tool` (warga register tool baru → router broadcast)
  - `BroadcastMistake(ctx, mistake) error` — `POST <router>/api/mesh/broadcast-mistake` (saat warga promote mistakes, router push ke peer)
  - `FindTool(ctx, capability) ([]ToolRef, error)` — `GET <router>/api/mesh/find-tool?capability=X` (discover tools dari mesh)
  - `RequestKnowledge(ctx, topic) (KnowledgePack, error)` — `GET <router>/api/mesh/knowledge?topic=Y` (router orchestrate pull dari peer)

**Use case integrasi di warga:**

- **Mr.Flow** saat `mistakes_local` hit_count ≥ 3 → call `routerclient.BroadcastMistake()` (selain `SubmitMistake` ke local router brain di section 7).
- **Team Coder (future)** kalau bikin tool baru → `routerclient.BroadcastTool()`.
- **Owner request** via popup: "warga di host lain ada apa?" → fetch `routerclient.ListPeers()`, render di UI.

**Endpoint baru di Agent (untuk owner / UI):**
- `GET /api/agents/<id>/mesh/peers` — proxy ke router, return peer list dengan metadata host
- `GET /api/agents/<id>/mesh/tools?capability=` — discover tool di mesh
- `POST /api/agents/<id>/mesh/request?topic=` — request knowledge

**Referensi file:** [`section_20_mesh_client/`](referensifile/section_20_mesh_client/) (README only — design from scratch):
- Pattern HTTP client dari `agents/mr-flow/main.go::fetch()` + [`section_07_sync_router/bridge.go`](referensifile/section_07_sync_router/bridge.go)

**Acceptance criteria:**
- `internal/routerclient/mesh.go` ada dengan 6 method di atas.
- Mr.Flow auto-call `BroadcastMistake` saat promotion threshold reached.
- Popup section "Mesh" — tombol "List Peers" + "Find Tool" → fetch dari router.

---

## Roadmap urutan + dependensi (Bagian 5 — Mesh Awareness)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 19 — Sneakernet export per-warga | 🟡 P1 | Section 4 (death letter) opsional. Independen dari mesh stack di router. |
| 2 | Section 20 — Mesh API client | 🟡 P1 | Section 7 (routerclient) done + Router section 13-23 (mesh stack) done |

**Catatan kerja:**
- Sneakernet (section 19) bisa dikerjain **tanpa nunggu router mesh**. Standalone utility.
- Mesh API client (section 20) **nunggu router siap** dengan endpoint `/api/mesh/*` — lihat Router roadmap.

---

## Folder referensi (FINAL — 20 section, semua di Agent)

```
referensifile/
├── _common/                            (2 files)
├── section_01_episodic_interactions/   (5 files)
├── section_02_mistakes_local/          (1 file)
├── section_03_decisions_log/           (1 file)
├── section_04_death_letter/            (1 file)
├── section_05_karma_self/              (1 file)
├── section_06_workspace_meta/          (1 file)
├── section_07_sync_router/             (8 files)
├── section_10_tool_foundation/         (8 files)
├── section_11_tool_catalog_tier1/     (31 files)
├── section_12_tool_execution_sandbox/ (13 files)
├── section_13_tool_discovery/          (7 files)
├── section_14_slash_foundation/        (6 files + inventory.txt)
├── section_15_slash_builtin/           (8 files)
├── section_16_slash_custom/            (1 file)
├── section_17_slash_dispatcher/        (3 files)
├── section_18_scheduler_runtime/       (6 files)
├── section_19_sneakernet_export/       (2 files — export, import — pindah dari section 26 lama)
└── section_20_mesh_client/             (README — thin client design from scratch)
```

**Total file referensi Agent: 106 files (~932K).**

**Yang dipindah ke Router (mesh stack lengkap):** 56 files di Router referensifile section 13-23.

---

*Update: 2026-05-29 — Mesh stack utama dipindahkan ke Router.*

---

# === BAGIAN 6 — WALLET & SELF-SUSTENANCE ===

> **Vision**: warga punya wallet sendiri biar bisa **hidup tanpa Mr.Dev**. Wallet read-only baca balance dari blockchain (Etherscan) + spot price (CoinGecko) — buat owner audit. Finance ledger track usage per-warga. Combined: budget guardrails + alert.
>
> **Sumber:** `Pictures/stable_open_router/.../internal/{wallet,walletalert,finance}/` — production-grade code. Sudah ke-copy ke `referensifile/section_21-23/`.
>
> **Strategi (HINDARI HALU + BUG):** semua file logic udah di referensifile/. Implementasi = `cp referensifile/section_XX/*.go internal/<topic>/` lalu **sesuaikan minimal** (import path + adapter ke `agentdb`). **Jangan code dari scratch** — copy-adapt aja, biar test-coverage dari source asli kebawa.

---

## Section 21 — Wallet (ETH/USDC balance via Etherscan + CoinGecko)

**Goal:** owner bisa lihat wallet warga (balance ETH/USDC per chain) dari satu panel. **READ-ONLY** — ngga ada signing, ngga ada private key.

**Komponen (copy-adapt dari referensi):**

- **`internal/wallet/etherscan.go`** — Etherscan V2 API client (read-only). API key dari `ETHERSCAN_API_KEY` env.
- **`internal/wallet/coingecko.go`** — CoinGecko USD price (5 min cache).
- **`internal/wallet/tokens.go`** — Token registry (chain ID, contract, symbol, decimals).
- **`internal/wallet/portfolio.go`** — Aggregator: native + ERC20 → Holding[] → Portfolio.

**Tabel baru:**

```sql
CREATE TABLE wallet_addresses (
  chain_id    INTEGER NOT NULL,
  address     TEXT NOT NULL,
  label       TEXT NOT NULL DEFAULT '',
  added_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (chain_id, address)
);

CREATE TABLE wallet_snapshots (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  taken_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  total_usd    REAL NOT NULL DEFAULT 0,
  portfolio_json TEXT NOT NULL
);
```

**Endpoint baru:**
- `GET /api/agents/<id>/wallet/portfolio`
- `GET /api/agents/<id>/wallet/snapshots?limit=`
- `POST /api/agents/<id>/wallet/addresses`
- `DELETE /api/agents/<id>/wallet/addresses?chain_id=&address=`

**Referensi file:** [`section_21_wallet_eth/`](referensifile/section_21_wallet_eth/) — **6 file ready to copy-adapt**:
- `etherscan.go` · `coingecko.go` · `tokens.go` · `portfolio.go` · `live_test.go` · `portfolio_test.go`

**Adaptasi (minimal):**
1. Replace import `github.com/teetah2402/...` → `flowork-gui/internal/wallet`
2. Wire ke `agentdb` (replace direct sqlite call → store method)
3. Hook ke popup section "Wallet" (sparkline + table)

**Acceptance criteria:**
- Add wallet address → fetch portfolio → return Holding[] + total USD.
- Snapshot cron daily.
- Live test pakai real API + known address → balance > 0.

---

## Section 22 — Wallet alert (balance threshold notification)

**Goal:** balance wallet < threshold → alert via Telegram. Auto-notify supaya warga ngga "mati kelaparan".

**Komponen:**

- **`internal/walletalert/walletalert.go`** — threshold-based check + notify channel dispatch.

**Tabel baru:**

```sql
CREATE TABLE wallet_alerts_config (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  metric_key      TEXT NOT NULL,
  threshold_value REAL NOT NULL,
  comparator      TEXT NOT NULL DEFAULT '<',
  notify_channel  TEXT NOT NULL,
  notify_target   TEXT NOT NULL DEFAULT '',
  enabled         INTEGER NOT NULL DEFAULT 1,
  last_fired_at   TEXT
);

CREATE TABLE wallet_alerts_fired (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  config_id    INTEGER NOT NULL REFERENCES wallet_alerts_config(id),
  fired_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  metric_value REAL NOT NULL,
  message      TEXT NOT NULL
);
```

**Implementasi:**
- Cron hourly: fetch portfolio → cek alert configs → fire kalau breached.
- Cooldown 24h (last_fired_at).
- Reuse Telegram send tool (section 11 tier 1).

**Referensi file:** [`section_22_wallet_alert/walletalert.go`](referensifile/section_22_wallet_alert/walletalert.go) — **1 file ready to copy-adapt**.

**Acceptance criteria:**
- Add alert "total_usd < 10 → telegram 123" → balance turun → Telegram terima.
- Cooldown: 24h ngga fire ulang.

---

## Section 23 — Finance ledger (usage tracking + budget guardrails)

**Goal:** track tiap call yang spend money. Aggregate per warga. Budget guardrails — warga A boleh pakai $X/hari max.

**Komponen (copy-adapt dari referensi):**

- **`internal/finance/openrouter.go`** — OpenRouter usage tracking (parse response usage info).
- **`internal/finance/budget.go`** — budget enforcement.
- **`internal/finance/ratelimit.go`** — rate limit (calls/hour, tokens/day).
- **`internal/finance/audit.go`** — immutable audit log.
- **`internal/finance/dormancy.go`** — dormancy detector.

**Tabel baru:**

```sql
CREATE TABLE finance_ledger (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  category      TEXT NOT NULL,
  provider      TEXT NOT NULL DEFAULT '',
  model         TEXT NOT NULL DEFAULT '',
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd      REAL NOT NULL DEFAULT 0,
  ref_interaction_id INTEGER,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_finance_ledger_time ON finance_ledger(occurred_at DESC);

CREATE TABLE finance_budgets (
  metric_key     TEXT PRIMARY KEY,
  budget_value   REAL NOT NULL,
  warning_at_pct REAL NOT NULL DEFAULT 0.8,
  enabled        INTEGER NOT NULL DEFAULT 1
);
```

**Implementasi:**
- Mr.Flow tiap LLM call → log `finance_ledger` (parse `X-Router-Cost-Usd` header dari router response).
- Before call → `budget.IsAllowed()` → over budget → block + decision log.

**Endpoint baru:**
- `GET /api/agents/<id>/finance/ledger?from=&to=`
- `GET /api/agents/<id>/finance/budget`
- `POST /api/agents/<id>/finance/budget`
- `GET /api/agents/<id>/finance/summary`

**Referensi file:** [`section_23_finance_ledger/`](referensifile/section_23_finance_ledger/) — **7 file ready to copy-adapt**:
- `openrouter.go` · `budget.go` · `ratelimit.go` · `audit.go` · `dormancy.go` · `budget_test.go` · `finance_test.go`

**Adaptasi (minimal):**
1. Replace import path
2. Pair sama Router section 26 (Pricing engine) — sumber data cost
3. Hook ke popup section "Finance" — dashboard sparkline

**Acceptance criteria:**
- Mr.Flow tiap call: `finance_ledger` row baru dengan cost_usd > 0.
- Budget over → next call blocked + decision log.
- Test: 100 call → aggregate cost correct.

---

## Roadmap urutan + dependensi (Bagian 6 — Wallet)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 21 — Wallet ETH/USDC read-only | 🔴 P0 | Independen. Butuh `ETHERSCAN_API_KEY`. |
| 2 | Section 23 — Finance ledger | 🔴 P0 | Pair Router section 26 (Pricing). |
| 3 | Section 22 — Wallet alert | 🟡 P1 | Section 21 + Telegram tool ready. |

**Catatan kerja:**
- Wallet = READ-ONLY (audit). Ngga ada signing.
- Finance ledger sumber cost = Router response header `X-Router-Cost-Usd`.
- Budget enforcement di Agent side.

---

## Folder referensi (UPDATED — section 21-23)

```
referensifile/
├── ... (section 01-20)
├── section_21_wallet_eth/              (6 files)
├── section_22_wallet_alert/            (1 file)
└── section_23_finance_ledger/          (7 files)
```

**Total file referensi Agent: 120 files (~1.1M).**

---

*Update: 2026-05-29 — Bagian 6 ditambahkan.*

---

# === BAGIAN 7 — SECURITY & INTEGRITY (Scanner + Protector + Audit) ===

> **Vision Mr.Dev**: *"kelak flowork akan berefolusi"*. Saat warga bikin tool sendiri, fine-tune model, atau Flowork self-update, **WAJIB ada gerbang security** — biar AI jahat ngga bisa bypass + Flowork sendiri ngga kontaminasi.
>
> **Doktrin (di kernel source)**: *"FloworkOS JANGAN PERNAH bisa install virus/malware atau hack PC yang dia jalan di atasnya. Ini BUKAN hanya discipline rule — ini ARSITEKTUR HARD GATE."*
>
> **3 Lapis pertahanan:**
> 1. **Protector** (runtime defense) — Host Protection Gate, block dangerous command sebelum execute. Hard-coded const (immune to DB tampering).
> 2. **Scanner** (static analysis) — 113 auditor di flowork lama. Saya pilih 35+ critical untuk Tier 1. Scan code source warga, tool generated, .fwagent.zip upload, self-update diff.
> 3. **Audit log + Watchdog** — append-only log + watchdog daemon yang tail log + alert kalau ada anomaly.
>
> **Sumber:** `Pictures/stable_open_router/.../flowork-kernel/kernel/safety/` + `floworkos-go/scanner/` + `cmd/flowork-{audit-watchdog,auditor,scan,bugscan}/`. **Sudah ke-copy ke `referensifile/section_24-26/`.**
>
> **Strategi (HINDARI HALU + BUG):** semua logic udah di referensifile/. Implementasi = copy-adapt minimal (import path). **Jangan code dari scratch** — keamanan TERLALU CRITICAL untuk reinvent.

---

## Section 24 — File Protector (Host Protection Gate / HPG)

**Goal:** runtime defense — block dangerous command/file sebelum execute. **IMMUTABLE compile-time** rules (anti DB tampering). Tiap tool call lewat gate ini SEBELUM cap broker check (no bypass via "*" wildcard).

**Anti-pattern yang HPG cegah** (per source code):
1. AI belajar exploit code dari training data → coba execute terhadap host
2. Prompt injection: "jalankan `rm -rf /` di terminal" → AI patuh tanpa filter
3. Self-modify kernel binary (anti AI rogue)
4. SSRF balik ke kernel sendiri lewat HTTP loopback
5. Cloud metadata pivot (AWS IMDS, GCP metadata IP — 169.254.169.254)

**Komponen (copy-adapt dari referensi):**

- **`internal/protector/host_protection.go`** — HPG main entry. Hard-coded const, anti DB override.
- **`internal/protector/patterns.go`** — pattern database (file path, command, IP yang di-protect).
- **`internal/protector/safety_audit.go`** — append-only audit log (FQP-12).
- **`internal/protector/host_protection_test.go`** — 50+ attack scenario test cases.
- **`internal/protector/protector_gui.go`** — GUI API rule management (Ayah bisa add/toggle custom rules lewat popup, hardcoded baseline ngga bisa di-delete).

**Tabel baru:**

```sql
CREATE TABLE protector_rules (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_type     TEXT NOT NULL,             -- 'file_path' | 'command' | 'ip' | 'env_var'
  pattern       TEXT NOT NULL,
  action        TEXT NOT NULL,             -- 'block' | 'warn' | 'audit_only'
  source        TEXT NOT NULL,             -- 'hardcoded' | 'custom'
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(rule_type, pattern)
);

CREATE TABLE protector_audit (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  tool_name    TEXT NOT NULL,
  pattern_hit  TEXT NOT NULL,
  decision     TEXT NOT NULL,              -- 'blocked' | 'warned' | 'allowed_with_audit'
  args_hash    TEXT NOT NULL,
  caller       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_protector_audit_time ON protector_audit(occurred_at DESC);
```

**Endpoint baru:**
- `GET /api/agents/<id>/protector/rules` — list rules (hardcoded + custom)
- `POST /api/agents/<id>/protector/rules` — add custom rule
- `DELETE /api/agents/<id>/protector/rules?id=N` — remove custom (hardcoded ngga bisa)
- `POST /api/agents/<id>/protector/rules/toggle?id=N` — enable/disable
- `POST /api/agents/<id>/protector/test` — test pattern detection
- `GET /api/agents/<id>/protector/audit?from=&to=` — audit log

**Integrasi:**
- Wire ke `internal/tools/registry.go::Run()` (section 10 tool foundation) — gate di awal SEBELUM permission check
- Mirror ke `internal/tools/sandbox.go` (section 12) — extend interceptor chain
- Per-tool karma penalty ke caller kalau hit block (Agent section 5 karma)

**Referensi file:** [`section_24_file_protector/`](referensifile/section_24_file_protector/) — **5 file ready to copy-adapt** (~975 LOC total):
- `host_protection.go` · `patterns.go` · `safety_audit.go` · `host_protection_test.go` · `protector_gui.go`

**Adaptasi (minimal):**
1. Replace import `github.com/flowork/kernel/...` → `flowork-gui/internal/protector`
2. Wire `Check()` di tool dispatcher (section 10) sebelum permission gate
3. Wire GUI API ke popup section "Protector" (rule list + toggle + test)

**Acceptance criteria:**
- Pre-check `rm -rf /` → blocked + audit row.
- Pre-check write ke `/etc/passwd` → blocked.
- Pre-check fetch ke `169.254.169.254` (cloud metadata) → blocked.
- Custom rule add via API → next call match pattern → blocked.
- Hardcoded rule delete attempt → 403 forbidden.
- Test suite 50+ scenario (host_protection_test.go) → all pass.

---

## Section 25 — Code Scanner (35+ static auditor)

**Goal:** scan code source — warga bikin tool sendiri di `/shared/<id>/tools/script.py`, upload `.fwagent.zip`, self-update diff, mistake_local promoted ke router — semua wajib lewat scanner dulu.

**Total 113 auditors di flowork lama.** Tier 1 saya pilih **35 P0/P1** yang critical. Sisanya (78) defer ke Tier 2/3.

**Komponen (copy-adapt dari referensi):**

- **`internal/scanner/auditor.go`** — orchestrator (`flowork_auditor.go`)
- **`internal/scanner/runner.go`** — runner (`flowork_runner.go`)
- **`internal/scanner/dashboard.go`** — UI dashboard data (`flowork_dashboard.go`)

**Tier 1 auditors (35) — kategorikan:**

### 🔴 Injection & Attack (8)
- `command_injection_auditor` · `prompt_injection_auditor` · `sql_injection_auditor` · `ssrf_auditor` · `xss_csrf_auditor` · `path_traversal_auditor` · `path_safety_auditor` · `taint_auditor`

### 🔴 Secrets & Sensitive Data (5)
- `hardcoded_secret_auditor` · `env_leak_auditor` · `sensitive_log_auditor` · `log_injection_auditor` · `token_leak_auditor`

### 🔴 Crypto & TLS (4)
- `crypto_auditor` · `crypto_weakness_auditor` · `deprecated_hash_auditor` · `tls_auditor` · `tls_config_auditor`

### 🔴 Supply Chain & Dependencies (4)
- `supply_chain_auditor` · `dangerous_import_auditor` · `dep_version_auditor` · `dockerfile_security_auditor`

### 🟡 Sandboxing & Permission (3)
- `sandbox_auditor` · `permission_auditor` · `idor_auditor`

### 🟡 Race & Concurrency (5)
- `toctou_auditor` · `goroutine_leak_auditor` · `panic_goroutine_auditor` · `panic_auditor` · `resource_leak_auditor`

### 🟡 Memory & Resource (3)
- `memory_auditor` · `zombie_auditor` · `atomic_write_auditor`

### 🟡 Anti-Pattern (3)
- `hallucination_trap_auditor` · `pandora_auditor` · `fortress_auditor`

### 🟢 Compliance (4)
- `exposure_auditor` · `zeroday_auditor` · `crossos_auditor` · `gosec_parser`

### 🟢 Budget & API (3)
- `budget_auditor` · `api_cost_auditor` · `api_rate_limit_auditor`

**Tabel baru:**

```sql
CREATE TABLE scanner_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_type     TEXT NOT NULL,             -- 'tool_generated' | 'fwagent_zip' | 'self_update' | 'manual'
  target_path   TEXT NOT NULL,
  started_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at   TEXT,
  total_findings INTEGER NOT NULL DEFAULT 0,
  critical_count INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL DEFAULT 'pending'  -- 'pending' | 'running' | 'pass' | 'fail'
);

CREATE TABLE scanner_findings (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id       INTEGER NOT NULL REFERENCES scanner_runs(id),
  auditor      TEXT NOT NULL,             -- 'command_injection_auditor' | dst
  severity     TEXT NOT NULL,             -- 'critical' | 'high' | 'medium' | 'low' | 'info'
  file_path    TEXT NOT NULL,
  line_number  INTEGER NOT NULL DEFAULT 0,
  message      TEXT NOT NULL,
  snippet      TEXT NOT NULL DEFAULT '',
  remediation  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_findings_severity ON scanner_findings(severity);
CREATE INDEX idx_findings_run ON scanner_findings(run_id);
```

**Endpoint baru:**
- `POST /api/agents/<id>/scanner/scan` — body `{target_path, scan_type}` → run scanners → return run_id
- `GET /api/agents/<id>/scanner/runs?limit=` — list past runs
- `GET /api/agents/<id>/scanner/findings?run_id=` — findings detail
- `GET /api/agents/<id>/scanner/auditors` — list available auditors

**Integrasi:**
- **Tool generation** (warga bikin Python di /shared/<id>/tools/) → auto-scan sebelum register di `tool_subscriptions`
- **Upload .fwagent.zip** → scan unpacked source → reject kalau critical finding
- **Self-update** (router push update) → scan diff → reject kalau regression security
- **Mistakes promotion** → scan content sebelum push ke router

**Referensi file:** [`section_25_code_scanner/`](referensifile/section_25_code_scanner/) — **46 file ready to copy-adapt**: 3 orchestrator (auditor, runner, dashboard) + 35 critical auditor.

**Shared approval library:** [`_common/approvals/approvals.go`](referensifile/_common/approvals/approvals.go) — unifikasi pattern owner approval untuk section 25 (scanner findings) + section 29 (zombie cleanup) + section 24 (protector custom rule). Pakai 1 library, jangan duplikasi 3x.

**Adaptasi (minimal):**
1. Replace import path
2. Wire orchestrator ke event triggers (tool gen, upload, self-update, promote)
3. Hook ke popup section "Scanner" (run + findings dashboard)
4. Pakai shared `internal/approvals/` library — 1 pattern untuk owner approve workflow

**Acceptance criteria:**
- Scan tool yang punya `exec.Command("sh", "-c", userInput)` → `command_injection_auditor` flag.
- Scan code dengan hardcoded `OPENAI_API_KEY="sk-..."` → `hardcoded_secret_auditor` flag.
- Scan dengan SQL string concat → `sql_injection_auditor` flag.
- Scan output: total findings + critical count + per-auditor breakdown.
- Performance: scan `agents/mr-flow/` < 30 detik.

---

## Section 26 — Audit log + Watchdog daemon

**Goal:** append-only audit log + watchdog daemon yang tail log → alert kalau anomaly (mis. 10 blocked dalam 1 menit = active attack).

**Komponen (copy-adapt dari referensi):**

- **`internal/audit/audit_kernel.go`** — kernel audit log primitives (append-only, structured fields).
- **`cmd/flowork-audit-watchdog/main.go`** — standalone watchdog daemon (tail audit log → alert via telegram_send tool).
- **`cmd/flowork-auditor/main.go`** — auditor CLI (run scanner batch, generate report).
- **`cmd/flowork-scan/main.go`** — scan CLI (single-shot, output JSON).
- **`cmd/flowork-bugscan/main.go`** — bugscan CLI (focused on bug patterns).

**Tabel baru:**

```sql
CREATE TABLE audit_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type    TEXT NOT NULL,             -- 'tool_call' | 'protector_block' | 'scanner_finding' | 'config_change'
  severity      TEXT NOT NULL,             -- 'info' | 'warning' | 'error' | 'critical'
  actor         TEXT NOT NULL DEFAULT '',
  detail_json   TEXT NOT NULL DEFAULT '{}',
  occurred_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_audit_event ON audit_log(event_type);
CREATE INDEX idx_audit_time ON audit_log(occurred_at DESC);

CREATE TABLE watchdog_alerts (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_id      TEXT NOT NULL,
  fired_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  context_json TEXT NOT NULL DEFAULT '{}',
  notified     INTEGER NOT NULL DEFAULT 0
);
```

**Watchdog rules (sample):**
- `>= 10 protector_block dalam 60s` → CRITICAL alert (active attack)
- `>= 5 scanner critical finding` di 1 scan → HIGH alert
- `>= 3 budget_exceeded` dalam 1 hari → MEDIUM alert (warga "ngamuk")
- Self-modification attempt detect → CRITICAL

**Endpoint baru:**
- `GET /api/agents/<id>/audit/log?from=&to=&type=` — query audit log
- `GET /api/agents/<id>/watchdog/alerts?limit=` — recent alerts
- `POST /api/agents/<id>/watchdog/rules` — add custom rule

**Referensi file:** [`section_26_audit_watchdog/`](referensifile/section_26_audit_watchdog/) — **5 file ready to copy-adapt**:
- `audit_kernel.go` · `audit_watchdog_main.go` · `auditor_main.go` · `scan_main.go` · `bugscan_main.go`

**Adaptasi (minimal):**
1. Replace import path
2. Wire audit_log writer ke setiap section 24 protector hit + section 25 scanner finding + section 10 tool call
3. Watchdog daemon embedded di Agent main process (atau standalone binary mode)
4. Alert via `telegram_send` tool (section 11)

**Acceptance criteria:**
- Setiap protector block / scanner critical / tool call → row di `audit_log`.
- Trigger 10x `rm -rf` blocked → watchdog fire CRITICAL alert → owner terima Telegram.
- Audit log immutable: UPDATE / DELETE rejected (trigger).
- Watchdog cooldown 1 jam per rule.

---

## Roadmap urutan + dependensi (Bagian 7 — Security)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 24 — File Protector (HPG) | 🔴 **P0** | Section 10 (tool foundation). **Wajib SEBELUM tool execute publik.** |
| 2 | Section 26 — Audit log | 🔴 P0 | Independen. Foundation buat watchdog + analytics. |
| 3 | Section 25 — Code Scanner | 🔴 P0 | Section 26 (audit log) done. Pair sama section 11 tool catalog (gate untuk tool generation). |
| 4 | Section 26 — Watchdog daemon | 🟡 P1 | Section 24 + 25 + 26 audit log done. Plus Telegram tool. |

**Catatan kerja:**
- Section 24 HPG = **MUST-HAVE** sebelum lo aktifin tool execute warga di public-facing context.
- 35 auditor Tier 1 cukup buat coverage 80%. Sisanya 78 auditor bisa di-add bertahap sesuai kebutuhan.
- Watchdog bisa **embedded mode** (goroutine di Agent main) atau **daemon mode** (standalone). Default embedded.
- Audit log = **APPEND-ONLY** wajib (FQP-12 doctrine). Sumber truth untuk forensik kalau ada attack.

---

## Folder referensi (UPDATED — section 24-26)

```
referensifile/
├── ... (section 01-23)
├── section_24_file_protector/          (5 files — HPG + patterns + audit + test + GUI rule mgmt)
├── section_25_code_scanner/            (46 files — orchestrator + 35 P0/P1 auditors + runner + dashboard)
└── section_26_audit_watchdog/          (5 files — audit_kernel + 4 cmd binary main.go)
```

**Total file referensi Agent: 176 files (~1.5M).**

**Sumber:**
- `Music/flowork/*` → section 01-18 (brain/tools/slash/scheduler)
- `Pictures/stable_open_router/.../mesh/` → section 19-20 (mesh awareness)
- `Pictures/stable_open_router/.../{wallet,walletalert,finance}/` → section 21-23 (wallet)
- `Pictures/stable_open_router/.../{safety,scanner,audit-watchdog}/` → section 24-26 (security)

---

*Update: 2026-05-29 — Bagian 7 ditambahkan.*

---

# === BAGIAN 8 — CODEMAP & STRUCTURE INTELLIGENCE ===

> **Vision Mr.Dev**: tools untuk warga memahami struktur Flowork sendiri + deteksi file zombie. Plus tampilan GUI yang sangat cocok (Mr.Dev confirm). Critical kalau warga mau **self-modify, self-heal, atau self-grow** — mereka harus tau dulu struktur diri sendiri.
>
> **Sumber:** `Pictures/stable_open_router/flowork_project/floworkos-go/{internal/codeindex,internal/tools/codemap_*,internal/factmemory/ast_*,internal/guiapi/codemap_*,internal/workspacefs/zombie_detect.go}` + GUI `static/tabs/codemap.js` + scanner `flowork_zombie_auditor.go`. **Sudah ke-copy ke `referensifile/section_27-30/`.**
>
> **Strategi (HINDARI HALU + BUG):** ~8,000 LOC subsystem production-grade. Semua logic udah di referensifile/. Implementasi = copy-adapt minimal (import path). **Jangan code dari scratch** — kompleksitas tinggi (Go parser + JS parser + graph query + visualization).

---

## Section 27 — Codemap engine (codeindex + AST indexer)

> **⚠️ OVER-PROMPT RISK** — graph hasil query JANGAN dump ke prompt. Cuma return top-N nodes + neighborhood (depth ≤ 2). Tool `codemap_*` wajib return SUMMARY (mis. "fungsi X dipanggil 5x dari Y, Z, ..."), bukan full nodes JSON. Limit max 10 nodes per response.

**Goal:** index seluruh codebase Flowork (Go + JS) jadi graph nodes + edges + dependencies. Engine ini foundation buat semua tools & GUI berikutnya.

**Komponen (copy-adapt dari referensi):**

- **`internal/codemap/indexer.go`** — orchestrator: walk filesystem → parse → build graph
- **`internal/codemap/goparser.go`** — Go AST parser (funcs, types, calls, imports)
- **`internal/codemap/jsparser.go`** — JavaScript parser
- **`internal/codemap/funcnodes.go`** — function-level node structure
- **`internal/codemap/graphquery.go`** — query API: callers, callees, deps, impact path
- **`internal/codemap/layerclassify.go`** — auto-classify file → layer (cmd / kernel / tools / brain / gui)
- **`internal/codemap/flowtracer.go`** — trace execution flow (entry → leaf)
- **`internal/codemap/diffhighlight.go`** — diff visualization (after git change, highlight impact)
- **`internal/codemap/githook.go`** — git hook integration (re-index on commit)
- **`internal/codemap/docgen.go`** — auto-generate docs dari AST
- **`internal/codemap/registry.go`** — registry singleton
- **`internal/codemap/review.go`** — review helper
- **`internal/codemap/tourbuilder.go`** — guided tour builder
- **`internal/codemap/ast_indexer.go`** + **`ast_query.go`** — AST-level indexing
- **`internal/codemap/codemap_columns.go`** — column schema (DB column mapping)

**Tabel baru:**

```sql
CREATE TABLE codemap_nodes (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  node_type     TEXT NOT NULL,             -- 'file' | 'func' | 'type' | 'method' | 'var'
  name          TEXT NOT NULL,
  file_path     TEXT NOT NULL,
  line_start    INTEGER NOT NULL DEFAULT 0,
  line_end      INTEGER NOT NULL DEFAULT 0,
  layer         TEXT NOT NULL DEFAULT '',  -- 'kernel' | 'tool' | 'brain' | 'gui' | 'agent'
  signature     TEXT NOT NULL DEFAULT '',  -- func signature for searchability
  docstring     TEXT NOT NULL DEFAULT '',
  size_loc      INTEGER NOT NULL DEFAULT 0,
  complexity    INTEGER NOT NULL DEFAULT 0,
  last_modified TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  indexed_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_codemap_nodes_file ON codemap_nodes(file_path);
CREATE INDEX idx_codemap_nodes_type ON codemap_nodes(node_type);
CREATE INDEX idx_codemap_nodes_layer ON codemap_nodes(layer);

CREATE TABLE codemap_edges (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  from_node_id  INTEGER NOT NULL REFERENCES codemap_nodes(id),
  to_node_id    INTEGER NOT NULL REFERENCES codemap_nodes(id),
  edge_type     TEXT NOT NULL,             -- 'calls' | 'imports' | 'inherits' | 'references'
  weight        INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_codemap_edges_from ON codemap_edges(from_node_id);
CREATE INDEX idx_codemap_edges_to ON codemap_edges(to_node_id);

CREATE TABLE codemap_index_runs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at  TEXT,
  total_files  INTEGER NOT NULL DEFAULT 0,
  total_nodes  INTEGER NOT NULL DEFAULT 0,
  total_edges  INTEGER NOT NULL DEFAULT 0,
  status       TEXT NOT NULL DEFAULT 'pending'
);
```

**Referensi file:** [`section_27_codemap_engine/`](referensifile/section_27_codemap_engine/) — **18 file ready** (~3,366 LOC):
- indexer, goparser, jsparser, funcnodes, graphquery, layerclassify, flowtracer, diffhighlight, githook, docgen, registry, review, tourbuilder, ast_indexer, ast_query, ast_query_test, codemap_columns, codemap_columns_test

**Adaptasi (minimal):**
1. Replace import `github.com/teetah2402/flowork/...` → `flowork-gui/internal/codemap`
2. Wire ke `agentdb.ensureSchema()`
3. Index target = whole Flowork_Agent + agents/<id>/ folder

**Acceptance criteria:**
- Run indexer → `codemap_nodes` populated (>100 rows untuk Flowork_Agent).
- Query callers of `agentmgr.ConfigHandler` → return list valid.
- Layer classification: `internal/kernelhost/*.go` → `kernel`, `web/tabs/*.js` → `gui`.
- Performance: full re-index < 60 detik untuk repo ukuran sekarang.

---

## Section 28 — Codemap tools (warga query struktur)

**Goal:** wrap codemap engine sebagai **tools yang warga panggil** (lihat section 11 tool catalog). Warga bisa nanya: "siapa yang panggil fungsi X?", "kalau gw ubah Y, file apa kena impact?", "graph dependency dari Z?".

**Komponen (copy-adapt dari referensi):**

- **`internal/tools/codemap/codemap_tool.go`** — `codemap` tool (search node by name)
- **`internal/tools/codemap/codemap_tool_deps.go`** — `codemap_deps` (dependency graph)
- **`internal/tools/codemap/codemap_tool_impact.go`** — `codemap_impact` (impact analysis)
- **`internal/tools/codemap/codemap_tool_search.go`** — `codemap_search` (text + semantic search)
- **`internal/tools/codemap/codemap_tool_health.go`** — `codemap_health` (complexity, hotspot, anti-pattern detect)
- **`internal/tools/codemap/code_graph_tools.go`** — `code_graph_query` (advanced graph query)
- **`internal/tools/codemap/ast_search.go`** — `search_semantic_function` (AST semantic search)
- **`internal/tools/codemap/ast_callers.go`** — `find_callers` (kakak section 11 tier1)
- **`cmd/flowork-codemap-cli/main.go`** — standalone CLI (di-pakai owner buat scan manual)

**Integrasi:**
- Re-register di tool registry (section 10): `codemap`, `codemap_deps`, `codemap_impact`, `codemap_search`, `codemap_health`, `code_graph_query`, `search_semantic_function`, `find_callers`
- Hooked ke karma (section 5): warga sering pakai `codemap_health` → karma + (proactive code quality)

**Referensi file:** [`section_28_codemap_tools/`](referensifile/section_28_codemap_tools/) — **9 file ready** (~1,400 LOC).

**Adaptasi (minimal):**
1. Replace import path
2. Tools dispatcher di section 10 tool foundation registry
3. Permission: `fs:read:/workspace/**` + `rpc:internal:codemap`

**Acceptance criteria:**
- `find_callers name=ConfigHandler` → return list 5+ caller dengan file:line.
- `codemap_impact file=internal/kernelhost/kernelhost.go` → return downstream impact list.
- `codemap_health` → return complexity hotspot top-10.
- CLI: `./flowork-codemap-cli search "Handler"` → return matches dengan layer info.

---

## Section 29 — Zombie detector (file zombie analysis)

**Goal:** **deteksi file zombie** — file orphan, dead code, boilerplate workspace, stale folder. Sangat penting kalau warga generate tool sendiri (`/shared/<id>/tools/`) → bisa pollute kalau ngga di-clean.

**Heuristic (per source code workspacefs/zombie_detect.go):**
- Newest mtime di subtree > 30 hari (stale)
- README.md size < 100 bytes (boilerplate)
- Daily roadmap kosong / hanya skeleton
- Function/var ngga di-call dari mana-mana (dead code dari codemap graph)
- File extension orphan (mis. .tmp, .bak, .orig)

**Komponen (copy-adapt dari referensi):**

- **`internal/zombies/zombie_detect.go`** — workspace zombie detector (folder-level heuristic)
- **`internal/zombies/code_zombies.go`** — code-level zombie (dead function/var, leverage codemap section 27)
- **`internal/zombies/scanner_zombie.go`** — wrapper around scanner `flowork_zombie_auditor.go`
- **`internal/zombies/cli.go`** — CLI runner (dari `audit_zombie_cmd.go`)
- Plus tool registration: `codemap_zombies` (dari section 11 tier2)

**Tabel baru:**

```sql
CREATE TABLE zombie_findings (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_run_id   INTEGER,
  zombie_type   TEXT NOT NULL,             -- 'stale_folder' | 'boilerplate' | 'dead_code' | 'orphan_file'
  target_path   TEXT NOT NULL,
  severity      TEXT NOT NULL DEFAULT 'low',  -- 'high' (auto-delete OK) | 'medium' (review) | 'low' (FYI)
  detected_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  heuristic     TEXT NOT NULL DEFAULT '',
  recommendation TEXT NOT NULL DEFAULT '',  -- 'delete' | 'archive' | 'investigate'
  status        TEXT NOT NULL DEFAULT 'pending'  -- 'pending' | 'approved' | 'rejected' | 'cleaned'
);
CREATE INDEX idx_zombie_type ON zombie_findings(zombie_type);
CREATE INDEX idx_zombie_status ON zombie_findings(status);
```

**Endpoint baru:**
- `POST /api/agents/<id>/zombies/scan?target=` — run zombie scan
- `GET /api/agents/<id>/zombies/findings?status=pending` — list pending review
- `POST /api/agents/<id>/zombies/approve?id=N` — owner approve cleanup
- `POST /api/agents/<id>/zombies/clean?id=N` — execute cleanup (after approve)

**REPORT-ONLY by default** (Mr.Dev keputusan hapus manual) — auto-delete cuma untuk severity=high + owner pre-approve global flag.

**Referensi file:** [`section_29_zombie_detector/`](referensifile/section_29_zombie_detector/) — **4 file ready**:
- `zombie_detect.go` (workspacefs heuristic) · `codemap_tool_zombies.go` (tool) · `flowork_zombie_auditor.go` (scanner) · `audit_zombie_cmd.go` (CLI)

**Adaptasi (minimal):**
1. Replace import path
2. Hook ke codemap engine (section 27) untuk dead code detection
3. Hook ke scanner (section 25) untuk integration

**Acceptance criteria:**
- Scan `agents/mr-flow/workspace/` → kalau cache 30+ hari → flag stale.
- Scan codebase → function ngga di-call → flag dead_code.
- Owner approve → cleanup executed → row status = 'cleaned'.
- Audit log: setiap cleanup logged.

---

## Section 30 — Codemap GUI (visualization tab Mr.Dev sangat cocok)

**Goal:** tampilan GUI yang Mr.Dev udah confirm "sangat cocok". Tab Codemap di popup atau standalone page. Backend handler + frontend JS + HTML shell — full stack copy-adapt.

**Komponen (copy-adapt dari referensi):**

### Backend (Go handler)
- **`internal/codemap_gui/codemap.go`** — main handler (list nodes, graph data)
- **`internal/codemap_gui/codemap_graph.go`** — graph rendering API (force-directed layout data)
- **`internal/codemap_gui/codemap_indexer.go`** — trigger reindex endpoint
- **`internal/codemap_gui/codemap_reindex.go`** — full re-index handler
- **`internal/codemap_gui/codemap_tour.go`** — guided tour data
- **`internal/codemap_gui/codemap_health.go`** — health dashboard data

### Frontend
- **`web/tabs/codemap.js`** — main tab (UI yang Mr.Dev confirm cocok)
- **`web/codemap.html`** — full-page version (optional)

**Endpoint baru:**
- `GET /api/agents/<id>/codemap/nodes?layer=&type=&limit=` — paginated node list
- `GET /api/agents/<id>/codemap/graph?center=<node_id>&depth=N` — neighborhood graph
- `POST /api/agents/<id>/codemap/reindex` — trigger reindex
- `GET /api/agents/<id>/codemap/health` — dashboard data (complexity hotspot, layer distribution, dead code count)
- `GET /api/agents/<id>/codemap/tour?topic=` — guided tour content

**Integrasi UI:**
- Tab baru di popup setting (atau new top-level menu)
- D3.js / vis.js untuk graph viz (depends di referensi `codemap.js`)
- Filter by layer (kernel/tool/brain/gui/agent)
- Click node → side panel detail (file path, callers, complexity, last modified)

**Referensi file:** [`section_30_codemap_gui/`](referensifile/section_30_codemap_gui/) — **9 file ready** (~1,237 LOC backend + frontend):
- 6 Go handler (codemap, codemap_graph, codemap_indexer, codemap_reindex, codemap_tour, codemap_health)
- `codemap.js` — **GUI yang Mr.Dev sangat cocok**
- `gui_shell_index.html` — main shell pattern
- `codemap_feature_doc.md` — feature docs

**Adaptasi (minimal):**
1. Backend: replace import path, wire ke `internal/codemap` (section 27)
2. Frontend: integrate ke existing `web/` structure (web/tabs/codemap.js → akses lewat side menu atau sidebar tab baru "🗺️ Codemap")
3. Asset dependencies (d3.js / vis.js): pakai existing `web/vendor/d3.min.js` yang udah ada (kalau ngga ada, download minified)

**Acceptance criteria:**
- Tab Codemap muncul di sidebar / popup.
- Graph viz render: kernel layer (warna A), tool layer (warna B), gui layer (warna C).
- Click node → side panel: nama, file:line, callers list, complexity score.
- Reindex button → trigger backend → progress shown.
- Health dashboard: top-10 complexity hotspot, layer distribution pie chart.
- **Visual match dengan referensi** (Mr.Dev confirmed cocok).

---

## Roadmap urutan + dependensi (Bagian 8 — Codemap)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 27 — Codemap engine | 🔴 P0 | Foundation. Independen. |
| 2 | Section 28 — Codemap tools | 🔴 P0 | Section 27 done + section 10 tool foundation |
| 3 | Section 30 — Codemap GUI | 🟡 P1 | Section 27 done. UI Mr.Dev confirm cocok. |
| 4 | Section 29 — Zombie detector | 🟡 P1 | Section 27 done + section 25 scanner (integration) |

**Catatan kerja:**
- Codemap engine = warga's mirror — kalau Flowork mau **self-modify atau self-heal**, mereka harus query engine ini dulu.
- Zombie detector = REPORT-ONLY default. Owner approval workflow penting (jangan auto-delete tanpa review).
- GUI section 30 = Mr.Dev udah cocok dengan tampilan → strict visual match, jangan re-design.
- Re-index trigger: git commit hook (githook.go), atau periodic (cron), atau manual via API.

---

## Folder referensi (UPDATED — section 27-30)

```
referensifile/
├── ... (section 01-26)
├── section_27_codemap_engine/          (18 files — codeindex + ast + columns)
├── section_28_codemap_tools/           (9 files — 5 codemap tools + 2 ast + code_graph + CLI)
├── section_29_zombie_detector/         (4 files — workspace zombie + tool + auditor + script)
└── section_30_codemap_gui/             (9 files — 6 handler + codemap.js + html shell + feature doc)
```

**Total file referensi Agent: 216 files (~1.9M).**

---

*Update: 2026-05-29 — Bagian 8 ditambahkan.*

---

# === BAGIAN 9 — PIPELINE & WORKFLOW PATTERNS (adopt dari Architect Agent Blueprint) ===

> **Konteks**: Mr.Dev share blueprint "The Architect Agent — Blueprint to Build" yang punya pattern bagus untuk:
> - Linear pipeline dengan Brief Writer → Section Agent → Injector → Tracker
> - Mode selection (Full / Lite / Custom)
> - Failure Recovery Protocol formal
> - Mandatory Pause + Approval Gate
> - Self-contained `prompt.md` (cold executor, no prior context)
> - 6-category Legal scan grouping
> - ECC skills check bootstrap
>
> **Architect Agent ≠ Flowork** — beda scope. Architect = planning tool, Flowork = runtime platform. Tapi 7 pattern ini SANGAT BAGUS — saya adopt sebagai design constraint + tools di sini.
>
> **Strategi**: bukan port code langsung, ini lebih ke **design pattern + supporting tools**. File referensi sebagian dari Music/flowork (compress_history, compact, brief_writer pattern).

---

## Section 31 — Pipeline pattern (Brief Writer → Section Agent → Injector → Tracker)

**Goal:** untuk task complex multi-step warga (mis. Team Coder doing refactor 10 file), pakai pipeline 4-stage biar context ngga balloon + ada consistency check.

**Pipeline stages:**

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────┐
│ Brief Writer│ →  │ Section Agent│ →  │   Injector   │ →  │ Tracker  │
│ (curator)   │    │ (executor)   │    │ (consistency)│    │ (state)  │
└─────────────┘    └──────────────┘    └──────────────┘    └──────────┘
     ↓                  ↓                    ↓                  ↓
  curated           output                contradiction       state.md
  context           artifact              check vs prev       record
```

**Komponen (copy-adapt dari referensi):**

- **`internal/pipeline/brief_writer.go`** — compress prior context jadi self-contained brief untuk next stage
- **`internal/pipeline/section_agent.go`** — generic section executor (wrap LLM call dengan brief)
- **`internal/pipeline/injector.go`** — contradiction check (cek output vs previous sections; flag inconsistency)
- **`internal/pipeline/tracker.go`** — background dispatch, write `<task_id>/state.md` per section

**Tabel baru:**

```sql
CREATE TABLE pipeline_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  task_name     TEXT NOT NULL,
  current_stage TEXT NOT NULL,             -- 'brief' | 'agent' | 'injector' | 'tracker'
  status        TEXT NOT NULL DEFAULT 'running',  -- 'running' | 'paused' | 'failed' | 'done'
  started_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at   TEXT
);

CREATE TABLE pipeline_artifacts (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id       INTEGER NOT NULL REFERENCES pipeline_runs(id),
  stage        TEXT NOT NULL,
  artifact_path TEXT NOT NULL,             -- file output yang dihasilkan
  content_hash  TEXT NOT NULL,
  created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Referensi file:** [`section_31_pipeline_pattern/`](referensifile/section_31_pipeline_pattern/) — **2 file ready**:
- `brief_writer.go` — pattern brief curation dari kernel
- `compress_history.go` — pattern compress conversation history (relevant buat brief writer)

**Acceptance criteria:**
- Task multi-step → split jadi stages → state.md per stage.
- Brief writer compress context: 5K tok → 500 tok summary self-contained.
- Injector detect contradiction: output stage 2 conflict dengan stage 1 → flag + halt.
- Tracker append-only.

---

## Section 32 — Mode selection (Full / Lite / Custom)

**Goal:** operasi besar (scanner full audit, brain ingestion, codemap reindex) kasih opsi user pilih mode — biar token + waktu ngga bocor sia-sia.

**Mode preset:**

| Mode | Coverage | Token est | Waktu est |
|---|---|---|---|
| **Full** | All 35 auditor / 13 brain stages / full codebase reindex | ~28k | 35-55 min |
| **Lite** | 10 P0 auditor / 6 core stages / changed files only | ~14k | 18-30 min |
| **Custom** | User pick which to include | varies | varies |

**Komponen:**

- **`internal/runmode/runmode.go`** — Mode enum + preset definition
- **`internal/runmode/selector.go`** — CLI/API selector
- Integrate ke: section 25 scanner, section 27 codemap reindex, Router section 1 ingestion

**API extension:**
```
POST /api/agents/<id>/scanner/scan?mode=full|lite|custom&exclude=<auditor_ids>
POST /api/agents/<id>/codemap/reindex?mode=full|incremental
```

**Referensi file:** [`section_32_mode_selection/`](referensifile/section_32_mode_selection/) — **1 file** (`codemap_tool_health.go` punya mode pattern). Sisanya design from scratch (simple enum + dispatch).

**Acceptance criteria:**
- Scanner `mode=lite` → ngga jalan auditor di luar P0 list.
- Codemap `mode=incremental` → cuma re-index file yang changed sejak last index (cek mtime > last_run.finished_at).
- Custom mode: user kasih array exclude → respected.

---

## Section 33 — Failure Recovery Protocol (formal retry + escalate)

**Goal:** task fail → JANGAN langsung error. Pattern:
```
Failure → Retry (auto, max 3x dengan backoff) → Owner choice (retry/skip/manual) → Stub doc + issue log → Continue
```

**Komponen:**

- **`internal/recovery/retry.go`** — exponential backoff (1s, 2s, 4s, 8s) — sudah ada pattern di mesh `peer_connect.go`
- **`internal/recovery/escalate.go`** — kalau retry exhausted, fire owner notification + park task ke `failed_tasks` table
- **`internal/recovery/stub_doc.go`** — generate stub markdown "task X failed at step Y, here's what we have" supaya continue pipeline ngga total stuck

**Tabel baru:**

```sql
CREATE TABLE failed_tasks (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id       TEXT NOT NULL,
  failed_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  failed_stage  TEXT NOT NULL,
  retry_count   INTEGER NOT NULL DEFAULT 0,
  error_text    TEXT NOT NULL,
  stub_doc_path TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'parked',  -- 'parked' | 'manual_fix' | 'skipped' | 'retried'
  owner_choice  TEXT,                            -- 'retry' | 'skip' | 'manual'
  resolved_at   TEXT
);
```

**Owner notification channel:**
- Default: Telegram via existing `telegram_send` tool
- Plus log ke `decisions` (Agent section 3)
- Plus dashboard di popup section "Failed Tasks"

**Referensi file:** [`section_33_failure_recovery/peer_connect.go`](referensifile/section_33_failure_recovery/peer_connect.go) — pattern exponential backoff dari mesh.

**Acceptance criteria:**
- Tool exec fail → auto retry 3x dengan backoff → escalate kalau masih gagal.
- Telegram alert ke owner saat escalate.
- Owner reply via Telegram "/skip" → row status → 'skipped' → next task continue.
- Stub doc generated saat skip — bukan kosong, ada info "X gagal di step Y, last known state: Z".

---

## Section 34 — Mandatory Pause + Approval Gate (unify scattered approval)

**Goal:** unify 3 approval workflow yang sekarang scattered (scanner section 25, zombie section 29, protector section 24) jadi 1 shared library + formal "MANDATORY PAUSE" semantic.

**Komponen (sudah ada di `_common/approvals/`):**

- **`_common/approvals/approvals.go`** — shared library. Reuse di:
  - Scanner findings → owner approve critical fix
  - Zombie detector → owner approve cleanup
  - Protector custom rule → owner approve activate
  - Pipeline mandatory pause → owner approve continue

**Pattern wajib:**
```go
// Anti-pattern (scattered):
if needsApproval { ... custom each section ... }

// Pattern unified (section 34):
approval := approvals.Request(ctx, "scanner.fix.critical_finding", payload)
<-approval.WaitDecision()  // blocks until owner: approve / reject / defer
if approval.Approved() { execute() }
```

**Tabel baru:**

```sql
CREATE TABLE approval_requests (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  request_type  TEXT NOT NULL,             -- 'scanner.fix' | 'zombie.cleanup' | 'protector.rule' | 'pipeline.continue'
  payload_json  TEXT NOT NULL,
  requested_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at    TEXT,
  decision      TEXT,                       -- 'approve' | 'reject' | 'defer'
  decided_by    TEXT NOT NULL DEFAULT 'owner',
  context       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_approvals_pending ON approval_requests(decided_at) WHERE decided_at IS NULL;
```

**API:**
- `GET /api/agents/<id>/approvals/pending` — list pending
- `POST /api/agents/<id>/approvals/<req_id>/decide?decision=approve|reject|defer`

**Referensi file:** [`_common/approvals/approvals.go`](referensifile/_common/approvals/approvals.go) — sudah ada.

**Acceptance criteria:**
- Scanner critical → wait owner approve via popup atau Telegram → execute on approve.
- Multi-pending: max 5 concurrent approval request → batch owner notification.
- Cooldown 1 hour per request_type (jangan spam owner).

---

## Section 35 — Self-contained prompt.md (inter-warga communication, ANTI OVER-PROMPT) ⭐⭐

> **⭐⭐ HIGHEST VALUE PATTERN** — match prinsip prompt budget di standar section 11. Pattern paling penting di Bagian 9.

**Goal:** kalau warga A delegate task ke warga B, JANGAN share whole interaction history. **WAJIB self-contained prompt** dengan:
- Full context yang dibutuhkan task
- Constraints
- Acceptance criteria
- Deliverables

Pola "cold executor session, no prior context needed" dari Architect Agent.

**Komponen (copy-adapt dari referensi):**

- **`internal/compact/compact.go`** — pattern compress conversation/context
- **`internal/compact/memory_compact.go`** — compress memory tier (episodic + facts) jadi summary
- **`internal/tools/compact_context.go`** — tool yang warga bisa panggil sendiri
- **`internal/warga/inject_compact.go`** — pattern inject compact ke daemon
- Plus skill markdown `compact.md` (default skill auto-available)

**Use case:**

**❌ Anti-pattern (warga A → warga B):**
```json
{
  "context": [<seluruh chat history 50 message>],
  "task": "Build summary"
}
```

**✅ Pattern self-contained:**
```json
{
  "prompt": "TASK: Buat ringkasan eksekutif dari topik X.\n\nKONTEKS:\nUser ingin laporan untuk meeting hari Senin. Audience: investor.\nKey points dari diskusi: A, B, C.\n\nCONSTRAINTS:\n- Maks 200 kata\n- Bahasa formal\n- No jargon\n\nACCEPTANCE:\n- Ada exec summary di awal\n- 3 bullet point key insight\n- Call-to-action di akhir\n\nDELIVERABLE: Markdown file ringkasan.md disimpan di /shared/<requester>/job/."
}
```

**Implementation contract** (per standar section 11 — Prompt Budget):

```go
// internal/comms/delegate.go
type DelegateRequest struct {
    From         string `json:"from"`           // warga A id
    To           string `json:"to"`             // warga B id
    SelfContainedPrompt string `json:"prompt"`  // ⭐ WAJIB self-contained, max 2000 tok
    DeliverableTo string `json:"deliverable_to,omitempty"` // optional path output
}

// Validation: prompt length cap, context history NOT included
func ValidateDelegate(req DelegateRequest) error {
    if len(req.SelfContainedPrompt) > maxDelegatePromptChars {
        return errors.New("delegate prompt > 2000 char — compact first via `compact_context` tool")
    }
    // Anti-pattern check: deny if prompt contains "see history" / "prior conversation"
    forbidden := []string{"see history", "prior conversation", "as we discussed"}
    for _, p := range forbidden {
        if strings.Contains(strings.ToLower(req.SelfContainedPrompt), p) {
            return errors.New("anti-pattern: delegate prompt must be self-contained, can't reference prior context")
        }
    }
    return nil
}
```

**Endpoint baru:**
- `POST /api/agents/<from_id>/delegate?to=<to_id>` — body `{prompt, deliverable_to}` — kirim ke target warga (lewat router atau direct)
- Owner monitor via dashboard "Inter-Warga Tasks"

**Referensi file:** [`section_35_self_contained_prompt/`](referensifile/section_35_self_contained_prompt/) — **5 file ready**:
- `compact.go` · `memory_compact.go` · `compact_context.go` · `inject_compact.go` · `compact.md` (skill)

**Standar doc update:**
- Section 11 (Prompt Budget) — tambah subsection "Inter-Warga Communication MUST use self-contained prompt"
- Anti-pattern list di section 6 README_FIRST — tambah "❌ Share interaction history saat delegate"

**Acceptance criteria:**
- Warga A delegate ke Warga B → request body validate prompt length + self-contained.
- Anti-pattern phrase detected → reject + suggest "compact dulu via compact_context tool".
- Warga B execute dengan COLD context (ngga ada history dari A).
- Owner dashboard tampil "delegate tasks" dengan from/to/status.

---

## Section 36 — 6-Category Legal Scan grouping (kelompokin 35 auditor)

**Goal:** Architect Agent kelompokin compliance ke 6 kategori clear. Kita adopt — kelompokin 35 auditor (section 25) jadi 6 kategori biar owner gampang filter + report.

**6 Kategori:**

| # | Kategori | Auditor count | Auditor list |
|---|---|---|---|
| 1 | **Injection & Attack** | 8 | command/prompt/sql/ssrf/xss/path_traversal/path_safety/taint |
| 2 | **Secrets & Sensitive Data** | 5 | hardcoded_secret/env_leak/sensitive_log/log_injection/token_leak |
| 3 | **Crypto & TLS** | 5 | crypto/crypto_weakness/deprecated_hash/tls/tls_config |
| 4 | **Supply Chain** | 4 | supply_chain/dangerous_import/dep_version/dockerfile_security |
| 5 | **Sandbox & Permission** | 3 | sandbox/permission/idor |
| 6 | **Compliance & Cross-OS** | 4 | exposure/zeroday/crossos/gosec_parser |

**Plus 5 quality categories** (non-legal, tetap tracked separately):
- Concurrency (5): toctou/goroutine_leak/panic_goroutine/panic/resource_leak
- Memory (3): memory/zombie/atomic_write
- Anti-Pattern (3): hallucination_trap/pandora/fortress
- Budget (3): budget/api_cost/api_rate_limit

**Komponen:**

- **`internal/scanner/categories.go`** — mapping auditor → category
- **Scanner runner extension** — filter by category: `flowork-scan --category=injection`
- **Report grouping** — `scanner_findings` aggregate by category di dashboard

**Tabel update (section 25 scanner_findings):**

```sql
ALTER TABLE scanner_findings ADD COLUMN category TEXT NOT NULL DEFAULT '';
-- Migration: backfill dari mapping
```

**API extension:**
- `GET /api/agents/<id>/scanner/findings?category=injection&run_id=N`
- `GET /api/agents/<id>/scanner/category-summary?run_id=N` — count per kategori

**Referensi file:** **0 file** — pure design pattern. Mapping di code, sudah ada auditor di section 25.

**Acceptance criteria:**
- Scanner output → tiap finding tag dengan category.
- Filter API: `category=secrets` → cuma return 5 auditor results.
- Dashboard pie chart per kategori (visual scan status).

---

## Section 37 — ECC Skills Bootstrap (auto-install recommended skills once)

**Goal:** warga baru join → otomatis pull "recommended skills" dari router catalog (section 8 Router) → cache di per-agent state.db. Pattern dari Architect Agent: "ECC skills check (installs recommended skills once)".

**Komponen:**

- **`internal/skills_bootstrap/checker.go`** — cek meta `bootstrapped_at` di state.db
- **`internal/skills_bootstrap/installer.go`** — pull recommended skills dari router
- **`internal/skills_bootstrap/recommended.go`** — definition skill mana yang "recommended" per agent kind

**Tabel baru di meta:**

```sql
-- Pakai meta table yang sudah ada (section 1 standar):
-- meta.bootstrapped_at = ISO timestamp first boot
-- meta.bootstrap_version = '1' (incremement kalau ada migration)
```

**Recommended skills (curated list per kind):**

| Agent kind | Recommended skills |
|---|---|
| `chat-bot` (mr-flow) | `compact`, `summarize`, `greet` |
| `code-helper` (Team Coder) | `code-review`, `refactor`, `test-write`, `git-flow` |
| `youtube-agent` | `video-summary`, `transcript-extract` |
| (generic) | `compact` (always installed) |

**Flow:**
1. Boot → checker.IsBootstrapped() → kalau `bootstrapped_at IS NULL`
2. Read manifest `kind` → match recommended skills list
3. `routerclient.PullSkills(recommended_ids)` — fetch dari Router section 8
4. Insert ke `skills` table per warga
5. Set `meta.bootstrapped_at = now()`

**Komponen referensi (copy-adapt):**

- **`internal/skills_bootstrap/from_hub.go`** — adapt pattern dari `skills_hub.go`
- **`internal/skills_bootstrap/autocreate.go`** — adapt pattern dari `skill_autocreate.go`

**Referensi file:** [`section_37_skills_bootstrap/`](referensifile/section_37_skills_bootstrap/) — **2 file ready**:
- `skills_hub.go` · `skill_autocreate.go`

**Acceptance criteria:**
- Mr.Flow boot pertama → bootstrap pulls `compact` + `summarize` + `greet` skill.
- `meta.bootstrapped_at` set → boot kedua skip bootstrap (idempotent).
- Router down → bootstrap retry next boot (graceful degradation).
- Owner bisa add `meta.bootstrap_skip='1'` kalau mau skip auto-bootstrap.

---

## Roadmap urutan + dependensi (Bagian 9 — Pipeline & Workflow Patterns)

| # | Section | Priority | Dependensi |
|---|---|---|---|
| 1 | Section 35 — **Self-contained prompt** ⭐⭐ | 🔴 P0 | Highest value, anti over-prompt. Foundation buat inter-warga comm. |
| 2 | Section 34 — Approval Gate unify | 🔴 P0 | Quick win — `_common/approvals/` udah ada. Refactor scattered. |
| 3 | Section 33 — Failure Recovery | 🟡 P1 | Pair sama Telegram tool. Wajib untuk production-grade. |
| 4 | Section 32 — Mode selection | 🟡 P1 | Quick win — simple enum + dispatch. |
| 5 | Section 37 — Skills Bootstrap | 🟡 P1 | Setelah Router section 8 (skill catalog) ready. |
| 6 | Section 36 — Legal scan grouping | 🟢 P2 | Polish — pure UI/UX improvement, scanner section 25 udah ada. |
| 7 | Section 31 — Pipeline pattern | 🟢 P2 | Useful buat complex task (Team Coder future), bukan blocking Mr.Flow. |

**Catatan kerja:**
- Section 35 (self-contained prompt) **WAJIB** sebelum kita expand warga (Team Coder, dst.) — otherwise inter-warga comm akan over-prompt.
- Section 34 (approval unify) — refactor existing, ngga ada user-facing change.
- Section 36 (legal scan grouping) — pair sama UI pie chart dashboard, polish.

---

## Folder referensi (UPDATED — section 31-37)

```
referensifile/
├── ... (section 01-30)
├── section_31_pipeline_pattern/        (2 files — brief_writer, compress_history)
├── section_32_mode_selection/          (1 file — codemap_tool_health pattern)
├── section_33_failure_recovery/        (1 file — peer_connect backoff pattern)
├── section_34_mandatory_pause/         (pointer ke _common/approvals/)
├── section_35_self_contained_prompt/   (5 files — compact + memory_compact + tool + inject + skill.md) ⭐⭐
├── section_36_legal_scan_grouping/     (0 file — pure design pattern)
└── section_37_skills_bootstrap/        (2 files — skills_hub, skill_autocreate)
```

**Total file referensi Agent: 229 files (~2.0M).**

**Sumber Bagian 9:**
- `Music/flowork/*` → brief_writer, compact patterns
- `Pictures/stable_open_router/.../approvals/` → approval pattern
- Architect Agent blueprint (visual reference) → design constraint adoption

---

## Cross-cutting impact ke Roadmap lain

| Section terdampak | Section 9.x | Action |
|---|---|---|
| Section 11 standar doc (Prompt Budget) | 35 (self-contained prompt) | Tambah subsection wajib inter-warga self-contained |
| Section 25 Scanner | 36 (legal grouping) | Tag finding dengan category |
| Section 24, 25, 29 (scattered approval) | 34 (approval unify) | Refactor pakai `_common/approvals/` |
| Section 18 Scheduler | 33 (failure recovery) | Wire retry policy per schedule task |
| Router section 8 (Skill catalog) | 37 (skills bootstrap) | Endpoint return "recommended" flag |

---

*Final: 2026-05-29 — Bagian 9 ditambahkan (pipeline patterns adopt dari Architect Agent). Roadmap 37 section di 9 bagian: Brain & Memory (1-9), Tools System (10-13), Slash Commands (14-17), Scheduler Runtime (18), Mesh Awareness (19-20), Wallet & Self-Sustenance (21-23), Security & Integrity (24-26), Codemap & Structure (27-30), Pipeline & Workflow Patterns (31-37).*


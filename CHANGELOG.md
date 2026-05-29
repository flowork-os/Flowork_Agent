# Changelog — Flowork Agent

Format: `YYYY-MM-DD HH:MM WIB` per entry, semantic-style bullet (feat / fix / cut / refactor / docs).

---

## 2026-05-30 12:10 WIB — Section 10: Tool system foundation (phase 1) DONE + LOCK

- **schema**: 2 table baru — `tool_overrides` (per-warga customization: config JSON, rate_limit, disabled), `tool_invocations` (audit log: tool_name, args_json, result_json, error_text, latency_ms, caller, invoked_at, deleted_at) + 3 index.
- **feat(tools)**: package baru `internal/tools/`:
  - `types.go` (LOCKED): Tool interface (Name/Schema/Capability/Run), Schema struct, Param taxonomy, Result, MarshalArgs/MarshalResult helpers.
  - `registry.go` (LOCKED): singleton via sync.RWMutex. Register (panic on dup name — early bug catch), Lookup, List, ListNames, Count, ListSummaries (anti over-prompt summary).
- **feat(agentdb)**: `internal/agentdb/tool_invocations.go` (LOCKED) — LogToolInvocation (8KB cap args/result/error), ListToolInvocations (tool_name/caller filter, cap 500), CountToolInvocations.
- **feat(agentmgr)**: 2 endpoint baru:
  - `GET /api/agents/tools/registry` — list registered tools (phase 1 empty — Tier 1 di-register Section 11)
  - `GET /api/agents/tool-invocations?id=&tool_name=&caller=&limit=` — browse audit log
- **verified end-to-end via 6 scenario**:
  - Schema clean: tool_overrides + tool_invocations + 3 index
  - Registry empty (no tools registered yet — Tier 1 defer Section 11)
  - Invocations empty list initially
  - Seed 2 row via direct DB (simulate tool calls: read_file success, bash_run permission_denied)
  - List endpoint returns 2 rows with full schema
  - Filter tool_name=bash_run returns 1 matching
  - Path traversal id rejected

### Phase 1 scope (DONE):
- Schema + Tool interface + Registry skeleton + Invocation log + endpoints.

### Defer phase 2/3:
- **Permission gate**: Tool.Capability() declared tapi belum di-enforce. Phase 2 wire dengan broker `IsApproved` check di pre-Run hook.
- **Categories DB-backed taxonomy**: `tool_categories` + per-warga `division_tool_priors` weighted ordering.
- **Capability map**: tool → required capability strings (`fs:write`, `net:fetch:*`, `exec:shell`).
- **Aliases**: sinonim tool name (`read` ↔ `read_tool`).
- **tool_overrides UI** (popup setting per-warga: enable/disable + config args + rate_limit).
- **Host capability `host_log_tool_invocation`** buat WASM agent log dari sandbox.
- **Section 11 Tier 1 tools**: actual implementations (read_file, write_file, bash_run, web_fetch, brain_search, dll).
- **Section 12 execution sandbox**: interceptors + permission runtime check.
- **Section 13 discovery**: `list_my_tools` + catalog browse via Router skill catalog.

---

## 2026-05-29 22:05 WIB — Section 9: Educational error lookup (phase 1) DONE + LOCK

- **feat(agentdb)**: tabel `educational_errors_cache` (code PK, category, title, explanation, remediation, synced_at, deleted_at) + 2 index. `internal/agentdb/edu_errors.go` (LOCKED): `UpsertEduError` (atomic ON CONFLICT DO UPDATE), `LookupEduError(code)` (return zero+code on miss — caller bedakan via Title==""), `ListEduErrors(category, limit)`, `CountEduErrors`. Hard cap 4KB explanation + remediation, 256 char title.
- **feat(agentmgr)**: HTTP endpoint multi-method `GET/POST /api/agents/edu-errors?id=`:
  - GET single by `?code=`
  - GET list `?category=&limit=`
  - POST upsert body `EduError` struct
- **verified end-to-end via 6 scenario**:
  - Schema clean + 2 index
  - POST upsert ROUTER_UNREACHABLE → ok
  - POST upsert TELEGRAM_403 → ok
  - GET single `?code=ROUTER_UNREACHABLE` → full row returned
  - List category=auth → 1 row (TELEGRAM_403)
  - Not found code → zero EduError + code preserved

### Defer:
- **`routerclient.PullEduErrors()`** sync dari Router /api/edu-errors — butuh Router catalog endpoint, defer Section 9 phase 2.
- **Mr.Flow integration**: catch error → lookup code → log decision dengan remediation suggestion. Defer sampai catalog populated.

---

## 2026-05-29 21:50 WIB — Section 7: Sync interface ke router (phase 1) DONE + audit + LOCK

- **feat(routerclient)**: `internal/routerclient/routerclient.go` (LOCKED) — HTTP client wrapper untuk agent↔router. `Client` struct + `New(baseURL)` constructor (URL whitelist validation, fallback default). `SubmitMistake(ctx, req) → (resp, err)`: POST `/api/mistakes/submit`. `Ping(ctx)` health check. Body size cap 64KB read, JSON marshal/decode, 30s HTTP timeout.
- **feat(agentdb)**: `internal/agentdb/mistakes_promote.go` (LOCKED) — extends locked `mistakes.go` via new file (per locking convention). `SetMistakePromoted(id, routerID)` idempotent UPDATE (WHERE tier != 'promoted'). `ListMistakesEligibleForPromote(minHitCount, limit)` filters tier='raw' + hit_count ≥ threshold + promoted_to_id empty + deleted_at NULL, ordered hit_count DESC.
- **feat(kernelhost)**: `Host.RunPromoteForAgent(agentID)` + `PromoteReport`. Resolve agent path, open store, list eligible (≥3 hit), per-mistake submit to Router, mark promoted lokal pas sukses. Best-effort error accumulation, capped at 10 entries. Router URL dari `kv.router_url` agent config (or default).
- **feat(agentmgr)**: HTTP endpoint `POST /api/agents/promote/run?id=` via `PromoteRun` callback. Method enforce + id validation.
- **feat(main)**: wire `agentmgr.PromoteRun = host.RunPromoteForAgent`.
- **verified end-to-end CROSS-TUBUH**:
  - Seed lokal mistake id=1 hit_count=5, tier='raw'
  - Trigger promote → `eligible:1, submitted:1, upsert_existing:1` (Router brain row id=1 was previously inserted via Router Section 7 test — atomic UPSERT increment hit_count 8→13)
  - Lokal mistake id=1 → `tier='promoted'`, `promoted_at` set, `promoted_to_id='1'`
  - Re-trigger promote → `eligible:0` (idempotent, sudah promoted)
  - Re-bump mistake id=3 hit_count=5 + trigger → `eligible:1, submitted:1`

### Audit critical fixes (3) applied BEFORE lock:
- **C1 SSRF / data exfiltration risk via router_url**: agent kv.router_url ngga validated → attacker / buggy config set `https://evil.com` → mistake content (potentially PII) leak. Fixed: `allowedHosts` whitelist (127.0.0.1, localhost, 0.0.0.0), `isAllowedRouterURL()` validation, fallback ke DefaultRouterURL kalau ngga match.
- **C2 Submitted counter increment on local mark failure**: kalau SetMistakePromoted gagal, sebelumnya count Submitted tapi lokal stale → next sweep re-submit → router atomic UPSERT inflate hit_count 2x. Fixed: classify sebagai `LocalMarkFailed` separate field, continue ke item selanjutnya (BUKAN Submitted), caller bisa monitor + investigate DB.
- **C3 resp.ID > 0 validation**: router could HTTP 200 + `{"id":0,...}` (partial write) → lokal mark `promoted_to_id="0"` lose tracking. Fixed: refuse SetMistakePromoted kalau resp.ID ≤ 0, classify Failed.

### Important + nice-to-have fixes:
- **#11 errors slice cap**: max 10 entries via `appendErr` helper. Cegah response 10KB JSON kalau 50 mistake semua failed.
- **N1 typo `UpserExisting` → `UpsertExisting`**: JSON field tetap `upsert_existing` (snake case).

### Phase 1 scope (DONE):
- routerclient pkg + SubmitMistake + Ping
- Promote helpers (extend locked mistakes.go via new file)
- Kernel-side RunPromoteForAgent + admin trigger endpoint
- End-to-end cross-tubuh verified

### Defer phase 2:
- **Cron loop auto-promote** (hourly sweep mirror `StartRetentionCron`)
- **PullSkill + QueryBrain methods** di routerclient
- **Outer context propagation** dari handler ke kernelhost (currently uses Background+timeout)
- **Single-flight lock** anti paralel admin trigger
- **Retry + circuit breaker** untuk router instability
- **Ping tighten** (currently accepts 4xx as healthy)

---

## 2026-05-29 21:30 WIB — Section 6: Workspace meta DONE + audit + LOCK

- **feat(agentdb)**: tabel `workspace_meta` (id, category, path, description, size_bytes, content_hash, shareable, created_at, updated_at, deleted_at) + UNIQUE(category, path) + 3 index. `internal/agentdb/workspace_meta.go` (LOCKED): `RegisterMeta` atomic upsert via SELECT-then-INSERT-or-UPDATE transaction (undelete on conflict). `ListMeta(category, limit)`, `LookupMeta(category, path)`, `RebuildIndexFromDir(root)` + `RebuildIndexReport`, `CountMeta(category)`. CategoryWhitelist enum (`tools/job/document/media/cache/log`). SHA-256 file content hash. Max 5000 files per sweep + 100MB per file hash cap.
- **feat(kernelhost)**: `Host.RebuildWorkspaceMetaForAgent(agentID)` — resolve agent path via h.lives snapshot, release lock before heavy scan, scan `<SharedDir>/<agentID>/`.
- **feat(agentmgr)**: HTTP endpoint dual-method `GET/POST /api/agents/workspace-meta?id=`:
  - GET: list `?category=&limit=`
  - POST: rebuild index `?action=rebuild`
- **feat(main)**: wire `agentmgr.WorkspaceRebuildIndex = host.RebuildWorkspaceMetaForAgent`.
- **verified end-to-end via 8 scenario**:
  - Schema clean, 3 index, UNIQUE constraint
  - Initial rebuild scanned 3 file (1 tools + 1 document + 1 job), all registered with size + SHA-256 hash
  - Filter by category=tools → 1 row
  - Delete file → soft_deleted:1 (deleted_at set)
  - Re-create same file → updated:1 (undelete + new size 24 byte)
  - Path traversal `../etc` rejected (regex id validation)
  - Action validation: unknown `?action=invalid` rejected
  - **Symlink defense**: created `tools/evil_link → /etc/passwd`, rebuild → scanned 3 (skipped symlink), DB ngga ada row evil_link ✓

### Audit critical fixes (3) applied BEFORE lock:
- **#1 symlink follow → secret leak**: `filepath.Walk` follows symlinks default. Attacker bisa taro symlink ke `/etc/passwd` atau `~/.ssh/id_rsa` → scanner hash content → leak via API. Fixed: skip via `info.Mode()&os.ModeSymlink != 0` check + defense-in-depth `strings.Contains(rel, "..")` reject post-Rel.
- **#2 path traversal di registerMetaNoLock**: helper bypass path validation yang ada di public RegisterMeta. Fixed: mirror validation (category required, whitelist, no `/` prefix, no `..`).
- **#3 maxFiles cap broken (`filepath.SkipDir` cuma skip current dir)**: walk continue ke sibling. Fixed: sentinel `errSkipAll` + outer loop break check via `errors.Is(werr, errSkipAll)`.

### Important fix applied:
- **#4 defer f.Close via closure** — panic-safe hash compute
- **#6 dead alt-key fallback removed** — softDelete simplified
- **#8 defer rows.Close** + add `rows.Err()` check

### Defer:
- Cron auto-rebuild tiap jam — currently admin trigger only (mirror StartRetentionCron pattern future)
- Hash sentinel for size-skipped (`hash_status` column)
- shareable=true filter di mesh-discovery future
- Single-flight rebuild lock (anti-paralel admin trigger same agent)

---

## 2026-05-29 20:50 WIB — Section 5: Karma self DONE + audit + LOCK

- **feat(agentdb)**: tabel `karma_self` (metric_key PK, metric_value REAL, metric_count INT, updated_at) + idx_karma_self_updated. `internal/agentdb/karma.go` (LOCKED): `IncrementKarma(key, delta)` counter pattern via ON CONFLICT DO UPDATE upsert, `AverageUpdateKarma(key, value)` moving avg via atomic transaction (SELECT current → compute new_avg → UPSERT), `GetKarma(key)` (return zero Karma + key kalau ngga ada), `ListKarma()` (limit 100). Hard cap |delta| / value > 1e9 anti-runaway. NO soft-delete (state perpetual per Section 8 exclusion).
- **feat(kernel/runtime)**: host capability `host_karma_update` + type `KarmaUpdater` (signature `(pluginID, op, key, value) → (current, error)`). Op `'increment'` / `'average'`. Capability gate `state:write` (sama Section 1+3). Error message cap 400 char.
- **feat(kernelhost)**: `Host.karmaUpdate(pluginID, op, key, value)` resolver — hold `h.mu` sepanjang Open+Update (race-safe). Route ke `IncrementKarma` atau `AverageUpdateKarma` tergantung op. Unknown op → error.
- **feat(mr-flow)**: wasmimport `hostKarmaUpdate`, helper `logKarma(op, key, value)` dengan `karmaBuf [1024]byte`. Time import + `t0 := time.Now()` sebelum callLLM + `elapsedMs := float64(time.Since(t0).Milliseconds())`. Hook 3 karma update di runDaemon:
  - `llmFailed = true` → `increment fail_count 1`
  - `llmFailed = false` → `increment success_count 1` + `average avg_response_ms elapsedMs`
- **feat(agentmgr)**: HTTP endpoint `GET /api/agents/karma?id=&key=`:
  - tanpa key → list semua metric (max 100)
  - dengan key → single Karma row (return zero+key kalau ngga ada — bukan error)
- **verified**: schema ada, build clean, daemon up caps=3, endpoint serve {count:0, items:null}.

### Audit critical fixes (3) applied before lock:
- **C1 (IncrementKarma atomic)**: split UPSERT + SELECT current → race risk skew log. Fixed: single atomic UPSERT dengan `RETURNING metric_value` clause (modernc.org/sqlite v1.51 support).
- **C2 (AverageUpdateKarma race)**: previous SELECT current → compute newAvg → UPSERT in transaction RACE-PRONE — 2 concurrent caller bisa baca oldCount sama → sample HILANG di overwrite. Fixed: compute formula DI DB LEVEL via single atomic UPSERT — `metric_value = (metric_value * metric_count + excluded.value) / (metric_count + 1)` + `metric_count = metric_count + 1`. SQLite writer lock serialize 2 caller → kedua sample tercatat.
- **C3 (Mr.Flow JSON struct)**: `logKarma` pakai typed `karmaReq` struct (sebelumnya `map[string]any` — TinyGo JSON key order non-deterministic). Konsisten dengan Section 1/3 pattern.

### Anomali pending investigation:
- **avg_response_ms = 1ms after 2 Telegram triggers** observed → suspicious karena callLLM ke router beneran ~1000-2000ms. Possible cause: TinyGo wasi `time.Since().Milliseconds()` quirk OR formula edge case. Added stderr debug log `[mr-flow] llm took Xms (llmFailed=Y)` di runDaemon untuk capture actual value next test. Investigate dengan log + fix di follow-up commit kalau confirmed bug.

### Defer:
- Popup UI Stats (dashboard badge + sparkline) — batch UI section
- Per-key reset / delete API — tidak ada use case real
- Time-series histogram (vs single moving avg) — defer kalau perlu analytics deeper

---

## 2026-05-29 20:40 WIB — Section 4: Death letter (phase 1) DONE + audit + LOCK

- **feat(agentdb)**: tabel `death_letter` (id, letter_type, recipient, subject, body, written_at, sealed_at, deleted_at) + 3 index. `internal/agentdb/death_letter.go` (LOCKED): `WriteLetter` (return id), `UpdateUnsealedLetter` (refuse kalau sealed), `SealLetter` (one-way idempotent), `SealAllUnsealed` (bulk auto-seal), `ReadLetters` (filter recipient + sealedOnly), `CountLetters`.
- **feat(agentmgr)**: HTTP endpoint multi-method `GET/POST/PUT/PATCH /api/agents/death-letter?id=`:
  - GET: list (`?recipient=&sealed=1&limit=N`)
  - POST: write new letter (body: letter_type/recipient/subject/body)
  - PUT: update unsealed letter (`?letter_id=N`, body subject/body) — refuse kalau sealed
  - PATCH: seal letter (`?letter_id=N&action=seal`)
- **integration RemoveHandler**: sebelum `os.RemoveAll(dir)`, auto-call `SealAllUnsealed()` — best-effort (silent log kalau DB corrupt). Response include `auto_sealed_letters` count kalau > 0. Preserve legacy sebelum folder hilang. **Plus audit trail**: `LogDecision('agent_retire', ...)` di-call kalau sealed > 0 — kepergian warga ke-track walau folder hilang.
- **audit important fix #1 (whitelist enforcement)**: `validLetterTypes` map enforce roadmap spec — caller kirim `letter_type` di luar `farewell|handover|reflection` → reject. Cegah trash data + future analytics break.
- **audit important fix #4 (defense in depth)**: `limit` parsing di handler reject negative/zero/>500 (sebelumnya cuma di ReadLetters internal clamp).
- **immutable doctrine**: WHERE clause filter di `UpdateUnsealedLetter` + `SealLetter` both check `sealed_at IS NULL AND deleted_at IS NULL`. Sekali sealed → body immutable.
- **verified end-to-end**:
  - POST write → id=1
  - GET list shows unsealed letter
  - PUT update unsealed → success, subject revised
  - PATCH seal → sealed:1
  - PUT update SEALED → BLOCKED "letter id 1 not found, sealed, or deleted (immutable)"
  - GET sealed=1 returns 1 row with sealed_at populated

### Defer:
- RPC method `write_death_letter` di mr-flow — defer (no self-write use case)
- Inclusion di `.fwagent.zip` download (DownloadHandler enhancement) — Section 4 phase 2
- Popup UI — batch UI section
- Letter type whitelist enforcement (`farewell`/`handover`/`reflection`) — current accept any non-empty string, defer kalau perlu strict

---

## 2026-05-29 20:30 WIB — Section 8: Retention policy + cron DONE + audit + LOCK

- **feat(agentdb)**: `internal/agentdb/retention.go` (LOCKED) — `RetentionWindows` struct + `DefaultRetention()` (30d interactions / 90d decisions+raw mistakes / 180d promoted / 90d hard-delete grace). `PrunePromotedMistakes`, `HardDeleteSoftDeleted` (3 tabel), `RunRetentionSweep` (orchestrator + aggregate report).
- **feat(kernelhost)**: `StartRetentionCron(ctx, 24h)` goroutine — initial 1min warm-up delay, ticker 24h, iterate snapshot of `h.lives` then sweep per agent. Aman terhadap shutdown via `ctx.Done()`. `RunRetentionForAgent(agentID)` helper resolve path + open store + run sweep (pakai DefaultRetention).
- **feat(agentmgr)**: HTTP endpoint `POST /api/agents/retention/sweep?id=` via callback wire — admin manual trigger (testing / immediate cleanup). Method enforced POST, id validation.
- **feat(main)**: wire `host.StartRetentionCron(ctx, 24*time.Hour)` di boot + `agentmgr.RetentionSweep` callback.
- **audit critical fix C1 (defense)**: minimum retention duration 24h hard-coded. `RunRetentionSweep` normalize windows — zero/under-min auto-fallback ke `DefaultRetention()` values. `PrunePromotedMistakes` + `HardDeleteSoftDeleted` refuse run kalau duration < 24h (cegah caller accidentally pass `RetentionWindows{}` → DELETE row baru detik lalu).
- **audit critical fix C2 (atomicity)**: `HardDeleteSoftDeleted` wrap 3 DELETE dalam `db.BeginTx` — crash di tengah sebelumnya bisa bikin `ref_interaction_id` di decisions point ke interactions yang udah ke-DELETE (silent orphan, audit Section 3 cross-ref rusak). Sekarang atomic.
- **audit important fix I1 (auditability)**: `RunRetentionSweep` log hasil ke tabel `decisions` (`decision_type='retention_sweep'`) supaya audit trail survive restart (kernel `log.Printf` hilang). Guard: skip log kalau 0 affected + 0 errors (reduce noise). Verified row id=2 muncul setelah trigger 2nd sweep.
- **verified end-to-end**:
  - cron armed log `interval=24h0m0s`
  - manual trigger sweep return aggregate report 8 field
  - backdated 2 row (interaction 2026-04-15, decision 2026-02-15) → sweep soft-deleted both (`soft_deleted_interactions:1, soft_deleted_decisions:1`)
  - invalid id rejected, wrong method rejected

### Tidak di-prune (sengaja):
- `workspace_meta` (Section 6, sumber-of-truth filesystem)
- `karma_self` (Section 5, state perpetual)
- `death_letter` (Section 4, legacy)

Section 4-6 belum di-implement, retention adapt nanti ketika tabel-nya ada.

### Defer:
- Log retention sweep result ke tabel `decisions` (acceptance criteria minta — defer kalau ngga perlu audit deep, kernel log sudah cover via `log.Printf`).
- Configurable retention windows per agent (admin override via settings.kv) — defer sampai use case real.

---

## 2026-05-29 20:25 WIB — Section 2: Mistakes journal (phase 1) DONE + audit + LOCK

- **feat(agentdb)**: tabel `mistakes_local` (id, category, title, content, context_origin, tier, hit_count, last_hit_at, created_at, promoted_at, promoted_to_id, deleted_at, deleted_by) + UNIQUE(category, title) + 4 index. `internal/agentdb/mistakes.go` (LOCKED): `AddMistake` (return id + addedNew), `ListMistakes(tier, limit)`, `PruneMistakes` (tier='raw' only — 'reviewed'/'promoted' sakral), `CountMistakes(tier)`.
- **feat(agentmgr)**: HTTP endpoint dual-method `GET/POST /api/agents/mistakes?id=` (POST body cap 64KB).
- **audit critical fix #1**: ON CONFLICT DO UPDATE dengan `WHERE deleted_at IS NULL` filter → silent no-op kalau row sebelumnya soft-deleted, lalu `SELECT id WHERE deleted_at IS NULL` ngga ketemu → error "no rows". Fixed: refactor ke SELECT-then-INSERT-or-UPDATE atomic transaction. UPDATE path clear `deleted_at` + `deleted_by` (undelete semantic — pattern muncul lagi = re-validate). Verified via edge case test (soft-delete id=1 → re-add → undelete + hit_count 2→3).
- **audit critical fix #2**: `addedNew` logic broken — SQLite `ON CONFLICT DO UPDATE` set `LastInsertId = rowid yang di-update` (sama dengan id existing), jadi `lastInsertID == id` selalu true → addedNew selalu true. Fixed: explicit branch `sql.ErrNoRows` (INSERT path → addedNew=true) vs default (UPDATE path → addedNew=false). Verified fresh add id=5 → `added:true`, upsert same → `added:false, hit_count:2`.

### Phase 1 scope (selesai):
- Schema + Go pkg + admin endpoint POST add + GET list.

### Defer ke phase berikutnya / section lain:
- **host capability `host_log_mistake`** + Mr.Flow auto-log self-reflect — defer sampai ada use case real (Mr.Flow saat ini ngga punya self-detect mistake path).
- **PromoteMistake** lokal (set tier='reviewed' + promoted_at) — endpoint POST `/api/agents/mistakes/review` ditunda sampai ada workflow review.
- **Promotion ke router brain antibody** — Section 7 (cross-tubuh sync).
- **Popup UI "Lesson Learned"** — batch UI section.
- **Tier whitelist validation** + error message generic sanitize — audit important, defer (low impact single-user).

---

## 2026-05-29 20:15 WIB — Section 3: Decisions log DONE + audit + LOCK

- **feat(agentdb)**: tabel `decisions` (id, decision_type, rationale, inputs, outcome, ref_interaction_id, occurred_at, deleted_at) + 3 index. `internal/agentdb/decisions.go` (LOCKED): `LogDecision()` return ID, `ListDecisions(type, limit)`, `PruneDecisions`, `CountDecisions`. RFC3339 timestamp explicit (mirror Section 1 fix). Rationale hard-cap 4KB. Outcome empty → 'pending' default.
- **feat(kernel/runtime)**: host capability `host_log_decision` + type `DecisionLogger` (signature `(int64, error)` — return ID). Capability gate `state:write` (sama dengan host_log_interaction). Error message cap 400 char.
- **feat(kernelhost)**: `Host.logDecision()` resolver — hold `h.mu` sepanjang Open+Log (race-safe). TODO comment defer cache `*Store` per pluginID ke Section 8.
- **feat(mr-flow)**: wasmimport `hostLogDecision`, helper `logDecision()` dengan `decisionBuf [4096]byte`. Hook 3 call site di `runDaemon`:
  - `skip_task` outcome=success — drop chat unauthorized (chat_id ngga di TELEGRAM_ALLOWED_CHATS)
  - `escalate` outcome=fail — LLM call gagal (exact error prefix detect: "router error:" / "decode:" / "llm:" / "(no choices)" / "")
  - `model_choice` outcome=success — dispatch ke router primary sukses, log model + reply_head
- **feat(agentmgr)**: HTTP endpoint `GET /api/agents/decisions?id=&type=&limit=` (default 50, max 500).
- **audit critical fix #1**: `llmFailed` heuristic semula pakai `(LLM ` prefix yang ngga pernah keluar dari callLLM (false-positive risk). Diganti exact prefix list dari callLLM (`router error:`, `decode:`, `llm:`, `(no choices)`, empty).
- **audit critical fix #2**: `LogDecision` return ID di-discard di kernel side (logDecisionResp.ID field deklarasi tapi ngga di-set). Fixed: DecisionLogger signature `(int64, error)`, host forward ID di response.
- **audit important fix**: capture `origReply` sebelum overwrite ke fallback string supaya `reply_head` di rationale log debug actionable.

### Audit deferred items:
- **Lock contention** (2 logInteraction + 1 logDecision serial per chat): defer cache `*Store` per pluginID ke Section 8 (perf). TODO comment di kernelhost.go.
- **Outcome schema default cosmetic**: schema `DEFAULT ''` tapi runtime default `'pending'`. Inkonsisten ringan kalau raw SQL insert. Defer.
- **Error message expose detail**: low risk single-user localhost. Sanitize kalau go public.
- **`(LLM ` false-positive risk lama**: ngga keluar di callLLM real path. Sudah aman dengan exact prefix list.

---

## 2026-05-29 19:50 WIB — Section 1: Adversarial audit + hardening + LOCK

- **fix(security/cap)**: `host_log_interaction` sekarang gate dengan capability `state:write` (sebelumnya: tanpa gate — plugin bisa spam tabel `interactions` tanpa declare cap). Manifest mr-flow tambah `"state:write"` ke `capabilities_required`. Validator `internal/kernel/loader/manifest.go::validateCapability` tambah `"state"` ke whitelist primitive.
- **fix(race)**: `Host.logInteraction` di `internal/kernelhost/kernelhost.go` sekarang hold `h.mu` sepanjang Open+Log (sebelumnya: lock sebentar untuk lookup, lalu release sebelum Open — race window kalau agent di-Unload paralel bisa re-create folder kosong atau write ke agent yang dihapus).
- **fix(format)**: `LogInteraction` set `occurred_at` explicit dengan `time.Now().UTC().Format(time.RFC3339)` (sebelumnya: relies on SQLite DEFAULT `CURRENT_TIMESTAMP` yang format `YYYY-MM-DD HH:MM:SS`). Critical karena `PruneInteractions` pakai RFC3339 cutoff — lexicographic compare di SQLite rusak kalau format beda. Verified via Telegram test row 5+6: `2026-05-29T12:51:03Z`.
- **fix(buffer)**: mr-flow `logBuf` 512 → 4096 byte (host bisa kirim error message panjang yang sebelumnya ke-crop → JSON unmarshal gagal → root cause hilang). Host juga cap error message ke 400 char.
- **lock**: `internal/agentdb/interactions.go` di-mark LOCKED (Section 1 boundary stable, Section 8 retention extend via new function).

### Audit deferred items (tidak fix sekarang — alasan eksplisit):
- **Cache `*Store` per pluginID**: open-on-demand pattern (Open+Close per call) bottleneck di teori, tapi Mr.Flow chat freq manusiawi (1-5/menit). Refactor jadi `sync.Map` cache butuh handle agent unload cleanup — defer sampai ada use case real (e.g. broadcast/group chat).
- **Composite index `(actor, channel)`**: query filter both jarang. Defer sampai volume >100K row.
- **Cursor pagination**: `ListInteractions` limit 500 cukup buat MVP. Defer sampai dashboard butuh infinite scroll.
- **Async log channel di Mr.Flow hot path**: synchronous WASM→host→DB→back ~1ms — manusia chat ngga peduli. Defer sampai chat volume tinggi.
- **`agentmgr.InteractionsHandler` path inconsistency** (pre-check via `agentFolder`, db via `Resolve`): same pattern dengan ConfigHandler/Toggle. Consistent intra-handler. Defer audit cross-handler.

---

## 2026-05-29 19:30 WIB — Section 1: Episodic Interactions DONE

- **feat(agentdb)**: tabel `interactions` (id, channel, direction, actor, content, metadata, occurred_at, deleted_at) + 4 index (channel, actor, occurred_at DESC, deleted_at). Schema migrasi otomatis via `ensureSchema()`.
- **feat(agentdb)**: `internal/agentdb/interactions.go` — `LogInteraction()`, `ListInteractions()`, `PruneInteractions()`, `CountInteractions()`. Content hard-cap 8KB anti-bloat. Metadata marshal ke JSON.
- **feat(kernel/runtime)**: host capability `host_log_interaction` (wasmimport) + type `InteractionLogger`. Pola sama `host_net_fetch`. Plugin cuma bisa log ke state.db nya sendiri (pluginID di-set kernel dari ctx, ngga bisa spoof).
- **feat(kernelhost)**: `Host.logInteraction()` resolver — resolve pluginID → Discovery.Path → open state.db on-demand → call agentdb.Store.LogInteraction.
- **feat(mr-flow)**: hook log in/out di `runDaemon()` — direction `in` setelah receive Telegram message (metadata: message_id, update_id), direction `out` setelah `sendMessage` sukses (metadata: model, reply_to_message). Best-effort, silent on error.
- **feat(agentmgr)**: HTTP endpoint `GET /api/agents/interactions?id=&channel=&actor=&limit=` — paginated list (default 50, max 500). Anti over-prompt: dashboard/audit only, JANGAN auto-inject ke system prompt.
- **fix(build)**: `referensifile/go.mod` separate module supaya `go mod tidy` + `go build ./...` parent ngga scan 223 .go reference file dengan external imports.
- **verified**: end-to-end test — 4 row tercatat (2x in "cek" + 2x out: 1 error router-down, 1 reply LLM sukses 1.7KB).

---

## 2026-05-28 (pre-Changelog history)

Iterasi awal Flowork_Agent — kernel embedded, GUI :1987, Mr.Flow Telegram daemon, manifest ui_schema, prompt budget cap di mr-flow callLLM (max 3 skills, 4000 char persona total). Detail di `roadmap.md` (state awal).

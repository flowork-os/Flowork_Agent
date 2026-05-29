# Changelog — Flowork Agent

Format: `YYYY-MM-DD HH:MM WIB` per entry, semantic-style bullet (feat / fix / cut / refactor / docs).

---

## 2026-05-30 14:35 WIB — Section 15: Tier 1 slash commands (5 productive) DONE + LOCK

- **feat(slashcmd)**: `internal/slashcmd/context.go` (LOCKED) — mirror tools/context.go pattern. `WithStore/FromStore`, `WithCaller/FromCaller`, `WithAgent/FromAgent`. ctxKey private anti-collision.
- **feat(slashcmd/builtins/tier1.go)** (LOCKED): 5 productive commands + InitTier1():
  - **/version** (aliases: ver, v) — daemon version, tools count, slash count, agent ID
  - **/now** (aliases: time, date) — UTC RFC3339 + WIB local (UTC+7) + unix_ms
  - **/stats** (alias: status) — karma metrics + counts (interactions/decisions/mistakes/letters/edu_errors/tool_invocations)
  - **/tools** — list builtin tools dengan capability grouped by prefix (fs/net/rpc/state/time/none)
  - **/interactions** (aliases: chat, history) — last 10 Telegram interactions with direction + actor + content preview
- **plumbing**: kernelhost.dispatchSlash + agentmgr.SlashRunHandler open store + inject ke ctx via WithStore. SlashDispatcherFunc signature extended dengan ctx param (anti circular import note updated).
- **feat(builtins.go)**: Init() now calls InitTier1() (8 total slash commands).
- **verified end-to-end via 6 scenario**:
  - Registry lists 8 commands sorted alphabetical
  - /version returns "Flowork Agent 0.4.0-embedded-kernel" + 11 tools + 8 slash commands
  - /now returns UTC + WIB local + unix_ms
  - /stats returns karma (success_count=2, avg_response_ms=3016ms n=2) + counts (24 interactions, 6 decisions, 3 mistakes, 2 letters, 2 edu_errors, 29 tool_invocations)
  - /tools groups 11 tools by capability prefix (fs/net/rpc/state/time/none)
  - /interactions returns last 10 Telegram in/out chronologically
  - /v alias resolves to version

### Section 11 + 14 + 15 + 17 stack:
- 11 builtin tools (echo, now, memory_x3, file_x3, brain_search, telegram_send, webfetch)
- **8 builtin slash commands** (help, echo, ping + version, now, stats, tools, interactions)
- Mr.Flow Telegram bot detects `/` → dispatcher → reply tanpa LLM (token saving)
- `/help`, `/ping`, `/version`, `/stats`, `/tools`, `/interactions` ready untuk Mr.Dev kirim ke Telegram

### Defer phase 2+:
- More Tier 1: /search (wrap brain_search tool), /memory (wrap memory_get/set), /agents (list warga, multi-warga future), /mistakes (last 5)
- Custom command loader Section 16 (.md files from workspace)
- Permission gate (broker check) per-command capability

---

## 2026-05-30 14:15 WIB — Section 17: Mr.Flow Telegram /slash integration DONE

- **feat(kernel/runtime)**: host capability `host_slash_dispatch` (4-arg uint32 pattern same as host_log_*). `SlashDispatcher` type + `hostState.slash` field + `slashDispatch()` method. Capability gate `state:write`. Plugin sends `{text, caller?}`, host parses + dispatches via callback + return `{ok, command, text, error}`. Result text cap 8KB anti-overflow guest buffer.
- **feat(kernel/runtime)**: Bootstrap signature extended dengan SlashDispatcher param.
- **feat(kernelhost)**: `SlashDispatcherFunc` package-level callback var (anti circular import dengan slashcmd). `Host.dispatchSlash` resolver — resolve agent path, call callback, log invocation per-agent via `store.LogSlashInvocation` (best-effort, ngga blocking guest reply).
- **feat(main.go)**: wire `kernelhost.SlashDispatcherFunc = func(...) { slashcmd.Dispatch(ctx, text) ... }`.
- **feat(mr-flow/main.go)**: `wasmimport host_slash_dispatch` + helper `dispatchSlash()` dengan `slashBuf [16384]byte`. Branch di `runDaemon`: kalau message text mulai `/`, skip LLM call + dispatch via host, send slash result back ke Telegram dengan source='slash' di metadata.
- **Mr.Flow caps now 3**: `net:fetch:https://api.telegram.org`, `net:fetch:http://127.0.0.1:2402/v1/chat/completions`, `state:write` (shared dengan log_interaction/log_decision/karma/slash).

### Integration ready, behavior verify pending Telegram trigger:
- Daemon up `caps=3`
- WASM rebuilt 282KB
- Mr.Flow detects leading `/` → branch ke host_slash_dispatch (skip LLM = no token waste)
- Caller format: `telegram:<chat_id>` propagated ke audit log
- Reply path: slash result → sendMessage → logInteraction direction='out' source='slash'

### End-to-end test path (Mr.Dev → bot):
- `/help` → list 3 commands
- `/ping` → "pong"
- `/echo halo` → "halo"
- `/xyz` → "command not found: /xyz"
- `text without slash` → fallback ke LLM (unchanged behavior)

---

## 2026-05-30 13:50 WIB — Section 14: Slash command foundation (phase 1) DONE + LOCK

- **schema**: 2 table baru — `slash_invocations` (audit log: command, args, caller, result_text, error_text, duration_ms, invoked_at, deleted_at) + 3 index; `slash_aliases` (alias→canonical mapping, PK alias).
- **feat(slashcmd)**: package baru `internal/slashcmd/`:
  - `types.go` (LOCKED): SlashCommand interface (Name/Aliases/Description/Run), Result (Text + Format)
  - `registry.go` (LOCKED): singleton via sync.RWMutex. Register panic on dup name OR alias collision. Lookup resolves name OR alias case-insensitive
  - `dispatcher.go` (LOCKED): `Dispatch(ctx, text)` → (Result, cmdName, error). Parse: strip "/", split first token as name, rest as argsRaw
- **feat(slashcmd/builtins)**: `internal/slashcmd/builtins/builtins.go` (LOCKED) — 3 commands + Init():
  - `/help` (aliases: h, ?) — list all registered commands dengan descriptions, markdown format
  - `/echo <text>` — echo input back
  - `/ping` (alias pong) — health check, returns "pong"
- **feat(agentdb)**: `internal/agentdb/slash_invocations.go` (LOCKED) — LogSlashInvocation (8KB cap fields), ListSlashInvocations (command/caller filter, cap 500).
- **feat(agentmgr)**: 3 endpoint:
  - `POST /api/agents/slash/run?id=<agent>` body `{text, caller?}` → dispatch + log
  - `GET /api/agents/slash/registry` → list registered commands
  - `GET /api/agents/slash-invocations?id=&command=&caller=&limit=` → browse audit log
- **feat(main.go)**: `slashbuiltins.Init()` panggil early sebelum kernel boot.
- **verified end-to-end via 10 scenario** + 7 invocation log rows:
  - Schema clean: slash_invocations + slash_aliases + 3 index
  - Registry lists 3 commands sorted alphabetical
  - `/help` returns markdown list dengan aliases
  - `/h` alias resolves to help → text_len 218
  - `/echo halo Mr.Flow phase 14 verify` → returns input back
  - `/ping` → "pong"
  - `/pong` (alias) → resolves to ping, returns "pong"
  - Unknown `/nonexistent` → 404 error logged
  - Plain text "plain text" → "not a slash command (missing /)"
  - `/echo` missing args → "usage: /echo <text>" error logged
  - Audit log captures 7 invocations dengan correct caller + duration + error_flag

### Phase 1 scope (DONE):
- Schema + interface + registry + dispatcher + 3 demo commands + 3 endpoints + audit log.

### Defer phase 2+:
- **Section 15 Tier 1 commands**: `/search /list /stats /agents /tools /skill /memory /now /uptime /version` dst — real productive commands.
- **Section 16 custom command loader**: `.md` files di `<workspace>/.flowork/commands/*.md` → auto-register.
- **Section 17 integration handler**: Mr.Flow Telegram bot detect leading `/` → call dispatcher (via host capability host_slash_dispatch).
- **Fuzzy match fallback**: kalau `/sumar` typo → suggest `/summarize`.
- **Skill catalog fallback**: kalau slash ngga di-register, query Router skill catalog (Section 8 Router done).
- **Permission gate**: pre-Run check broker capability (mirror tools).

---

## 2026-05-30 13:30 WIB — Section 11 phase 1d: webfetch (SSRF-guarded) DONE + LOCK

- **feat(tools/builtins)**: `internal/tools/builtins/web.go` (LOCKED) — `webfetch` tool (capability `net:fetch:*`). Defense:
  - Scheme whitelist: http, https only (file/javascript/etc rejected)
  - Hostname resolve via net.LookupIP + IP CIDR block: 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 (cloud metadata), IPv6 ::1/128, fc00::/7, fe80::/10
  - CheckRedirect re-validates target + strips Authorization header
  - Response body cap 1MB, HTTP timeout 30s
  - User-Agent identifies Mr.Flow
- `Init()` register webfetch (11 builtin tools total).
- **verified end-to-end via 6 SSRF + 1 real fetch scenario**:
  - 127.0.0.1 → blocked "private/loopback/metadata range"
  - 169.254.169.254 (AWS/GCP IMDS) → blocked
  - 192.168.1.1 (private LAN) → blocked
  - file:// scheme → blocked "scheme must be http/https"
  - https://example.com → status 200, 528 bytes HTML body fetched ✓
  - Missing url → reject

### Section 11 progress (auto-incremental):
- Phase 1a (5 demo): DONE
- Phase 1b (3 file ops): DONE
- Phase 1d (webfetch): DONE
- Phase 1e (brain_search): DONE
- Phase 1f (telegram_send): DONE — **11 builtin tools live**
- Phase 1c shell (bash_run): defer (sandbox harder)
- Phase 1g task/plan/todo orchestration: defer P2

---

## 2026-05-30 13:15 WIB — Section 11 phase 1f: telegram_send DONE + LOCK

- **feat(tools/builtins)**: `internal/tools/builtins/telegram.go` (LOCKED) — `telegram_send` tool (capability `net:fetch:telegram`). Bot token + allowed_chats from agent `secrets` table via `Store.Secrets()`. Triple security:
  - Token never logged atau echo back ke caller
  - chat_id WAJIB ada di `TELEGRAM_ALLOWED_CHATS` (anti-spam guard) — chat_id `9999999999` test rejected
  - Text cap 4096 char (Telegram API limit) + truncate dengan "…"
- HTTP timeout 15s, body cap 64KB on response.
- `Init()` register telegram_send (10 builtin tools total).
- **verified end-to-end** + real Telegram message landing:
  - Missing chat_id → "chat_id required (non-zero)"
  - Missing text → "text required (non-empty)"
  - chat_id 9999999999 → "not in TELEGRAM_ALLOWED_CHATS (anti-spam guard)"
  - Real allowed chat_id 2012305087 → **message_id 3871, 366ms send sukses**, Mr.Dev's phone received: "🎯 Section 11 phase 1f verify..."

### Section 11 progress:
- Phase 1a (5 demo): DONE
- Phase 1b (3 file ops): DONE
- Phase 1e (brain_search): DONE
- Phase 1f (telegram_send): DONE — **10 builtin tools live**
- Phase 1c shell (bash_run): defer (security review)
- Phase 1d web (webfetch): defer
- Phase 1g task/plan/todo: defer P2

---

## 2026-05-30 13:00 WIB — Section 11 phase 1e: brain_search (cross-tubuh tool) DONE + LOCK

- **feat(routerclient)**: `internal/routerclient/brain_search.go` (NEW unlocked) — extend Client dengan `SearchBrain(ctx, query, k)` method. GET `/api/brain/search-drawers?query=&k=` ke Router. Body cap 512KB. k validation (default 5, max 20). Mirror existing brainSearchDrawersHandler response shape.
- **feat(tools/builtins)**: `internal/tools/builtins/brain.go` (LOCKED) — `brain_search` tool (capability `rpc:router:brain`). Resolve router_url dari agent kv config (mirror kernelhost.RunPromoteForAgent pattern). Args: `{query, k}`. Returns `{query, hits[wing/room/content/score/drawer_id], count}`. k normalize float64→int (JSON number type), default 5, max 10 anti over-prompt.
- **feat(builtins.go)**: extend `Init()` register brain_search (total 9 builtin tools).
- **verified end-to-end cross-tubuh chain**: Agent dispatcher → routerclient.SearchBrain → Router `/api/brain/search-drawers` (handlers_brain_views.go) → brain.Retrieve BM25/FTS → 859K drawer brain → top-K hits returned.
  - Registry 9 tools alphabetical
  - query 'Section 1' → 3 hits dari general/knowledge + general/final_general dengan score ~0.107 (Davis Municipal Code drawer match)
  - query 'cek log' → 2 hits dari general/openai + general/fallback rooms
  - Missing query rejected
  - Latency 260ms (network round-trip ke Router :2402)

### Section 11 progress:
- Phase 1a (5 demo): DONE
- Phase 1b (3 file ops): DONE
- Phase 1e (brain_search): DONE — **9 builtin tools live, cross-tubuh verified**
- Phase 1c shell (bash_run): defer (security review needed)
- Phase 1d web (webfetch): defer
- Phase 1f comms (telegram_send): defer
- Phase 1g task/plan/todo: defer P2

---

## 2026-05-30 12:45 WIB — Section 11 phase 1b: 3 file ops tools + SharedDir plumbing

- **feat(tools/builtins)**: `internal/tools/builtins/file.go` (LOCKED) — 3 tool implementations:
  - **file_read** (`fs:read:/shared/*`) — read file by `{category, name}`, 4MB cap, truncated flag
  - **file_write** (`fs:write:/shared/*`) — create/overwrite file, 4MB cap, bytes_written return
  - **file_list** (`fs:read:/shared/*`) — list filenames di category, symlinks skipped (audit Section 6 pattern)
- **security**: triple path defense — (1) category whitelist (tools/job/document/media/cache/log mirror SharedSubfolders), (2) `filepath.Base()` strips traversal, (3) defense-in-depth `strings.HasPrefix(abs, sharedDir+sep)` post-Join sanity.
- **feat(tools/context.go)**: extended dengan `WithSharedDir/FromSharedDir` ctx helpers. ctxKey enum added `keySharedDir`.
- **feat(kernelhost)**: `Host.SharedDirForAgent(agentID)` — return absolute path `<SharedDir>/<agentID>/`.
- **feat(agentmgr)**: `SharedDirForAgent` callback var + dispatcher inject ctx kalau callback wired.
- **feat(main.go)**: wire `agentmgr.SharedDirForAgent = host.SharedDirForAgent`.
- **feat(builtins.go)**: extend `Init()` register 3 file tools (total 8 builtin).
- **verified end-to-end via 8 scenario** + disk inspection:
  - Registry 8 tools (5 demo + 3 file) sorted alphabetical
  - file_write document/section-11-1b-test.md (64 bytes) → disk verified
  - file_read content preserved exactly
  - file_list document returns 2 files (existing test_note.md + new)
  - Path traversal `../../etc/passwd` → filepath.Base strips → "passwd" not found di document/ (BLOCKED safely)
  - Invalid category 'BAD!' → whitelist rejected
  - File not found → clear error
  - Empty category cache → count:0

### Section 11 progress:
- Phase 1a (5 demo tools): DONE
- Phase 1b (3 file ops): DONE — 8 builtin tools total
- Phase 1c shell (bash_run): defer
- Phase 1d web (webfetch): defer
- Phase 1e brain (search/recall): defer
- Phase 1f comms (telegram_send): defer
- Phase 1g task/plan/todo: defer P2

---

## 2026-05-30 12:30 WIB — Section 11: Tool Tier 1 phase 1a (5 demo tools + dispatcher) DONE + LOCK

- **schema**: tabel `tool_memory` (k PK, v, updated_at) WITHOUT ROWID — separate dari existing `kv` table supaya ownership tool terisolasi.
- **feat(agentdb)**: `internal/agentdb/tool_memory.go` (LOCKED) — `GetToolMemory` (return value + found bool), `SetToolMemory` (atomic UPSERT, 32KB value cap, 256B key cap), `DelToolMemory` (DESTRUCTIVE physical remove — schema no deleted_at), `ListToolMemoryKeys` (cap 100, keys-only anti over-prompt).
- **feat(tools)**: `internal/tools/context.go` (LOCKED) — ctx propagation helpers: WithStore/FromStore (`*agentdb.Store`), WithCaller/FromCaller (mis. 'daemon', 'http-admin', 'rpc'), WithAgent/FromAgent (agent ID). ctxKey type private anti collision.
- **feat(tools/builtins)**: `internal/tools/builtins/builtins.go` (LOCKED) — 5 tool implementations + `Init()` bootstrap:
  - **echo** (capability: none) — return input message
  - **now** (`time:read`) — return RFC3339 + unix_ms
  - **memory_get** (`state:read`) — read tool_memory by key, return found bool
  - **memory_set** (`state:write`) — atomic upsert
  - **memory_delete** (`state:write`) — DESTRUCTIVE remove
- **feat(agentmgr)**: `ToolRunHandler` POST `/api/agents/tools/run?id=<agent>` body `{tool_name, args, caller?}`. Lookup tool dari registry, inject store+caller+agent ke ctx, dispatch Run, log invocation (best-effort), return Result. MaxBytesReader 64KB.
- **feat(main.go)**: `builtins.Init()` panggil early sebelum kernel boot. Panic on duplicate name (early bug catch).
- **verified end-to-end via 10 scenario** + 9 invocation row di tool_invocations:
  - Registry lists 5 tools (sorted by name)
  - echo returns input
  - now returns RFC3339 + unix_ms
  - memory_set + get full lifecycle (write → read found:true → delete → re-read found:false)
  - Unknown tool rejected via "tool not registered: nonexistent"
  - Echo missing required arg → error logged with latency
  - Invocation log captures BOTH success + error path dengan caller correctly attributed

### Phase 1a scope (DONE):
- Foundation pattern proven: Register → Lookup → Run via ctx (store/caller/agent) → LogInvocation → Result return.

### Defer phase 1b/1c/1d (real Tier 1 tools):
- **1b file ops**: read, write, edit, multiedit, glob, grep, list (~950 LOC) — needs path traversal validation + workspace sandbox
- **1c shell**: bash_run (~250 LOC) — exec.CommandContext + 30s timeout + capture stdout/stderr
- **1d web**: webfetch (~150 LOC) — pipe ke existing host_net_fetch host capability (or direct HTTP client)
- **1e brain**: brain_search, brain_recall (~160 LOC) — routerclient.QueryBrain (defer routerclient extension)
- **1f comms**: telegram_send (~80 LOC) — reuse Mr.Flow sendMessage logic
- **1g task/plan/todo**: orchestration (~700 LOC) — heaviest, defer P2

### Section 11 phase 2 (security):
- Permission gate enforce: dispatcher check `tools.Tool.Capability()` against broker `IsApproved(agentID, cap)` before Run.
- Rate limiting via `tool_overrides.rate_limit` field.
- Tool disable toggle via `tool_overrides.disabled`.

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

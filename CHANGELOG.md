# Changelog — Flowork Agent

Format: `YYYY-MM-DD HH:MM WIB` per entry, semantic-style bullet (feat / fix / cut / refactor / docs).

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

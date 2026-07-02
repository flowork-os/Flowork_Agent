# WORKLOG — Papan Kerja Bersama Lintas-Agent (PR-0 Sistem Saraf Otonom)

> Owner: Aola Sahidin (Mr.Dev) · floworkos.com. Pondasi buat MANDOR (idle-supervisor): biar bisa
> lihat "siapa ngerjain apa, mana yang nyangkut" lintas SEMUA agent. Arsitektur: lock/ARSITEKTUR.md.

## KENAPA
Mandor (roadmap P0-B: PC idle → bangunin supervisor → rekonsiliasi kerjaan belum kelar) ga bisa kerja
tanpa SATU tempat lihat status kerja semua agent. Dulu: `agent_runs` tersebar per-agent (state.db
masing-masing), ga ada view terpadu. Worklog nutup gap itu — TANPA bikin sistem dobel.

## CARA KERJA (aggregator READ-only — cabut-akar, bukan tambal)
Satu sumber kebenaran = **`agent_runs`** (internal/tools/builtins/agent_run.go, NON-frozen) — tabel
lifecycle task panjang per-agent (`id,label,state,checkpoint,updated`; state: pending→running→
paused⇄running→done|stopped). Worklog cuma men-SCAN, ga nambah tabel/sistem baru.

- File **`agent/feature_worklog.go`** (NON-frozen, package main) daftar lewat SEAM feature-registry
  (`RegisterFeature`, fase `PhaseRoute`) → mount `GET /api/worklog`. NOL sentuh frozen (main.go beku tetap utuh).
- Handler: `Host.AgentIDs()` → tiap agent `Host.OpenAgentStore(id)` → `store.DB()` → `SELECT ... FROM agent_runs`.
  Agent yang belum pernah pakai agent_run (tabel ga ada) → di-skip (fail-safe). Store TIDAK di-close (handle dikelola Host).
- Output JSON: `{enabled,count,stale_min,items:[{agent,id,label,state,updated,stale,priority}]}`.
  - `?all=1` = ikutin yang done/stopped. Default = cuma yang BELUM kelar (yang Mandor peduliin).
  - `stale=true`: state running/paused tapi `updated` lewat ambang → NYANGKUT.
  - **`priority` (owner 2026-06-27): tugas dari schedule/trigger + mr-flow = "high"** (kepentingan owner), sisanya "normal".
  - **Urut: high dulu → nyangkut → paling lama.** (prioritas owner di atas segalanya).

### Deteksi prioritas (jujur — beda keandalan)
- **mr-flow (orkestrator): RELIABLE** — by agent-id (switch `FLOWORK_ORCHESTRATOR`, default mr-flow).
- **schedule/trigger/wakeup: via KONVENSI MARKER LABEL** (`[schedule]`/`[trigger]`/`[wakeup]`/`[cron]` di label run).
  Alasan: `agent_runs` BELUM nyimpen caller (invoke bawa "scheduler"/"wakeup" tapi ga ke-persist). Marker = hook
  forward-compat; di-enforce nanti lewat doktrin TASK-DISIPLIN (agent kasih marker) ATAU follow-up: tambah kolom
  `origin` di agent_runs (agent_run.go NON-frozen) + stamp dari invoke. **BELUM full — jangan over-claim.**
- TIDAK maksa agent polling tabel bersama (hormatin model invoke SINKRON: Mandor yang baca → re-dispatch).
  Catatan agent_run.go: "shared cross-agent registry = coordination store terpisah" — worklog = READ snapshot, BUKAN registry tulis bersama.

## SWITCH (GUI = kebenaran)
- `FLOWORK_WORKLOG` (bool, default ON) — OFF = endpoint balik kosong.
- `FLOWORK_WORKLOG_STALE_MIN` (int, default 60) — menit sebelum task dianggap nyangkut.
- Kategori GUI: "Autonomy / Orchestration" (internal/fwswitch/registry.go, NON-frozen → auto muncul di GUI Settings).

## TOKEN / ANTI-HALU
Nol dampak prompt — `/api/worklog` dipanggil Mandor, BUKAN di-inject tiap turn. Ga nambah token/halu.

## DOKTRIN AOLA-014 KE BRAIN LIVE — jalur edition-independent (penting)
Nambah konstitusi sacred ke brain LIVE ke-blok `editionGate` (router edisi FREE = konstitusi read-only,
anti-rebrand; cuma "corporate" yg unlock). Owner ga mau corporate (boundary). Jalur BENER (bukan bypass):
- **`router/cmd/brain-addconst/` (FROZEN)** — CLI owner-local: manggil `brain.AddConstitution` (jalur INTERNAL
  yg sama dipake seed boot, BUKAN HTTP editionGate). Owner-local = inheren owner-authed. Cabut-akar: pisahin
  "owner nambah doktrin" dari "rebrand". Gate rebrand TETAP utuh. Pakai:
  `go run ./cmd/brain-addconst -section AOLA-XXX -amp 999999 -content-file <f> -db <brainDB>` (flag `-db` wajib
  buat proses standalone — sidecar.BrainDB() ga ke-resolve di luar router). Idempotent (skip kalau section ada).
- AOLA-014 SEEDED juga di `doctrine_seed.json` (fresh-brain) → install baru otomatis dapet.

## FILE
- `agent/feature_worklog.go` (NON-frozen — DEFER freeze, nunggu refactor `collectWorklog`→`internal/worklog`).
- `agent/internal/triggers/type_idle.go` (**FROZEN** + seam `idleLoadReader` var-default buat OS lain).
- `router/cmd/brain-addconst/main.go` (**FROZEN**) — CLI tambah konstitusi live.
- `router/internal/brain/doctrine_seed.json` (FROZEN, re-hash) — seed AOLA-014.
- `agent/internal/fwswitch/registry.go` (NON-frozen, seam switch) — 2 switch worklog.
- Baca: `agent/internal/tools/builtins/agent_run.go` (sumber data `agent_runs`).

## VERIFIKASI (2026-06-27)
`GOWORK=off go build ./...` OK · `go vet` OK · `TestKernelFreeze` PASS (frozen utuh) ·
delete-test (pindah feature_worklog.go+_test → build tetap OK = additif, core self-sufficient) ·
**unit-test `TestCollectWorklog`+`TestWorklogPriorityMarker` PASS** (prioritas+nyangkut+filter+sort,
DB in-memory) · live: rebuild+restart, `GET /api/worklog` → HTTP 401 (route ke-wire + ke-auth, bukan 404).
Data-test penuh via GUI (butuh sesi owner) atau pas MANDOR konsumsi internal.

## STATUS FREEZE (2026-06-27) — SEMUA FROZEN (Mandor operasional + teruji e2e)
✅ Hash di `KERNEL_FREEZE.md`, `chattr +i`, TestKernelFreeze PASS, append ditolak:
- `internal/worklog/worklog.go` · `internal/tools/builtins/worklog_tool.go` · `feature_worklog.go`
- `mandor_seed.go` · `feature_mandor.go` · `internal/triggers/type_idle.go` (+seam `idleLoadReader`)
- `../router/cmd/brain-addconst/main.go` · `../router/internal/brain/doctrine_seed.json` (re-hash)

## MANDOR — OPERASIONAL ✅ (validasi LIVE 2026-06-27)
Switch `FLOWORK_MANDOR=true` → boot: seed agent `mandor.fwagent` + subscribe tool `worklog`+`agent_command`
+ seed rule trigger `idle-mandor` (type idle, target mandor, enabled). Bukti e2e:
- agent kebentuk, 2 tool ke-subscribe, rule di flowork.db (enabled=1).
- invoke mandor (Rule 9) → log `[mandor] tool_call: worklog args={}` (BENERAN panggil tool, bukan halu) →
  papan kosong → "aman, ga ada nyangkut" (patuh aturan "papan kosong = diem").
- Alur penuh: idle (load<60% + cooldown 30mnt) → engine invoke mandor → baca worklog → high/stale →
  `agent_command` re-dispatch ke agent pemilik. REM: idle + cooldown + "papan kosong → diem".

REM lanjutan (refinement, belum): deteksi owner-aktif eksplisit + token-budget gate (lihat roadmap).

## SENSOR REFLEX (host-side poller, no token) — saudara Mandor
Prinsip: pas PC berat JANGAN invoke LLM (nambah beban) → reflex = poller HOST murni (baca /proc/loadavg
atau agent_runs), notify owner Telegram, cooldown anti-spam. Semua FROZEN + teruji + delete-test.
- **`feature_busy.go`** (reflex beban-TINGGI, P0-A): load>`FLOWORK_BUSY_PCT`(90) → "PC berat, mau jeda/standby?"
  TAWARI + kasih kesadaran, **JANGAN auto-matiin** (owner yang putusin). Switch `FLOWORK_BUSY_ALERT`. Test `TestBusyShouldAlert`.
- **`feature_deadair.go`** (dead-air, P0-C): ada tugas AKTIF tapi semua beku >`FLOWORK_DEADAIR_MIN`(60) → anomali
  (token/provider/error) → alert owner. Idle tanpa tugas = normal. Reuse `worklog.Collect`. Test `TestDeadairDecide`.
- **`type_idle.go`** (FROZEN, trigger): load<60% → fire rule `idle-mandor` (wake Mandor). Beban-RENDAH = produktif.
  → Tiga-tiganya = orkestrator sadar-beban: tinggi→tawari, rendah→Mandor, beku→alarm.

## KEABADIAN — AUTO-START SAAT BOOT (ide #2)
Owner: *"pas PC nyala dia HARUS auto nyala."* Docktor (`os/selfheal/watchdog.sh`) = systemd-user unit
Restart=always. TAPI systemd-user cuma nyala pas LOGIN — kecuali **lingering** ON. Fix (2026-06-27):
`loginctl enable-linger` → `Linger=yes` → docktor + stack nyala pas BOOT (pre-login). Di-permanenin di
`install-watchdog.sh` (idempotent, best-effort). install-watchdog.sh + watchdog.sh SENGAJA non-chattr
(operasional, ke-overwrite auto-update; chattr+i bakal jebol update). Self-restart on-demand = deferred.

## GUI PANEL "Otonomi" (surface)
`agent/web/tabs/otonomi.js` + nav button (index.html) + `ACTIVE_TABS` (app.js) — semua NON-frozen (GUI seam).
Tab read-only: render `/api/worklog` (papan, prioritas, nyangkut) + `/api/journal` (pelajaran/eureka/antibody/skill/insting).
Fetch same-origin (cookie auth). Web di-`go:embed` → rebuild biar ke-serve. Owner liat Mandor mantau apa + koloni belajar apa.

## COST-OF-THOUGHT + PRE-FLIGHT (data-nyata, anti-halu)
- **`internal/tools/builtins/preflight_tool.go`** (FROZEN): tool `preflight` → {load_pct, busy, urgency, advice}.
  DATA NYATA (/proc/loadavg) buat putusin effort sebelum task berat — BUKAN ramalan (anti-halu Rule 1).
  Cover roadmap Shadow-P3 + Cost-of-thought meter. Switch `FLOWORK_URGENCY` (hemat/normal/deadly). Test `TestPreflightAdvice`.
- **Model dinamis per-agent + fallback = VERIFIED LIVE (2026-06-27):** tiap agent punya `router_model` (GUI, kv).
  Model gagal/ga-ada → router `dispatcher.go` auto-fallback ([model]→combo→global, log "unavailable→trying next").
  Bukti: POST model bogus → balik jawaban dari `flowork-brain.gguf` (fallback). Cost-of-thought model-tier = ke-handle existing.

## MANDOR — progres (P0-B)
- [x] Papan kerja READ `/api/worklog` + collectWorklog + prioritas (high=schedule/trigger/mr-flow).
- [x] Doktrin **AOLA-014 TASK-DISIPLIN** — seed `doctrine_seed.json` + LIVE brain (id 846, 14 entri) + verified (mr-flow nyebut by-name).
- [x] **Trigger tipe `idle`** (`internal/triggers/type_idle.go`, **FROZEN**, seam Register + `idleLoadReader`): PC load < ambang (default 60%) + cooldown → fire event. Fail-safe (OS non-linux/error → ga fire). Test PASS.
- [x] **`internal/worklog`** (pkg): logic agregasi dipisah dari transport → share feature HTTP + builtin tool (anti-duplikat). Test `TestCollect`+`TestPriorityOf` PASS.
- [x] **Tool `worklog`** (`internal/tools/builtins/worklog_tool.go`, NON-frozen): MANDOR baca papan dari DALAM agent (tanpa auth) via hook `WorklogScanHook` (di-wire feature_worklog.go, pegang Host).
- [x] **Skeleton agent MANDOR** (`mandor_seed.go` + `feature_mandor.go`, NON-frozen): seed via `seedUtilityAgent` (persona supervisor, model haiku GUI-overridable). GATED switch `FLOWORK_MANDOR` (default OFF) → ga dibikin dormant + ke-reap sebelum operasional.
- [ ] **Operasional MANDOR** (nyusul): subscribe tool `worklog` + agent-invoke ke mandor · rule trigger `idle→mandor` (DATA) · reconcile + rem (ada kerja/token/owner-ga-aktif/cooldown) · rebuild+restart+validasi · baru flip `FLOWORK_MANDOR=on`.
- [x] **F-C WAKE-PUSH mandor by-EVENT (2026-07-02, verified live)** — `feature_wake_mandor.go`
  (sibling non-frozen, deletable; delete-test PASS). Worker selesai via tool `agent_command`
  → mandor langsung dibangunin: wrap seam `builtins.InvokeAgentFunc` (var Pola B di
  `agent_command.go` FROZEN — NOL unlock) pas PhaseSeed → sukses → `Engine.RunNow` semua rule
  `worklog-pending` enabled. Guard: debounce 60s (klaim slot CUMA kalau ada rule layak),
  anti self-loop (rule target == worker di-skip). Switch GUI `FLOWORK_WAKE_MANDOR` (default ON).
  Seed rule `worklog-mandor` (type worklog-pending, target mandor, idempotent, gate FLOWORK_MANDOR);
  rule live di-insert 2026-07-02. Poll `worklog-pending` TETAP jalan = jaring pengaman task nyangkut.
  Bukti: worker `app-judge` kelar → log `wake-mandor: … RunNow rule worklog-mandor (run 183)` →
  mandor rekonsiliasi papan (status ok). Unit test debounce+filter PASS.
- [ ] Re-dispatch nyangkut: wire `internal/zombie/detector.go`.
- [ ] (opsional) panel VISUAL papan kerja di frontend live.
- [ ] Freeze cluster worklog (feature/tool/pkg/seed) SETELAH Mandor operasional + teruji.

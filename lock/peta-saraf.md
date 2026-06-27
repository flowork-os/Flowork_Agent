# 🧠 PETA-SARAF — Daftar Saklar yang AI Boleh Pakai buat Tumbuh

> Inti Flowork = TUBUH (beku). Saraf = tempat perubahan boleh ngalir. AI tumbuh CUMA lewat 3 saluran:
> **SWITCH** (env `FLOWORK_*`, default aman) · **DATA** (brain/mesh DB) · **MODUL** (`.fwpack` via AI Studio).
> Dok ini = peta saluran SWITCH (env + registry POLA-A). AI baca ini buat tau "tombol apa aja yang gue punya".
> Default tiap saklar dipilih AMAN: kosong/off/registry-kosong → inti jalan apa adanya.

> **MEKANISME (F2 — papan-colokan saraf, POLA A):** dok ini punya kembaran mesin di kode —
> `agent/nerve_registry.go` (papan beku, `RegisterNerve`/`Nerves()`/`NerveChannels()`, default kosong=aman)
> + `agent/nerve_seed_ext.go` (SIBLING deletable, isi katalog di-generate dari dok ini = SATU sumber).
> AI/GUI tanya **`GET /api/evolve/nerves`** → balik `{channels, count, by_kind, nerves[]}`. Update saraf:
> edit dok ini → regen tabel di `nerve_seed_ext.go`. Hapus sibling → papan kosong → inti tetep jalan.

> **KUNCI PENGUSUL (F3):** `agent/nerve_proposal_ext.go` (`NerveProposalVet`) klasifikasi usulan evolusi →
> saluran saraf: `add-skill→data` · `add-agent/add-app→modul` · `set-switch→switch` (target wajib saraf
> terdaftar) · `fix/refactor/doc/test→core-edit = DITOLAK`. Di-wire di guard `selfevolve_coreapply.go`:
> usulan di luar ruang saraf → blocked + arahin "pakai switch/data/modul / lapor butuh_tombol". Inti beku
> ga pernah disentuh. (Evolusi mode default OFF → guard ini jaga buat pas AUTO dinyalain.)

> **LAPOR BUTUH_TOMBOL (F4):** pas guard F3 nolak (AI mentok), `recordButuhTombol()`
> (`agent/nerve_butuh_tombol_ext.go`) catat `{lokasi, alasan, kind, channel}` ke antrian owner (KV
> `evolve_butuh_tombol`, dedupe + cap 200). Endpoint **`GET/DELETE /api/evolve/butuh-tombol`**. GUI:
> panel di tab Evolusi (`web/tabs/evolution.js`, i18n en+id) — owner baca → nambah saklar (jarang, sadar)
> → kosongin antrian. AI mentok = LAPOR, BUKAN bongkar inti.

---

## A. SAKLAR ENV (`FLOWORK_*`) — ±131, per domain

Presedensi: **GUI menang → file `~/.flowork/flowork_settings.json` overwrite ENV → default kode** (`fwswitch`).
Tipe: bool(on/off) · int · float · str · path · csv · json.

### Brain / Instinct
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_INSTINCT_INJECT | Suntik insting relevan ke prompt | on | bool |
| FLOWORK_INSTINCT_INJECT_MAX | Cap jumlah insting disuntik | 3 | int |
| FLOWORK_INSTINCT_SCOPED | Agent cuma dapat insting domain-nya | off | bool |
| FLOWORK_INSTINCT_SEMANTIC | Pilih insting by-makna (vektor) vs overlap | on | bool |
| FLOWORK_INSTINCT_SCOPE_MAP | Peta domain per-agent | — | json |
| FLOWORK_BRAIN_EXTERNAL_SCOPE | Caller eksternal ga dapat insting-tool | off | bool |

### Brain / Graph
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_CGM_CODEMAP | Proyeksi codemap ke graph agent | on | bool |
| FLOWORK_CGM_ORPHAN_BACKFILL | Link node orphan ke hub brain | on | bool |
| FLOWORK_CGM_DEADLETTER | Proyeksi task gagal ke graph | on | bool |
| FLOWORK_CGM_AUTODIGEST | Deep digest percakapan ke graph | off | int |
| FLOWORK_CGM_NODE_LIMIT | Batas node graph sebelum prune | 3000 | int |
| FLOWORK_CGM_EDGE_LIMIT | Batas edge graph sebelum prune | 6000 | int |
| FLOWORK_DREAMGRAPH_AUTOSYNC | Auto-populate Knowledge Graph router | on | bool |
| FLOWORK_DREAMGRAPH_SYNC_MIN | Interval refresh DreamGraph (menit) | 5 | int |
| FLOWORK_DREAMGRAPH_INSTINCTS | Proyeksi instinct ke DreamGraph | on | bool |
| FLOWORK_DREAMGRAPH_KNOWLEDGE | Proyeksi korpus knowledge jadi hub | on | bool |
| FLOWORK_DREAMGRAPH_MESH | Proyeksi pengetahuan mesh ke graph | on | bool |

### Brain / Search & Codemap
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_SEARCH_MINSCORE | Lantai relevansi cosine search | 0.45 | float |
| FLOWORK_BINARY_VECTOR | Binary-vector recall (jutaan drawer) | auto | str |
| FLOWORK_BINARY_VECTOR_MIN | Drawer minimum auto-aktif | 1000000 | int |
| FLOWORK_FRESH_MEMTYPES | Override daftar mem_type default | — | csv |
| FLOWORK_CODEMAP_AUTOENRICH | Auto-enrich codemap tiap interval | on | bool |
| FLOWORK_CODEMAP_AUTOENRICH_MIN | Interval auto-enrich (menit) | 30 | int |
| FLOWORK_CODEMAP_CANONICAL_AGENT | Agent referensi tool codemap_files | — | str |
| FLOWORK_CODEMAP_ROOT | Root path codemap scanning | — | path |

### Router / Tools & Capability
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_DEFER_TOOLS | Skema tool on-demand (hemat prompt) | off | bool |
| FLOWORK_EXPOSE_ALL_TOOLS | Semua agent akses semua tool | off | bool |
| FLOWORK_MAX_EXPOSED_TOOLS | Batas tool per-agent | ∞ | int |
| FLOWORK_DYNAMIC_TOOLS | Intent-gated tools (prune schema) | off | bool |
| FLOWORK_DYNAMIC_TOOLS_TOPK | Max tool relevan dikirim | 12 | int |
| FLOWORK_DYNAMIC_TOOLS_MINSCORE | Cosine min tool dianggap relevan | 0.30 | float |
| FLOWORK_TOOLCALL_RECOVER | Parse `<tool_call>` bocor model lokal | on | bool |
| FLOWORK_TOOL_GC_OFF | Matiin GC tools otomatis | off | bool |
| FLOWORK_TOOL_GC_MAXERR | Error threshold prune tool | 5 | int |
| FLOWORK_TOOL_GC_IDLE_DAYS | Hari idle sebelum prune tool | 90 | int |

### Router / LLM & Engine
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_LLM_IDLE_SLEEP | Idle-sleep LLM lokal | on | bool |
| FLOWORK_LLM_IDLE_SLEEP_SEC | Detik sebelum idle-sleep (min 10) | 60 | int |
| FLOWORK_LLM_IDLE_DEBUG | Debug log idle-sleep | off | bool |
| FLOWORK_LLM_MODEL | Model LLM default | — | str |
| FLOWORK_CODER_MODEL | Model coder/codegen | — | str |
| FLOWORK_REASONING | Mode reasoning llama-cpp | — | str |
| FLOWORK_CTX | Context window llama-cpp | — | int |
| FLOWORK_NGL | GPU layers llama-cpp | — | int |
| FLOWORK_CPU_MOE | CPU mixture-of-experts | off | bool |
| FLOWORK_KV_TYPE | KV cache type llama-cpp | — | str |
| FLOWORK_BRAIN_GGUF | Path model GGUF brain | auto | path |
| FLOWORK_BRAIN_VINDEX | Path vector index brain | auto | path |
| FLOWORK_LLAMA_BIN | Path binary llama-cpp-server | auto | path |
| FLOWORK_LOCALAI_AUTOSTART | Auto-start LocalAI saat boot | off | bool |
| FLOWORK_LOCAL_EMBED_URL | URL local embedding service | — | url |
| FLOWORK_LOCAL_EMBED_MODEL | Model local embedding | — | str |

### Router / KV-Cache, Context, Resilience
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_CACHE_REUSE | KV cache-reuse prefix statik | 0 | int |
| FLOWORK_PARALLEL_SLOTS | Parallel slots server (-np N) | 0 | int |
| FLOWORK_SLOT_SAVE_PATH | Persist KV slot ke disk | — | path |
| FLOWORK_SYS_STATUS | Sisipin kondisi PC ke prompt | on | bool |
| FLOWORK_RL_MAX_RETRY | Retry saat 429 sebelum fallback | 6 | int |
| FLOWORK_ROUTER_RETRY | Retry router transient backoff | off | bool |
| FLOWORK_RESILIENCE_OFF | Matiin agent resilience | off | bool |

### Mesh / Security
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_INTEGRITY_GATE | Tolak belajar mesh kalau frozen-core berubah | on | bool |
| FLOWORK_MESH_SHARE | Share & terima pengetahuan mesh | on | bool |
| FLOWORK_MESH_APPROVE | Mode approve masuk: manual / auto | manual | str |
| FLOWORK_KERNEL_MANIFEST | Path manifest kernel (integrity gate) | auto | path |

### Cloaking
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_CLOAK_SUFFIX | Suffix nama tool saat cloaking | _cc | str |
| FLOWORK_CLOAK_VERSION | Versi klien disamarkan | 2.1.92 | str |
| FLOWORK_CLOAK_DECOYS | Daftar tool decoy | — | csv |

### Autonomy / Orchestration
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_ORCHESTRATOR | Agent orkestrator default | mr-flow | str |
| FLOWORK_WORKLOG | Papan kerja bersama lintas-agent | on | bool |
| FLOWORK_WORKLOG_STALE_MIN | Ambang task nyangkut (menit) | 60 | int |
| FLOWORK_MANDOR | Agent Mandor (supervisor idle) | off | bool |
| FLOWORK_DEADAIR | Deteksi anomali semua-beku | on | bool |
| FLOWORK_DEADAIR_MIN | Ambang diem deadair (menit) | 60 | int |
| FLOWORK_BUSY_ALERT | Reflex beban-tinggi (notif owner) | on | bool |
| FLOWORK_BUSY_PCT | Threshold load CPU (%) busy alert | 90 | int |
| FLOWORK_GUARDIAN_AUTO | Guardian arm otomatis saat boot | on | bool |
| FLOWORK_GUARDIAN_INTERVAL_SEC | Interval guardian sentinel (detik) | — | int |
| FLOWORK_JOURNAL | Jurnal pengalaman | on | bool |
| FLOWORK_URGENCY | Mode urgensi cost-of-thought | normal | str |
| FLOWORK_FANOUT_BUDGET | Budget paralel task fanout | — | int |
| FLOWORK_REAP_ERRRATE | Error-rate ambang vonis Reaper (0..1) | 0.40 | float |
| FLOWORK_REAP_MIN_SAMPLES | Min run selesai sebelum Reaper vonis | 5 | int |

### Security / Scanner & Power
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_SCANNER_AUTOSCAN | Auto-scan kode saat berubah | on | bool |
| FLOWORK_POWER_ARMED | Enable power control (shutdown/reboot) | off | bool |
| FLOWORK_POWER_REQUIRE_APPROVAL | Wajib approval sebelum power action | on | bool |

### Agent Runtime & Identity
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_AGENT_ID | ID agent (inject host) | manifest | str |
| FLOWORK_AGENT_CONFIG | JSON config agent dari GUI | store | json |
| FLOWORK_AGENT_DB | Path state.db agent | /workspace/state.db | path |
| FLOWORK_AGENT_WORKSPACE | Path workspace agent | /workspace | path |
| FLOWORK_SHARED_WORKSPACE | Path workspace bersama | /shared | path |
| FLOWORK_PRIVILEGED_AGENTS | Daftar agent ID privileged | — | csv |
| FLOWORK_MCP_AGENT | Agent untuk MCP server | — | str |
| FLOWORK_SELF_HANDLE_PHRASES | Custom phrase self-response | — | csv |

### Browser / Desktop
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_BROWSER_URL | URL Chrome remote-debug | — | url |
| FLOWORK_CHROME_BIN | Path binary Chrome | auto | path |
| FLOWORK_BROWSER_HEADLESS | Browser headless | on | bool |
| FLOWORK_BROWSER_IDLE_MIN | Idle timeout browser (menit) | 15 | int |
| FLOWORK_BROWSER_FLAGS | Flag browser tambahan | — | csv |

### Telegram
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_TG_BOT_TOKEN | Bot token Telegram | — | str |
| FLOWORK_TG_FORMAT | Format pesan: html / plain | html | str |
| FLOWORK_TG_CHUNK | Chunking pesan | on | bool |
| FLOWORK_TG_MEDIA | Enable media (on-demand) | on | bool |
| FLOWORK_TG_ALLOWED_CHATS | Chat ID diizinkan | — | csv |

### Skills, Self-Evolution & Learning
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_SKILL_AUTOSYNC | Auto-sync skill dari Catalog | on | bool |
| FLOWORK_SKILL_AUTOSYNC_MIN | Interval auto-sync skill (menit) | 30 | int |
| FLOWORK_SKILL_REGISTRY | Repo skill registry | flowork-os/flowork-skills | str |
| FLOWORK_EDITION | Edisi: dev (evolusi penuh) / public | public | str |
| FLOWORK_LEGACY_DREAM | Legacy dream cycle (deprecated) | off | bool |
| FLOWORK_GROUP_SLASH | Group slash commands | off | bool |

### Path / Env / Discovery / Auth
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_DATA_DIR | Data directory portable | ~/.flowork | path |
| FLOWORK_HOME | Home directory guardian vault | ~ | path |
| FLOWORK_PROJECT_ROOT | Root project | cwd | path |
| FLOWORK_AGENTS_DIR | Folder agents | ~/.flowork/agents | path |
| FLOWORK_TOOLS_DIR | Folder sidecar-tools | auto | path |
| FLOWORK_ENGINE_DIR | Folder engine | auto | path |
| FLOWORK_CODESCAN_ROOT | Root codemap scanning | PROJECT_ROOT | path |
| FLOWORK_SIDECAR | Data home sidecar | ~/.flowork | path |
| FLOWORK_OS | Penanda OS host | auto | str |
| FLOWORK_SELF_URL | Self-URL MCP & TUI | — | url |
| FLOWORK_PUBLIC_URL | Public URL sistem | — | url |
| FLOWORK_SCHEME | Scheme HTTP (https/http) | http | str |
| FLOWORK_TRUST_PROXY | Trust X-Forwarded-* headers | off | bool |
| FLOWORK_ROUTER_URL | Router URL routerclient | settings | url |
| FLOWORK_BASE | Base URL flowork-gui (CLI) | 127.0.0.1:1987 | url |
| FLOWORK_LOOPBACK_SECRET | Secret loopback intra-process | random | str |
| FLOWORK_GITHUB_TOKEN | GitHub token API | — | str |
| FLOWORK_CONNECT_CONFIG | Config flowork-connect CLI | — | json |

### Timezone
| Saklar | Kontrol | Default | Tipe |
|---|---|---|---|
| FLOWORK_TZ_OFFSET_HOURS | Offset jam zona waktu | 7 | int |
| FLOWORK_TZ_LABEL | Label zona waktu | WIB | str |

> Test-only (bukan saklar produksi): FLOWORK_TEST_DG_X, FLOWORK_TEST_MESH_X, FLOWORK_TEST_PROJ_X,
> FLOWORK_X, FLOWORK_CGM_XXX, FLOWORK_CLOAK_ (prefix). FLOWORK_LIVE_DIGEST & FLOWORK_LIVE_* = internal.

---

## B. REGISTRY (POLA-A papan-colokan) — ±41 seam

Pola: registry DIBEKUIN (default KOSONG = aman); item baru dicolok lewat file SIBLING `_ext.go`
(`init(){ Register...() }`) yang BISA dihapus tanpa bikin build patah. Nambah kemampuan via colokan ini.

| Registry | Yang dicolok | Lokasi definisi |
|---|---|---|
| RegisterDetector | Runtime adopt custom (Python/Node/Go/Rust+) | agent/internal/apps/adopt/detect.go |
| RegisterScanRule | Pola berbahaya custom scanner | agent/internal/apps/adopt/scan.go |
| RegisterCapabilityVerifier | Pemeriksa jenis kapabilitas baru (AI Studio gate) | agent/studio_gate.go |
| RegisterFeature | Fitur self-contained per fase (Wire/Route/Seed) | agent/feature_registry.go |
| RegisterGraphProjection | Sumber proyeksi ke Cognitive Graph | agent/graph_autosync_ext.go + router |
| RegisterDeathObserver | Reaksi pada kematian kemampuan (DeathLetter) | agent/deathletter.go |
| RegisterLLMLifecycleObserver | Callback wake/sleep LLM idle | router/llm_idle_sleep.go |
| RegisterDynamic | Tool plugin runtime register/update | agent/internal/tools/dynamic.go |
| RegisterInterceptor | Interceptor pre-execution tool | agent/internal/tools/interceptors.go |
| RegisterHook | Slash hook (before/after) | agent/internal/slashcmd/hooks.go |
| RegisterExecStrategy | Strategi eksekusi crew (parallel/debate) | agent/internal/taskflow/taskflow_ext.go |
| RegisterSkillProvider | Provider skill ([]SkillDoc) | router/internal/brain/skill_provider.go |
| RegisterInstinctSelector | Selector insting custom | router/internal/router/instinctenrich.go |
| RegisterInstinctSelectorCtx | Selector insting ctx-aware (scoped) | router/internal/router/instinctenrich.go |
| RegisterDeliverer | Pengirim balasan trigger (telegram/chat) | agent/internal/triggers/deliver.go |
| RegisterGroupSyncHook | Hook sinkronisasi roster group | agent/internal/groupsapi/groupsapi_ext.go |
| RegisterPrimitive | Primitive capability kernel (browser/custom) | agent/internal/kernel/loader/manifest.go |
| RegisterBuiltins | Provider builtin kernel (store/time/fs/exec/…) | agent/internal/loket/providers.go |
| RegisterDeferPolicy | Kebijakan defer/expose-tool per-agent | agent/internal/agentmgr/tool_specs_defer.go |
| RegisterMeshFilter | Filter paket mesh (reject/pass/quarantine) | router/internal/mesh/filter_ext.go |
| RegisterMigration | Migration SQL (additive) | router/internal/store/migrate.go |
| RegisterProxyDeployTarget | Target deploy proxy | router/proxy_deploy_registry.go |
| RegisterTunnelProvider | Provider tunnel (ngrok/Cloudflare/SSH) | router/tunnel_registry.go |
| RegisterExtraRoute | Route HTTP tambahan | router/routes_ext.go |
| RegisterCLITool / RegisterCustomCLITool | CLI tool extra (code + DB-driven) | router/internal/clitools/ |
| RegisterToolcallExtractor | Extractor format tool-call muntah | router/internal/router/toolcall_recover_ext.go |
| (provider) Register × LLM/embedding/image/stt/tts/search/fetch/quota/mcp/translator/rtk | Provider plug per-jenis | router/internal/{executors,providers,…} |
| (tools/slashcmd/triggers) Register | Builtin tool / slash cmd / jenis trigger | agent/internal/{tools,slashcmd,triggers} |
| FanoutStrategy (var, POLA-B) | Strategi fan-out broadcast bus | agent/internal/loket/providers.go |

> Semua registry: default kosong → inti fallback/no-op/pass-through = AMAN. Hapus sibling `_ext.go` →
> build tetap OK (delete-test). Ini realisasi "nambah fitur tanpa buka file beku".

---

## C. AUDIT PERILAKU YATIM (perlu saklar SEBELUM kunci-total F5)

"Yatim" = kebijakan inti yang KELAK perlu disetel tapi sekarang HARDCODE tanpa saklar. Kalau dibekukan apa
adanya → AI/owner mentok → tergoda bongkar inti. Status: `beku` = udah di KERNEL_FREEZE (pasang saklar =
unfreeze sadar, §6.3) · `belum-beku` = boleh pasang saklar SEKARANG (low-risk, waktu yang tepat).

| Perilaku | Lokasi | Hardcode | Saran saklar | Prioritas | Status |
|---|---|---|---|---|---|
| Reaper error-rate + min-sampel auto-vonis | agent/reaper.go | 0.40 / 5 | FLOWORK_REAP_ERRRATE / _MIN_SAMPLES | tinggi | ✅ TERPASANG |
| Karma gate auto-commit core (self-evolution) | agent/internal/agentmgr/selfevolve.go:40 | EvolveKarmaThreshold=20 | FLOWORK_EVOLVE_KARMA | tinggi | beku (LOCKED-soft) |
| Anti-flailing window/dup/nudge | agent/agents/mr-flow/flail_guard.go:31-33 | 8/3/4 | FLOWORK_FLAIL_WINDOW/_DUPMAX/_NUDGES | tinggi | beku (FROZEN hdr) |
| Promote recurring-mistake jadi instinct | agent/mistake_promote_job.go:18 | promoteMinHit=3 | FLOWORK_PROMOTE_MIN_HIT | sedang | beku (FROZEN hdr) |
| Cold-archive retensi/threshold memori | agent/cognitive_archive_job.go:17-19 | 90hari / 50000 | FLOWORK_ARCHIVE_DAYS / _NODE_THRESHOLD | rendah | beku (FROZEN hdr) |
| Cap timeout bus.request loket (pernah di-tune tangan 120→240) | agent/internal/loket/service.go:218 | 240s | FLOWORK_LOKET_CALL_TIMEOUT | tinggi | beku |
| Mesh consensus quorum promote-brain | router/internal/mesh/consensus_phase3.go:26 | consensusN=2 | FLOWORK_MESH_CONSENSUS_N | tinggi | beku |
| Trusted-peer fast-path karma | router/internal/mesh/consensus_phase3.go:29 | 0.8 | FLOWORK_MESH_TRUSTED_FASTPATH_KARMA | tinggi | beku |
| Peer karma trust floor (auto-block) | router/internal/mesh/karma_gate.go:18 | KarmaFloor=0.2 | FLOWORK_MESH_KARMA_FLOOR | tinggi | beku |
| Embedding model selalu bge-m3 (no-hardcode!) | agent/internal/routerclient/embed.go:16 | "bge-m3" | FLOWORK_EMBED_MODEL | tinggi | beku |
| Login lockout policy (brute-force) | router/login_limiter.go:44 | 5 / 60m | FLOWORK_LOGIN_MAX_FAILS / _WINDOW | tinggi | beku |
| HTTP timeout default router client | agent/internal/routerclient/routerclient.go:22 | 30s | FLOWORK_ROUTER_TIMEOUT | tinggi | beku |
| Entity-resolution dedup threshold (graph) | agent/internal/agentdb/cognitive_resolve.go:8 | 0.86 | FLOWORK_RESOLVE_MINSCORE | tinggi | beku |
| Mesh dedup/endorsement similarity | router/internal/mesh/similarity.go:25 | 0.82 | FLOWORK_MESH_SIM_THRESHOLD | tinggi | beku |
| Skillpack publish-gate quality | router/internal/skillpack/karma.go:26-27 | 3 / 0.6 | FLOWORK_PUBLISH_MIN_USES / _POSITIVE | tinggi | beku |
| Max background-task concurrency | agent/task_worker.go:32 | 2 | FLOWORK_MAX_CONCURRENT_TASKS | sedang | beku |
| Scheduler per-job exec timeout | agent/internal/scheduler/engine.go:198 | 90s | FLOWORK_SCHED_EXEC_TIMEOUT_SEC | sedang | beku |
| Trigger action invocation timeout | agent/internal/triggers/engine.go:108 | 300s | FLOWORK_TRIGGER_TIMEOUT_SEC | sedang | beku |
| Upstream LLM request timeout | router/internal/router/dispatcher.go:27 | 300s | FLOWORK_ROUTER_HTTP_TIMEOUT | sedang | beku |
| Stream stall timeout | router/internal/streamutil/stall_reader.go:16 | 35s | FLOWORK_STREAM_STALL_TIMEOUT | sedang | beku |
| Quota fallback backoff/cooldown | router/internal/services/account_fallback.go:13-23 | 1s/4m/8/30s | FLOWORK_FALLBACK_BACKOFF_MAX / _COOLDOWN | sedang | beku |
| Auth session TTL | router/internal/store/authsessions.go:28 | 7hari | FLOWORK_SESSION_TTL | sedang | beku |
| Model alias table MITM (no-hardcode!) | router/internal/mitm/config.go:29-51 | nama model di-bake | RegisterModelAlias / FLOWORK_MITM_MODEL_MAP | sedang | beku |
| Mesh packet hop limit | router/internal/mesh/packet.go:44 | HopMax=7 | FLOWORK_MESH_HOP_MAX | rendah | beku |
| Retention soft/hard delete windows | agent/internal/agentdb/retention.go:23-27 | 30/90/90/180/90 | FLOWORK_RETAIN_*_DAYS | rendah | beku |

### Aturan tindak-lanjut audit
- **Temuan kunci:** mayoritas yatim ada di file yang SUDAH terproteksi (hash-lock atau header FROZEN/LOCKED).
  Cuma `reaper.go` (header bersih) yang aman dipasang otonom → **udah dikerjain** (2 saklar, build/vet/freeze PASS).
- **Sisa ±24 perilaku (beku):** pasang saklar = keputusan SADAR/izin owner (§6.3: unfreeze → seam → re-hash →
  re-freeze). Ini batch owner-gated sebelum F5. Urut prioritas: knob keamanan mesh (consensus/karma/sim) +
  no-hardcode (embed model `bge-m3`, model-alias MITM) + timeout yang pernah bikin mentok (loket/router) dulu.
- Sesudah saklar terpasang → perilaku pindah ke tabel A/B + dicoret dari sini. **NOL yatim** (gerbang F1 lulus)
  butuh batch owner-gated itu kelar. Sampai itu: F1 = peta lengkap + audit jelas; eksekusi saklar-beku nunggu owner.

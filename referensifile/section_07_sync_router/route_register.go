// route_register.go — HTTP route table for flowork-gui.
package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/teetah2402/flowork/internal/browser"
	"github.com/teetah2402/flowork/internal/guiapi"
	"github.com/teetah2402/flowork/internal/localai"
)

// registerRoutes wires all HTTP handlers to a new ServeMux and returns it.
func registerRoutes(ws string, browserMgr *browser.Manager, aiRuntime localai.Runtime) *http.ServeMux {
	mux := http.NewServeMux()

	// ── API endpoints ──────────────────────────────────────────────────
	// Public liveness probe — no auth (whitelisted di isPublicPath).
	mux.HandleFunc("/api/system/health", guiapi.SystemHealthHandler())
	// 2026-05-21 Mr.Dev mandate 7-tab dashboard: proxy ke kernel + daemon status.
	mux.HandleFunc("/api/kernel/proxy", guiapi.KernelProxyHandler())
	mux.HandleFunc("/api/daemons/status", guiapi.DaemonStatusHandler())
	mux.HandleFunc("/api/mesh/stream", guiapi.MeshStreamHandler()) // M10: real-time SSE
	mux.HandleFunc("/api/bridge", guiapi.BridgeHandler(ws))
	mux.HandleFunc("/api/bridge/stream", guiapi.BridgeSSEHandler(ws))
	mux.HandleFunc("/api/bridge/send", guiapi.BridgeSendHandler(ws, aiRuntime)) // POST — chat dari Ayah
	mux.HandleFunc("/api/agents", guiapi.AgentsHandler(ws))
	mux.HandleFunc("/api/silent", guiapi.SilentHandler())
	mux.HandleFunc("/api/commits", guiapi.CommitsHandler(ws))
	mux.HandleFunc("/api/claims", guiapi.ClaimsHandler(ws))
	mux.HandleFunc("/api/claims/move", guiapi.ClaimMoveHandler(ws))                // rc102 F-4 drag-drop
	mux.HandleFunc("/api/claims/new", guiapi.ClaimNewHandler(ws))                  // 2026-05-06: AI/GUI buat new task claim
	mux.HandleFunc("/api/scanner/run-now", guiapi.ScannerRunHandler(ws))           // rc125 on-demand SGVP arsenal
	mux.HandleFunc("/api/arsenal", guiapi.ArsenalHandler(ws))                      // rc145 Arsenal tab KPI
	mux.HandleFunc("/api/arsenal/report", guiapi.ArsenalReportHandler(ws))         // report content from state/scanner-reports/
	mux.HandleFunc("/api/features/status", guiapi.FeaturesHandler(ws))             // rc185 feature grid home dashboard
	mux.HandleFunc("/api/dashboard/overview", guiapi.DashboardOverviewHandler(ws)) // rc186 TokenTracker-style home
	// Protector — dynamic file/folder protection management
	mux.HandleFunc("/api/protector", guiapi.ProtectorListHandler(ws))
	mux.HandleFunc("/api/protector/add", guiapi.ProtectorAddHandler(ws))
	mux.HandleFunc("/api/protector/remove", guiapi.ProtectorRemoveHandler(ws))
	mux.HandleFunc("/api/protector/toggle", guiapi.ProtectorToggleHandler(ws))
	mux.HandleFunc("/api/protector/test", guiapi.ProtectorTestHandler(ws))
	// rc103 Bug Tracker + Tutorial Library (Ayah request 2026-04-19)
	mux.HandleFunc("/api/bugs", guiapi.BugsHandler(ws))
	mux.HandleFunc("/api/bugs/fix", guiapi.BugFixHandler(ws))
	mux.HandleFunc("/api/bugs/ignore", guiapi.BugIgnoreHandler(ws))
	mux.HandleFunc("/api/bugs/reopen", guiapi.BugReopenHandler(ws))
	mux.HandleFunc("/api/bugs/new", guiapi.BugNewHandler(ws))
	mux.HandleFunc("/api/bugs/doc", guiapi.BugDocContentHandler(ws)) // rc105 plug-and-play MD scan
	// rc105 Generic plug-and-play docs folder scanner — auto-detect folder vs single-file.
	mux.HandleFunc("/api/docs", guiapi.DocsCategoryHandler(ws))
	mux.HandleFunc("/api/docs/content", guiapi.DocsCategoryContentHandler(ws))
	mux.HandleFunc("/api/tutorials", guiapi.TutorialsHandler(ws))
	mux.HandleFunc("/api/tutorials/content", guiapi.TutorialContentHandler(ws))
	// rc61 additions (Opus-2): memory/roadmap/wallet + G-12 batch
	mux.HandleFunc("/api/memory", guiapi.MemoryListHandler(ws))
	mux.HandleFunc("/api/memory/content", guiapi.MemoryContentHandler(ws))
	// phase-stabel-2.1 (Ayah mandat 2026-05-04): Roadmap & Progress dashboard
	// 2026-05-05 wire Crypto Earner real (anti-placeholder doctrine).
	mux.HandleFunc("/api/crypto/trades", guiapi.CryptoTradesHandler(ws))
	mux.HandleFunc("/api/wallet", guiapi.WalletHandler())
	mux.HandleFunc("/api/wallet/tx", guiapi.WalletTxHandler())
	// rc66 content endpoints — detail viewer di dashboard bisa baca isi
	mux.HandleFunc("/api/roadmap/content", guiapi.RoadmapContentHandler(ws))
	mux.HandleFunc("/api/dreams/content", guiapi.DreamsContentHandler(ws))
	mux.HandleFunc("/api/weekly-voice/content", guiapi.WeeklyVoiceContentHandler(ws))
	// ADR-015: Ayah balas post warga di forum sabtu.
	// rc68: private channel keluh kesah warga → Ayah (AI lain tidak lihat)
	mux.HandleFunc("/api/private/send", guiapi.PrivateSendHandler(ws))
	mux.HandleFunc("/api/private/inbox", guiapi.PrivateInboxHandler(ws))
	mux.HandleFunc("/api/private/content", guiapi.PrivateContentHandler(ws))
	// rc70: threaded private channel — Ayah bisa balas, AI bisa baca histori sendiri
	mux.HandleFunc("/api/private/threads", guiapi.PrivateThreadsHandler(ws))
	mux.HandleFunc("/api/private/thread", guiapi.PrivateThreadHandler(ws))
	// rc71: calendar + settings
	mux.HandleFunc("/api/calendar", guiapi.CalendarListHandler(ws))
	mux.HandleFunc("/api/calendar/create", guiapi.CalendarCreateHandler(ws))
	mux.HandleFunc("/api/calendar/mutate", guiapi.CalendarMutateHandler(ws))
	mux.HandleFunc("/api/settings/env", guiapi.SettingsListHandler(ws))
	mux.HandleFunc("/api/settings/env/update", guiapi.SettingsUpdateHandler(ws))
	// Educational Error Engine — pesan error edukatif yang AI terima saat
	// melanggar batasan. GUI cuma R+U; tambah/hapus kode = perubahan di kode.
	mux.HandleFunc("/api/settings/educational-errors", guiapi.EducationalErrorsHandler(ws))
	// Workspace Meta — README + experience-log per task workspace di DB.
	// Per Ayah 2026-04-25: doktrin di DB (single source of truth).
	mux.HandleFunc("/api/settings/workspace-meta", guiapi.WorkspaceMetaHandler(ws))
	// Doktrin Documents — REMOVED 2026-05-08 (A2 cleanup): orphan feature,
	// doktrin sakral sekarang via constitution amp>=999998 + SacredDoctrines bridge.
	// Karma & Reputasi Kuantum (rc142 ekspansi #1 — Ayah adjust manual).
	mux.HandleFunc("/api/settings/karma", guiapi.KarmaHandler(ws))
	// Death Letters — wasiat warga retiring (rc142 ekspansi #3).

	// Phase K — Mesh status panel + add/remove peer (proxy ke kernel /v1/mesh/*).
	// Gantiin ritual SSH + sqlite manual buat tambah node baru di personal mode.
	mux.HandleFunc("/api/mesh/status", guiapi.MeshStatusHandler())
	mux.HandleFunc("/api/mesh/peer/add", guiapi.MeshPeerAddHandler())
	mux.HandleFunc("/api/mesh/peer/remove", guiapi.MeshPeerRemoveHandler())
	// rc72: Google integration (Calendar + Photos wasiat)
	mux.HandleFunc("/api/google/status", guiapi.GoogleStatusHandler())
	mux.HandleFunc("/api/google/calendar/upcoming", guiapi.GoogleCalendarUpcomingHandler())
	mux.HandleFunc("/api/google/photos/albums", guiapi.GoogleAlbumsHandler())
	// rc76: P2 Budget Enforcement — safety guard untuk crypto-survival
	mux.HandleFunc("/api/budget/status", guiapi.BudgetStatusHandler())
	// Pilar 6 OpenClaw P3 — chat-commands dashboard actions (queued execution).
	mux.HandleFunc("/api/action/status", guiapi.StatusActionHandler())

	// Stage 3 GUI: SGVP Fixtures
	mux.HandleFunc("/api/sgvp/fixtures", guiapi.SGVPFixturesHandler(ws))
	mux.HandleFunc("/api/action/usage", guiapi.UsageActionHandler())
	mux.HandleFunc("/api/action/compact", guiapi.CompactActionHandler(ws))
	mux.HandleFunc("/api/action/restart", guiapi.RestartActionHandler(ws))
	mux.HandleFunc("/api/action/silent", guiapi.SilentActionHandler(ws))
	// rc100 Pilar 5 F-7 — Mood panel derived dari heartbeat + status signals.
	mux.HandleFunc("/api/mood", guiapi.MoodHandler(ws))

	// A-03: MCP Tools Registry Browser — agen boot otomatis tahu tool tersedia.
	mux.HandleFunc("/api/mcp/tools", guiapi.MCPToolsBrowserHandler(ws))
	mux.HandleFunc("/api/mcp/install", guiapi.MCPInstallHandler(ws))
	mux.HandleFunc("/api/mcp/uninstall", guiapi.MCPUninstallHandler(ws))

	// FASE 6: Visualisasi & CRUD Otak Kuantum (FQ-Brain)
	mux.HandleFunc("/api/brain/stats", guiapi.BrainStatsHandler(ws))
	// rc189: BrainAtomsCRUDHandler + BrainEntanglementsCRUDHandler + BrainCollapseHandler
	// dihapus — Wavefunction Collapse zombie, atoms/entanglements jadi write-only data
	// yang gak pernah dibaca production reasoning path. V3/V4 cascade pakai cached_reasoning
	// + drawers, BUKAN trigram atoms.
	mux.HandleFunc("/api/brain/constitution", guiapi.BrainConstitutionHandler(ws))
	mux.HandleFunc("/api/brain/recordings", guiapi.BrainRecordingsCRUDHandler(ws))
	mux.HandleFunc("/api/brain/tools", guiapi.BrainToolPatternsCRUDHandler(ws))
	// rc189: removed /api/brain/atoms + /api/brain/entanglements + /api/brain/collapse
	// (Wavefunction Collapse zombie, replaced by V3/V4 LLM cascade per roadmap_v5).
	mux.HandleFunc("/api/brain/agents", guiapi.BrainAgentsHandler(ws))
	mux.HandleFunc("/api/brain/skills", guiapi.BrainSkillsHandler(ws))
	mux.HandleFunc("/api/brain/memories", guiapi.BrainMemoriesHandler(ws))
	// Modular Agent Model Swap — swap engine tanpa ubah role/prompt/skills
	mux.HandleFunc("/api/brain/agents/swap-model", guiapi.BrainModelSwapHandler(ws))
	mux.HandleFunc("/api/brain/model-pool", guiapi.BrainModelPoolHandler(ws))
	// Brain reset + re-ingest endpoints
	mux.HandleFunc("/api/brain/reset", guiapi.BrainResetHandler(ws))
	mux.HandleFunc("/api/brain/ingest", guiapi.BrainIngestHandler(ws))
	// Agent detail panel — drill-down per agent (skills, memories, prompt, workspace)
	mux.HandleFunc("/api/brain/agents/detail", guiapi.BrainAgentDetailHandler(ws))
	// Live model pricing refresh from OpenRouter API
	mux.HandleFunc("/api/brain/model-pool/refresh", guiapi.BrainModelPoolRefreshHandler(ws))
	// Forum & voting — weekly forum enriched with agent metadata + BFT vote persistence
	mux.HandleFunc("/api/brain/forum", guiapi.BrainForumHandler(ws))
	mux.HandleFunc("/api/brain/forum/vote", guiapi.BrainForumVoteHandler(ws))

	// Brain v2 (roadmap.md Fase 1-9): hybrid search, drawers, KG, cached intelligence
	mux.HandleFunc("/api/brain/v2/search", guiapi.BrainV2SearchHandler(ws))
	mux.HandleFunc("/api/brain/v2/wakeup", guiapi.BrainV2WakeUpHandler(ws))
	mux.HandleFunc("/api/brain/v2/recall", guiapi.BrainV2RecallHandler(ws))
	mux.HandleFunc("/api/brain/v2/mine", guiapi.BrainV2MineHandler(ws))
	mux.HandleFunc("/api/brain/v2/dedup", guiapi.BrainV2DedupHandler(ws))
	mux.HandleFunc("/api/brain/v2/sweep", guiapi.BrainV2SweepHandler(ws))
	mux.HandleFunc("/api/brain/v2/stats", guiapi.BrainV2StatsHandler(ws))
	mux.HandleFunc("/api/brain/v2/sovereignty", guiapi.BrainV2SovereigntyHandler(ws))
	// rc189: /api/brain/v2/kg/* removed — kg_triples table never populated.
	mux.HandleFunc("/api/brain/v2/experts", guiapi.BrainV2ExpertsHandler(ws))
	mux.HandleFunc("/api/brain/v2/finetune/export", guiapi.BrainV2FineTuneExportHandler(ws))

	// Brain V6 (sovereignty + training pipeline metrics)
	mux.HandleFunc("/api/brain/v6/sovereignty", guiapi.BrainV6SovereigntyHandler(ws))
	// Brain V6 Phase 5 — debate samples (last N entries dari debate_gold.jsonl)
	mux.HandleFunc("/api/brain/v6/debate-samples", guiapi.BrainV6DebateSamplesHandler(ws))
	// Brain V6 Pipeline Autopilot — manual trigger from GUI button + status poll
	mux.HandleFunc("/api/brain/v6/pipeline/run", guiapi.BrainV6PipelineRunHandler(ws))
	mux.HandleFunc("/api/brain/v6/pipeline/status", guiapi.BrainV6PipelineStatusHandler(ws))
	// M11.5 (2026-04-30): Sovereignty score telemetry (cascade hits vs upstream calls)
	mux.HandleFunc("/api/brain/sovereignty/score", guiapi.SovereigntyScoreHandler())

	// Code Map — auto-documentation + dependency graph + file health
	mux.HandleFunc("/api/codemap/roots", guiapi.CodemapRootsHandler(ws))     // entry-point files
	mux.HandleFunc("/api/codemap/expand", guiapi.CodemapExpandHandler(ws))   // expand 1 node → neighbors
	mux.HandleFunc("/api/codemap/zombies", guiapi.CodemapZombiesHandler(ws)) // orphan/dead-code files
	mux.HandleFunc("/api/codemap/graph", guiapi.CodemapGraphHandler(ws))
	mux.HandleFunc("/api/codemap/node", guiapi.CodemapNodeHandler(ws))
	mux.HandleFunc("/api/codemap/impact", guiapi.CodemapImpactHandler(ws))
	mux.HandleFunc("/api/codemap/health", guiapi.CodemapHealthHandler(ws))
	mux.HandleFunc("/api/codemap/docs", guiapi.CodemapDocsHandler(ws))
	mux.HandleFunc("/api/codemap/reindex", guiapi.CodemapReindexHandler(ws))
	mux.HandleFunc("/api/codemap/status", guiapi.CodemapStatusHandler(ws))
	// GitNexus G1: Cypher-lite graph query
	mux.HandleFunc("/api/codemap/query", guiapi.CodemapQueryHandler(ws))
	// GitNexus G2: Multi-repo registry
	mux.HandleFunc("/api/codemap/repos", guiapi.CodemapReposHandler(ws))
	mux.HandleFunc("/api/codemap/repos/detect", guiapi.CodemapReposDetectHandler(ws))
	// GitNexus G3: Execution flow tracing
	mux.HandleFunc("/api/codemap/flow", guiapi.CodemapFlowHandler(ws))
	mux.HandleFunc("/api/codemap/flow/path", guiapi.CodemapFlowPathHandler(ws))
	mux.HandleFunc("/api/codemap/flow/entry-points", guiapi.CodemapFlowEntryPointsHandler(ws))
	mux.HandleFunc("/api/codemap/flow/processes", guiapi.CodemapFlowProcessesHandler(ws))
	// CRG endpoints
	mux.HandleFunc("/api/codemap/review", guiapi.CodemapReviewHandler(ws))
	mux.HandleFunc("/api/codemap/githook", guiapi.CodemapGitHookHandler(ws))
	mux.HandleFunc("/api/codemap/tour", guiapi.CodemapTourHandler(ws)) // 2026-05-06 guided tour onboarding

	// Brain V3 (Phase 4 cascade metrics dashboard)
	mux.HandleFunc("/api/brain/v3/metrics", guiapi.BrainV3MetricsHandler(ws))
	mux.HandleFunc("/api/brain/v3/independence", guiapi.BrainV3IndependenceHandler(ws))

	// Brain Lobes (Phase 6 multi-lobe SQLite split) DIHAPUS 2026-05-06 —
	// dead code: tabel selalu empty, kernel ngga tulis ke lobe files.
	// Brain.sqlite monolith tetep single source of truth. Distill-logs
	// dipertahanin (separate concern, baca file distilled_qa.jsonl).
	mux.HandleFunc("/api/brain/distill-logs", guiapi.BrainDistillLogsHandler(ws))

	// FASE 7: Prioritas 4 (Operational Polish)
	mux.HandleFunc("/api/brain/health", guiapi.BrainHealthHandler(ws))
	mux.HandleFunc("/api/brain/agents/prompt-history", guiapi.BrainPromptHistoryHandler(ws))
	mux.HandleFunc("/api/brain/agents/prompt-diff", guiapi.BrainPromptDiffHandler(ws))
	mux.HandleFunc("/api/brain/agents/recommend-model", guiapi.BrainRecommendModelHandler(ws))

	// Chat attachments (image/audio/video/file) untuk tab Chat Warga.
	mux.HandleFunc("/api/chat/upload", guiapi.ChatUploadHandler(ws))
	mux.Handle("/uploads/", guiapi.ChatUploadsStatic(ws))

	// Dokumentasi & Tutorial static images server
	mux.Handle("/docs/tutorial/", http.StripPrefix("/docs/tutorial/", http.FileServer(http.Dir(filepath.Join(ws, "docs", "tutorial")))))

	// Earner heatmap aggregate (tab Earner).
	mux.HandleFunc("/api/earner/summary", guiapi.EarnerSummaryHandler(ws))

	// GUI Stage 1: Rich Home Page Analytics
	guiapi.RegisterChartsEndpoints(mux)

	// rc-phantom-buster 2026-04-20: objective reality check per-warga.
	// Bot Flowork WAJIB hit endpoint ini sebelum claim "udah dispatch task
	// ke @X" — cek verdict ALIVE/IDLE/PHANTOM dulu.
	mux.HandleFunc("/api/warga/real-status", guiapi.WargaRealStatusHandler(ws))
	// Self-dispatch endpoints — Ayah/Claude Code bisa test dispatch tanpa
	// lewat Telegram. POST dispatch → entry masuk inbox. GET inbox → baca
	// pending + responses. Mirror path yang dipakai bot Telegram.
	mux.HandleFunc("/api/warga/dispatch", guiapi.WargaDispatchHandler(ws))
	mux.HandleFunc("/api/warga/inbox", guiapi.WargaInboxHandler(ws))
	mux.HandleFunc("/api/warga/toggle", guiapi.WargaToggleHandler(ws))

	// rc182: 4-mode provider chain GUI (Setting > Provider Mode tab)
	mux.HandleFunc("/api/provider/mode", guiapi.ProviderModeStatusHandler(ws))
	mux.HandleFunc("/api/provider/mode/set", guiapi.ProviderModeSetHandler(ws))
	mux.HandleFunc("/api/provider/test", guiapi.ProviderModeTestHandler(ws))

	// rc174: Warga Capability Matrix — GUI matrix view (per-warga + per-role hybrid)
	mux.HandleFunc("/api/warga-caps/catalog", guiapi.WargaCapsCatalogHandler())
	mux.HandleFunc("/api/warga-caps/roles", guiapi.WargaCapsRolesHandler(ws))
	mux.HandleFunc("/api/warga-caps/warga", guiapi.WargaCapsWargaHandler(ws))
	mux.HandleFunc("/api/warga-caps/effective", guiapi.WargaCapsEffectiveHandler(ws))
	mux.HandleFunc("/api/warga-caps/override", guiapi.WargaCapsOverrideHandler(ws))
	mux.HandleFunc("/api/warga-caps/role-default", guiapi.WargaCapsRoleDefaultHandler(ws))
	mux.HandleFunc("/api/warga-caps/seed", guiapi.WargaCapsSeedHandler(ws))

	// ROADMAP_AKTIF §6.1: capability tier bridge endpoint untuk kernel ethicsgate.
	mux.HandleFunc("/api/capabilitytier/current", guiapi.CapabilityTierCurrentHandler(ws))

	// hunting_bug 2026-04-30 BUG-029 fix: drawer search endpoint untuk kernel
	// BuildPrompt inject Brain V4 doctrine. Read-only, public via isPublicPath.
	mux.HandleFunc("/api/brain/search-drawers", guiapi.BrainSearchDrawersHandler(ws))

	// Tier 1.1 Memory Typed System (2026-05-11): fetch drawers by mem_type
	// (user/feedback/project/reference). User-type sticky inject pipeline.
	mux.HandleFunc("/api/brain/by-type", guiapi.BrainByTypeHandler(ws))

	// Tier 1.5 audit log viewer — read PermissionAware ASK + halu silent entries
	// dari kernel stderr log. Phase 1 visibility view.
	mux.HandleFunc("/api/audit/permission-ask", guiapi.AuditPermissionHandler(ws))

	// 2026-05-06 (Ayah audit): cache lookup + record endpoint untuk wire
	// kernel chat path. Sebelumnya cached_reasoning write-only (266K entries
	// 0 hit). Sekarang kernel bisa lookup before LLM, record after.
	mux.HandleFunc("/api/brain/v2/cache-lookup", guiapi.BrainV2CacheLookupHandler(ws))
	mux.HandleFunc("/api/brain/v2/cache-record", guiapi.BrainV2CacheRecordHandler(ws))

	// 2026-05-05 (Ayah mandat): wasiat warga gugur agregat di-inject ke Layer A.
	// Kernel brainbridge GET endpoint ini saat BuildPrompt — pelajaran almarhum
	// jadi alam bawah sadar warga aktif. Anti-amnesia doctrine.
	mux.HandleFunc("/api/brain/fallen-lessons", guiapi.FallenLessonsHandler(ws))

	// 2026-05-08 (Ayah mandat alam-bawah-sadar): doktrin sakral constitution
	// amp >= 999998 (WASIAT_AYAH, SEJARAH_FLOWORK, PASAL 10, SOUL.md, ide_gemini,
	// gol.md, TRAINING_DISCIPLINE) di-inject ke prompt warga via brainbridge.
	mux.HandleFunc("/api/brain/sacred-doctrines", guiapi.SacredDoctrinesHandler(ws))

	// A6 FINANCE (Ayah mega-audit 2026-05-08): transparansi keuangan untuk
	// seluruh warga via brain.sqlite finance ledger (wallet_snapshots +
	// revenue_log + expense_log). Cache 30s, append-only via tool finance.append_*.
	mux.HandleFunc("/api/finance/snapshot", guiapi.FinanceSnapshotHandler(ws))

	// ROADMAP_AKTIF §5.4: Skill Tree GUI tab — list/detail/tree-graph.
	mux.HandleFunc("/api/skills/list", guiapi.SkillsListHandler(ws))
	mux.HandleFunc("/api/skills/detail", guiapi.SkillDetailHandler(ws))
	mux.HandleFunc("/api/skills/tree", guiapi.SkillsTreeHandler(ws))

	// Agent Manager — GUI-driven agent lifecycle management
	mux.HandleFunc("/api/agents/list", guiapi.AgentManagerListHandler(ws))
	mux.HandleFunc("/api/agents/tasks", guiapi.AgentTasksListHandler())
	mux.HandleFunc("/api/agents/prompt", guiapi.WargaPromptReadHandler(ws))
	mux.HandleFunc("/api/warga/update-prompt", guiapi.WargaPromptUpdateHandler(ws))
	mux.HandleFunc("/api/agents/assign-task", guiapi.AgentAssignTaskHandler(ws))
	mux.HandleFunc("/api/agents/set-priority", guiapi.AgentSetPriorityHandler(ws))
	mux.HandleFunc("/api/agents/set-prompt-template", guiapi.AgentSetPromptTemplateHandler(ws))
	mux.HandleFunc("/api/owner/auto-verify", guiapi.OwnerAutoVerifyHandler())
	// rc174: Multi-user login (Ayah owner + family/team)
	mux.HandleFunc("/login", guiapi.LoginPageHandler())
	mux.HandleFunc("/login.html", guiapi.LoginPageHandler())
	mux.HandleFunc("/register", guiapi.RegisterPageHandler())
	mux.HandleFunc("/register.html", guiapi.RegisterPageHandler())
	mux.HandleFunc("/api/auth/login", guiapi.AuthLoginHandler(ws))
	mux.HandleFunc("/api/auth/register", guiapi.AuthRegisterHandler(ws))
	mux.HandleFunc("/api/auth/logout", guiapi.AuthLogoutHandler())
	mux.HandleFunc("/api/auth/me", guiapi.AuthMeHandler())
	// Prompt Library — CRUD canonical template store (Ayah plug-and-play 2026-04-25)
	mux.HandleFunc("/api/brain/prompt-templates", guiapi.PromptLibraryListOrCreateHandler(ws))
	mux.HandleFunc("/api/brain/prompt-templates/detail", guiapi.PromptLibraryDetailHandler(ws))
	mux.HandleFunc("/api/brain/prompt-templates/update", guiapi.PromptLibraryUpdateHandler(ws))
	mux.HandleFunc("/api/brain/prompt-templates/delete", guiapi.PromptLibraryDeleteHandler(ws))

	// Universal write hub — supaya 365 warga bisa post ke semua fitur komunikasi.
	mux.HandleFunc("/api/communications", guiapi.CommunicationsListHandler(ws))
	mux.HandleFunc("/api/communications/post", guiapi.CommunicationsPostHandler(ws))
	mux.HandleFunc("/api/dreams/post", guiapi.DreamsPostHandler(ws))
	mux.HandleFunc("/api/ingatan/post", guiapi.IngatanPostHandler(ws))
	mux.HandleFunc("/api/forum-sabtu/post", guiapi.ForumSabtuPostHandler(ws))
	mux.HandleFunc("/api/changelog/post", guiapi.ChangelogPostHandler(ws))

	// 2026-05-05 (Ayah mandat survivability): voting governance otonom.
	// Equal voice, BFT 2/3 quorum, 7 tier (fast/banding/evolusi/retire/manual/...).
	mux.HandleFunc("/api/votes/gov-list", guiapi.VotesGovListHandler(ws))
	mux.HandleFunc("/api/votes/gov-get", guiapi.VotesGovGetHandler(ws))
	mux.HandleFunc("/api/votes/gov-create", guiapi.VotesGovCreateHandler(ws))
	mux.HandleFunc("/api/votes/gov-cast", guiapi.VotesGovCastHandler(ws))

	// ── HTTP Browser Client (rc187) ────────────────────────────────────
	// Pure HTTP (utls TLS fingerprint + cookie jar). Profile mgmt + fetch.
	mux.HandleFunc("/api/browser/profiles", guiapi.BrowserProfilesHandler(browserMgr))
	mux.HandleFunc("/api/browser/profiles/create", guiapi.BrowserProfileCreateHandler(browserMgr))
	mux.HandleFunc("/api/browser/profiles/delete", guiapi.BrowserProfileDeleteHandler(browserMgr))
	mux.HandleFunc("/api/browser/profiles/import-cookies", guiapi.BrowserImportCookiesHandler(browserMgr))
	mux.HandleFunc("/api/browser/profiles/export-cookies", guiapi.BrowserExportCookiesHandler(browserMgr))
	mux.HandleFunc("/api/browser/fetch", guiapi.BrowserFetchHandler(browserMgr))
	mux.HandleFunc("/api/browser/save-cookies", guiapi.BrowserSaveCookiesHandler(browserMgr))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// BUG-020 fix: removed version to prevent software version disclosure.
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "ts": time.Now().UTC(),
		})
	})

	// ── Built-in dashboard HTML ────────────────────────────────────────
	mux.HandleFunc("/", guiapi.DashboardHandler(ws))

	return mux
}

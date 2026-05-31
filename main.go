// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Single-binary bootstrap. Audit pass — flag.Parse addr, embed.FS
//   static, SIGINT/SIGTERM signal handling, srv timeout (ReadHeader 15s,
//   Idle 120s), graceful shutdown 5s, scheduler/walletalert/watchdog
//   engines wired dengan ctx propagation, mock API fallback handler.
//   Owner-mode auth stub OK untuk single-user.
//
// flowork-gui — Flowork microkernel + control panel, single binary.
//
// Sebelumnya GUI lama (:1987) proxy ke kernel terpisah (:1988). Sekarang
// kernel embedded via internal/kernelhost — satu port saja: :1987.
//
// Konsep: tiap agent punya tombol Setting yang buka popup (router / prompt
// / tools / schedule) — state warga terisolasi di state.db (agentdb). DI LUAR
// itu ada halaman Settings GLOBAL (owner-level) + DB global flowork.db
// (floworkdb): auth single-owner, API key, wallet personal. Warga TIDAK
// menyimpan apa pun di flowork.db.
//
// Packages aktif:
//
//	internal/httpx       JSON writer + no-cache middleware
//	internal/kernel      wazero runtime + capability broker + loader (WASI plugin)
//	internal/kernelhost  embed kernel + HTTP handlers (/api/kernel/*)
//	internal/agentmgr    /api/agents/* (upload/download/remove/config)

package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/codescan"
	"flowork-gui/internal/floworkauth"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/settingsapi"
	"flowork-gui/internal/scheduler"
	"flowork-gui/internal/walletalert"
	"flowork-gui/internal/watchdog"
	"flowork-gui/internal/slashcmd"
	slashbuiltins "flowork-gui/internal/slashcmd/builtins"
	slashcustom "flowork-gui/internal/slashcmd/custom"
	"flowork-gui/internal/tools"
	"flowork-gui/internal/tools/builtins"
)

//go:embed web
var webFS embed.FS

const version = "1.0.0"

func main() {
	addr := flag.String("addr", "127.0.0.1:1987", "listen address")
	flag.Parse()

	staticFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Section 11 phase 1a-1f: register 11 builtin tools (echo, now,
	// memory_x3, file_x3, brain_search, telegram_send, webfetch).
	// Section 14 phase 1: register 3 builtin slash commands (help, echo, ping).
	// Both Init panic on duplicate name — early bug catch.
	builtins.Init()
	slashbuiltins.Init()
	// Section 12 phase 2: register 3 built-in interceptor (workspace-path,
	// sensitive-file, persona-inject). SandboxRunV2 di agentmgr panggil chain
	// sebelum 3-gate sandbox.
	tools.InitDefaultInterceptors()

	// Section 17 phase 3: register pre-/post-hook framework + decisions log
	// + rate limit. DispatchWithHooks wraps locked Dispatch.
	slashcmd.InitHooks()

	// Section 17/15: wire slash dispatcher callback. Kernelhost pre-populate
	// ctx (Store/Caller/Agent) sebelum invoke ini supaya productive commands
	// (/stats /tools dst.) bisa akses store via slashcmd.FromStore.
	kernelhost.SlashDispatcherFunc = func(ctx context.Context, pluginID, text, caller string) (string, string, error) {
		result, cmdName, err := slashcmd.DispatchWithHooks(ctx, text)
		return result.Text, cmdName, err
	}

	host, err := kernelhost.Boot(ctx)
	if err != nil {
		log.Fatalf("kernel boot: %v", err)
	}
	defer host.Close(context.Background())

	// Section 16 phase 2: multi-warga + hot-reload fsnotify.
	// Enumerate semua agent → resolve <sharedDir>/<agentID>/commands/ → load
	// all + start fsnotify watcher debounce 500ms.
	commandsDirs := []string{}
	for _, agentID := range host.AgentIDs() {
		if shared, derr := host.SharedDirForAgent(agentID); derr == nil && shared != "" {
			commandsDirs = append(commandsDirs, filepath.Join(shared, "commands"))
		}
	}
	if loaded, skipped, lerr := slashcustom.LoadFromDirs(commandsDirs); lerr != nil {
		log.Printf("custom slash load: %v", lerr)
	} else if loaded > 0 || skipped > 0 {
		log.Printf("custom slash: loaded=%d skipped=%d across %d dirs", loaded, skipped, len(commandsDirs))
	}
	if werr := slashcustom.StartWatcher(ctx, commandsDirs); werr != nil {
		log.Printf("custom slash watcher: %v (hot-reload disabled)", werr)
	}

	// Wire ConfigHandler → kernel reload callback. Tanpa ini, save config
	// dari popup ngga restart daemon → env baru ngga kebawa.
	agentmgr.AgentIDsFunc = host.AgentIDs
	agentmgr.Reload = host.ReloadAgent
	agentmgr.RetentionSweep = func(agentID string) (any, error) {
		return host.RunRetentionForAgent(agentID)
	}
	agentmgr.WorkspaceRebuildIndex = func(agentID string) (any, error) {
		return host.RebuildWorkspaceMetaForAgent(agentID)
	}
	agentmgr.PromoteRun = func(agentID string) (any, error) {
		return host.RunPromoteForAgent(agentID)
	}
	agentmgr.SharedDirForAgent = func(agentID string) (string, error) {
		return host.SharedDirForAgent(agentID)
	}
	agentmgr.CapsCheckerForAgent = func(agentID string) func(capability string) bool {
		return host.CapsCheckerForAgent(agentID)
	}

	// Section 18 phase 1: scheduler engine. Tick 60s align ke top-of-minute.
	// Executor: kalau task mulai `/` → slash dispatch; selain itu → RPC
	// handle_message ke agent WASM (sama path Telegram).
	schedEngine := scheduler.New(
		host.AgentIDs,
		func(agentID string) (*agentdb.Store, error) {
			return host.OpenAgentStore(agentID)
		},
		func(ctx context.Context, agentID, scheduleID, task string) (string, error) {
			// Scheduler executor: forward task as user message ke agent
			// WASM. Agent's doHandle akan deteksi leading `/` dan route ke
			// slash dispatcher (Section 17 phase 2 parity), atau LLM
			// kalau plain text.
			task = strings.TrimSpace(task)
			reply, runErr := host.InvokeAgentMessage(ctx, agentID, task, "scheduler")
			// Section 18 phase 2: decisions log + karma update sinkron.
			if store, oerr := host.OpenAgentStore(agentID); oerr == nil {
				defer store.Close()
				outcome := "success"
				rationale := "schedule fire: " + task
				karmaKey := "schedule_success_count"
				if runErr != nil {
					outcome = "fail"
					rationale = "schedule fire fail: " + runErr.Error()
					if len(rationale) > 512 {
						rationale = rationale[:512] + "…"
					}
					karmaKey = "schedule_fail_count"
				}
				inputs := map[string]any{
					"schedule_id": scheduleID,
					"task":        task,
					"agent_id":    agentID,
					"reply_len":   len(reply),
				}
				_, _ = store.LogDecision("schedule_fire", rationale, outcome, inputs, 0)
				_, _ = store.IncrementKarma(karmaKey, 1)
			}
			return reply, runErr
		},
	)
	schedEngine.Start(ctx)
	defer schedEngine.Stop()
	agentmgr.SchedulerFireFunc = func(agentID, scheduleID string) (int64, error) {
		return schedEngine.FireNow(ctx, agentID, scheduleID)
	}

	// Section 22 phase 2: wallet alert cron evaluator. 1h tick + 24h
	// cooldown anti-spam. Notifier dispatch via Telegram tool (atau log
	// fallback kalau channel=log).
	walletEngine := walletalert.New(
		host.AgentIDs,
		host.OpenAgentStore,
		func(ctx context.Context, agentID, channel, target, message string) error {
			if channel == "telegram" && target != "" {
				// Phase 2 minimal: log dispatch saja. Phase 3 wire ke
				// telegram_send tool via Mr.Flow.
				log.Printf("[walletalert→telegram] agent=%s chat=%s msg=%s", agentID, target, message)
				return nil
			}
			log.Printf("[walletalert→%s] %s", channel, message)
			return nil
		},
	)
	walletEngine.Start(ctx)
	defer walletEngine.Stop()
	agentmgr.WalletAlertFireFunc = func() (int, int) {
		return walletEngine.FireNow(ctx)
	}

	// Section 26 phase 2: watchdog cron evaluator. Tick 60s, default rules
	// (protector_burst ≥10/60s CRITICAL, scanner_critical_burst ≥5/1h HIGH,
	// tool_call_storm ≥100/60s WARNING). 1h cooldown anti-spam per rule.
	watchdogEngine := watchdog.New(
		host.AgentIDs,
		host.OpenAgentStore,
		func(ctx context.Context, agentID, channel, message string) error {
			log.Printf("[watchdog→%s] agent=%s %s", channel, agentID, message)
			return nil
		},
	)
	watchdogEngine.Start(ctx)
	defer watchdogEngine.Stop()
	agentmgr.WatchdogFireFunc = func() (int, int) {
		return watchdogEngine.FireNow(ctx)
	}

	host.AutoBootDaemons(ctx)
	if err := host.StartWatcher(ctx); err != nil {
		log.Printf("kernel watcher start failed: %v (hot-reload disabled)", err)
	}
	// Section 8: retention cron — sweep tiap 24h, hard-delete grace 90 hari.
	host.StartRetentionCron(ctx, 24*time.Hour)

	// Doktrin edukasi — seed katalog educational_errors default ke tiap agent
	// (idempotent INSERT OR IGNORE; edit owner via GUI ngga ke-overwrite).
	for _, agentID := range host.AgentIDs() {
		if store, derr := host.OpenAgentStore(agentID); derr == nil {
			if n, serr := store.SeedEduErrors(); serr == nil && n > 0 {
				log.Printf("edu-errors: seeded %d entry baru → %s", n, agentID)
			}
			store.Close()
		}
	}

	// Background code scanner — watch source repo + kode buatan AI; auto-scan
	// file yang berubah, deteksi bug/celah dari tiap update. Critical/high →
	// audit log + push Telegram ke owner. (Tampil di tab Scanner sbg "auto:*".)
	codescanRoot := strings.TrimSpace(os.Getenv("FLOWORK_CODESCAN_ROOT"))
	if codescanRoot == "" {
		codescanRoot = agentdb.ProjectRoot()
	}
	codescanEngine := codescan.New(
		host.AgentIDs, host.OpenAgentStore, host.SharedDirForAgent,
		func(nctx context.Context, title, body string) error {
			return notifyOwnerTelegram(nctx, title+"\n\n"+body)
		},
		codescanRoot,
	)
	codescanEngine.Start(ctx)

	// DB global Flowork (owner-level: auth, settings, API key, wallet personal).
	// TERPISAH dari state.db per-warga (agentdb) — warga tetap terisolasi.
	fdb, err := floworkdb.Shared()
	if err != nil {
		log.Fatalf("flowork.db open: %v", err)
	}
	// Inject API key tersimpan → env, supaya engine (wallet, dll) langsung
	// pakai konfigurasi dari Settings tanpa hardcode/restart. Hanya key
	// UPPER_SNAKE (env-var) yang di-set; password hash (lowercase) di-skip.
	if secrets, serr := fdb.AllSecrets(); serr == nil {
		for k, v := range secrets {
			if k == strings.ToUpper(k) && strings.TrimSpace(v) != "" {
				_ = os.Setenv(k, v)
			}
		}
	}
	authMgr := floworkauth.NewManager(fdb)
	settingsAPI := settingsapi.New(fdb)
	// Wire host accessor untuk AI-wallets read-only (host-level, bukan cross-warga).
	settingsapi.AgentIDsFunc = host.AgentIDs
	settingsapi.OpenAgentStoreFunc = host.OpenAgentStore

	mux := http.NewServeMux()

	// Auth — single-owner password (floworkauth). Session cookie in-memory.
	mux.HandleFunc("/api/auth/me", authMgr.MeHandler)
	mux.HandleFunc("/api/auth/login", authMgr.LoginHandler)
	mux.HandleFunc("/api/auth/register", authMgr.RegisterHandler)
	mux.HandleFunc("/api/auth/logout", authMgr.LogoutHandler)
	mux.HandleFunc("/api/auth/change-password", authMgr.ChangePasswordHandler)
	mux.HandleFunc("/api/owner/auto-verify", ownerAutoVerify)
	mux.HandleFunc("/api/system/health", systemHealth)

	// Page routes — FileServer cuma map exact filename (/login.html), jadi
	// /login & /register butuh handler eksplisit yang serve embedded HTML.
	servePage := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			data, rerr := fs.ReadFile(staticFS, name)
			if rerr != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
		}
	}
	mux.HandleFunc("/login", servePage("login.html"))
	mux.HandleFunc("/register", servePage("register.html"))

	// Settings (owner-level, flowork.db global).
	mux.HandleFunc("/api/settings/wallet/addresses", settingsAPI.WalletAddressesHandler)
	mux.HandleFunc("/api/settings/wallet/portfolio", settingsAPI.WalletPortfolioHandler)
	mux.HandleFunc("/api/settings/keys", settingsAPI.KeysHandler)
	mux.HandleFunc("/api/settings/ai-wallets", settingsAPI.AIWalletsHandler)
	mux.HandleFunc("/api/settings/notify", settingsAPI.NotifyHandler)
	settingsapi.TestNotifyFunc = notifyOwnerTelegram

	// Kernel introspection (list agent, RPC call).
	mux.HandleFunc("/api/kernel/status", host.StatusHandler)
	mux.HandleFunc("/api/kernel/agents", host.AgentsHandler)
	mux.HandleFunc("/api/kernel/rpc", host.RPCHandler)
	mux.HandleFunc("/api/agents/ui-schema", host.UISchemaHandler)

	// Agent manager (upload .fwagent.zip, config per agent).
	mux.HandleFunc("/api/agents/upload", agentmgr.UploadHandler)
	mux.HandleFunc("/api/agents/download", agentmgr.DownloadHandler)
	mux.HandleFunc("/api/agents/remove", agentmgr.RemoveHandler)
	mux.HandleFunc("/api/agents/config", agentmgr.ConfigHandler)
	mux.HandleFunc("/api/agents/toggle", agentmgr.ToggleHandler)
	mux.HandleFunc("/api/agents/db/reset", agentmgr.DBResetHandler)
	mux.HandleFunc("/api/agents/interactions", agentmgr.InteractionsHandler)
	mux.HandleFunc("/api/agents/decisions", agentmgr.DecisionsHandler)
	mux.HandleFunc("/api/agents/mistakes", agentmgr.MistakesHandler)
	mux.HandleFunc("/api/agents/retention/sweep", agentmgr.RetentionSweepHandler)
	mux.HandleFunc("/api/agents/death-letter", agentmgr.DeathLetterHandler)
	mux.HandleFunc("/api/agents/karma", agentmgr.KarmaHandler)
	mux.HandleFunc("/api/agents/workspace-meta", agentmgr.WorkspaceMetaHandler)
	mux.HandleFunc("/api/agents/promote/run", agentmgr.PromoteRunHandler)
	mux.HandleFunc("/api/agents/edu-errors", agentmgr.EduErrorsHandler)
	mux.HandleFunc("/api/agents/tools/registry", agentmgr.ToolRegistryHandler)
	mux.HandleFunc("/api/agents/tool-invocations", agentmgr.ToolInvocationsHandler)
	mux.HandleFunc("/api/agents/tools/run", agentmgr.ToolRunHandler)
	mux.HandleFunc("/api/agents/slash/run", agentmgr.SlashRunHandler)
	mux.HandleFunc("/api/agents/slash/registry", agentmgr.SlashRegistryHandler)
	mux.HandleFunc("/api/agents/slash-invocations", agentmgr.SlashInvocationsHandler)
	mux.HandleFunc("/api/agents/router-skills/list", agentmgr.RouterSkillsListHandler)
	mux.HandleFunc("/api/agents/router-skills/get", agentmgr.RouterSkillsGetHandler)
	// Section 13 phase 2 — tool subscriptions + suggest.
	mux.HandleFunc("/api/agents/tools/catalog", agentmgr.ToolCatalogHandler)
	mux.HandleFunc("/api/agents/tools/my", agentmgr.ToolMyHandler)
	mux.HandleFunc("/api/agents/tools/subscribe", agentmgr.ToolSubscribeHandler)
	mux.HandleFunc("/api/agents/tools/unsubscribe", agentmgr.ToolUnsubscribeHandler)
	mux.HandleFunc("/api/agents/tools/suggest", agentmgr.ToolSuggestHandler)
	mux.HandleFunc("/api/agents/scheduler/runs", agentmgr.SchedulerRunsHandler)
	mux.HandleFunc("/api/agents/scheduler/trigger", agentmgr.SchedulerTriggerHandler)
	mux.HandleFunc("/api/agents/sneakernet/export", agentmgr.SneakernetExportHandler)
	mux.HandleFunc("/api/agents/sneakernet/import", agentmgr.SneakernetImportHandler)
	mux.HandleFunc("/api/agents/mesh/identity", agentmgr.MeshIdentityHandler)
	mux.HandleFunc("/api/agents/mesh/peers", agentmgr.MeshPeersHandler)
	mux.HandleFunc("/api/agents/wallet/addresses", agentmgr.WalletAddressesHandler)
	mux.HandleFunc("/api/agents/wallet/portfolio", agentmgr.WalletPortfolioHandler)
	mux.HandleFunc("/api/agents/wallet/snapshots", agentmgr.WalletSnapshotsHandler)
	mux.HandleFunc("/api/agents/wallet/alerts", agentmgr.WalletAlertsHandler)
	mux.HandleFunc("/api/agents/wallet/alerts/fired", agentmgr.WalletAlertsFiredHandler)
	mux.HandleFunc("/api/agents/wallet/alerts/tick", agentmgr.WalletAlertTickHandler)
	mux.HandleFunc("/api/agents/watchdog/tick", agentmgr.WatchdogTickHandler)
	mux.HandleFunc("/api/agents/finance/ledger", agentmgr.FinanceLedgerHandler)
	mux.HandleFunc("/api/agents/finance/summary", agentmgr.FinanceSummaryHandler)
	mux.HandleFunc("/api/agents/finance/budget", agentmgr.FinanceBudgetHandler)
	mux.HandleFunc("/api/agents/finance/check_budget", agentmgr.FinanceCheckBudgetHandler)
	mux.HandleFunc("/api/agents/protector/rules", agentmgr.ProtectorRulesHandler)
	mux.HandleFunc("/api/agents/protector/test", agentmgr.ProtectorTestHandler)
	mux.HandleFunc("/api/agents/protector/audit", agentmgr.ProtectorAuditHandler)
	mux.HandleFunc("/api/agents/scanner/scan", agentmgr.ScannerScanHandler)
	mux.HandleFunc("/api/agents/scanner/runs", agentmgr.ScannerRunsHandler)
	mux.HandleFunc("/api/agents/scanner/findings", agentmgr.ScannerFindingsHandler)
	mux.HandleFunc("/api/agents/scanner/auditors", agentmgr.ScannerAuditorsHandler)
	mux.HandleFunc("/api/agents/audit/log", agentmgr.AuditLogHandler)
	mux.HandleFunc("/api/agents/watchdog/alerts", agentmgr.WatchdogAlertsHandler)
	mux.HandleFunc("/api/agents/codemap/index", agentmgr.CodemapIndexHandler)
	mux.HandleFunc("/api/agents/codemap/nodes", agentmgr.CodemapNodesHandler)
	mux.HandleFunc("/api/agents/zombie/findings", agentmgr.ZombieFindingsHandler)
	mux.HandleFunc("/api/agents/zombie/ack", agentmgr.ZombieAckHandler)
	mux.HandleFunc("/api/agents/zombie/scan", agentmgr.ZombieScanHandler)
	mux.HandleFunc("/api/agents/self-prompt", agentmgr.SelfPromptHandler)
	mux.HandleFunc("/api/agents/self-prompt/render", agentmgr.SelfPromptRenderHandler)

	// Legacy reference GUI compat shim — map paths dari reference tabs ke
	// agent-scoped endpoint (default agent mr-flow).
	mux.HandleFunc("/api/wallet", agentmgr.WalletCompatHandler)
	mux.HandleFunc("/api/wallet/tx", agentmgr.WalletTxCompatHandler)
	mux.HandleFunc("/api/finance/snapshot", agentmgr.FinanceSnapshotCompatHandler)
	mux.HandleFunc("/api/brain/prompt-templates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			agentmgr.PromptTemplatesUpsertCompatHandler(w, r)
			return
		}
		agentmgr.PromptTemplatesListCompatHandler(w, r)
	})
	mux.HandleFunc("/api/brain/prompt-templates/detail", agentmgr.PromptTemplatesDetailCompatHandler)
	mux.HandleFunc("/api/brain/prompt-templates/update", agentmgr.PromptTemplatesUpsertCompatHandler)
	mux.HandleFunc("/api/brain/prompt-templates/delete", agentmgr.PromptTemplatesDeleteCompatHandler)
	mux.HandleFunc("/api/protector", agentmgr.ProtectorListCompatHandler)
	mux.HandleFunc("/api/protector/add", agentmgr.ProtectorAddCompatHandler)
	mux.HandleFunc("/api/protector/remove", agentmgr.ProtectorRemoveCompatHandler)
	mux.HandleFunc("/api/protector/toggle", agentmgr.ProtectorToggleCompatHandler)
	mux.HandleFunc("/api/protector/test", agentmgr.ProtectorTestCompatHandler)
	mux.HandleFunc("/api/codemap/graph", agentmgr.CodemapGraphCompatHandler)
	mux.HandleFunc("/api/codemap/status", agentmgr.CodemapStatusCompatHandler)
	mux.HandleFunc("/api/codemap/zombies", agentmgr.CodemapZombiesCompatHandler)
	mux.HandleFunc("/api/codemap/reindex", agentmgr.CodemapReindexCompatHandler)
	mux.HandleFunc("/api/codemap/roots", agentmgr.CodemapRootsCompatHandler)
	mux.HandleFunc("/api/codemap/docs", agentmgr.CodemapDocsCompatHandler)
	mux.HandleFunc("/api/agents/protector/approval/queue", agentmgr.ApprovalQueueHandler)
	mux.HandleFunc("/api/agents/protector/approve_pending", agentmgr.ApproveHandler)
	mux.HandleFunc("/api/agents/protector/reject_pending", agentmgr.RejectHandler)
	mux.HandleFunc("/api/agents/tool-audit", agentmgr.ToolAuditHandler)
	// Section 9 reference GUI tab (doktrin_edukasi.js) — compat shim
	// → agents/edu-errors agent-scoped (lihat legacy_compat_v2.go).
	mux.HandleFunc("/api/settings/educational-errors", agentmgr.EduErrorsCompatHandler)
	// Tool Registry reference GUI tab (warga_caps.js) — single-warga shim.
	mux.HandleFunc("/api/warga-caps/warga",     agentmgr.WargaListCompatHandler)
	mux.HandleFunc("/api/warga-caps/catalog",   agentmgr.WargaCapsCatalogCompatHandler)
	mux.HandleFunc("/api/warga-caps/effective", agentmgr.WargaCapsEffectiveCompatHandler)
	mux.HandleFunc("/api/warga-caps/override",  agentmgr.WargaCapsOverrideCompatHandler)
	mux.HandleFunc("/api/warga-caps/seed",      agentmgr.WargaCapsSeedCompatHandler)
	// Audit Log reference GUI tab (commits.js) — adapt audit → git-style.
	mux.HandleFunc("/api/commits", agentmgr.CommitsCompatHandler)

	// Catch-all stub utk path /api/* yang gak diregister.
	mux.HandleFunc("/api/", mockAPI)

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpx.NoCache(authMgr.Middleware(mux)),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()
	log.Printf("flowork-gui %s listening on http://%s (agents dir: %s)", version, *addr, host.AgentsDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

// ── Auth / system stubs ────────────────────────────────────────────────────

func ownerAutoVerify(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, map[string]any{"verified": true})
}

// notifyOwnerTelegram — push pesan ke owner via Telegram. Baca config dari
// SETTINGS GLOBAL (floworkdb) — BUKAN dari agent. Owner-level notif terpisah
// dari AI agent (yang sengaja terisolasi / plug-and-play). Diam (log only)
// kalau Telegram belum di-set di Settings.
func notifyOwnerTelegram(ctx context.Context, text string) error {
	fdb, err := floworkdb.Shared()
	if err != nil {
		return err
	}
	token, _ := fdb.GetSecret("NOTIFY_TG_TOKEN")
	chatID, _ := fdb.GetKV("notify_tg_chat")
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	if token == "" || chatID == "" {
		log.Printf("[codescan→notify] (telegram notif belum di-set di Settings) %s", truncStr(text, 120))
		return nil
	}
	form := url.Values{"chat_id": {chatID}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram sendMessage status %d", resp.StatusCode)
	}
	return nil
}

// truncStr — potong string buat log.
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func systemHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, map[string]any{
		"status":  "ok",
		"version": version,
		"ts":      time.Now().UTC().Format(time.RFC3339),
	})
}

// mockAPI — shape-friendly stub untuk unregistered /api/* paths.
func mockAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case strings.HasSuffix(path, "/list"),
		strings.HasSuffix(path, "/all"),
		strings.HasSuffix(path, "/inbox"),
		strings.HasSuffix(path, "/recent"):
		httpx.WriteJSON(w, map[string]any{"data": []any{}, "count": 0})
	case strings.HasSuffix(path, "/status"),
		strings.HasSuffix(path, "/state"),
		strings.HasSuffix(path, "/config"):
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"path": path, "method": r.Method})
	}
}

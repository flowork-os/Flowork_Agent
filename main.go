// flowork-gui — Flowork microkernel + control panel, single binary.
//
// Sebelumnya GUI lama (:1987) proxy ke kernel terpisah (:1988). Sekarang
// kernel embedded via internal/kernelhost — satu port saja: :1987.
//
// Konsep: GUI cuma 1 menu "AI Agent". Tiap agent punya tombol Setting
// yang buka popup (router / prompt / tools / schedule). Tidak ada Setting
// page global, tidak ada SQLite store.
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
	"io/fs"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/slashcmd"
	slashbuiltins "flowork-gui/internal/slashcmd/builtins"
	"flowork-gui/internal/tools/builtins"
)

//go:embed web
var webFS embed.FS

const version = "0.4.0-embedded-kernel"

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

	// Section 17/15: wire slash dispatcher callback. Kernelhost pre-populate
	// ctx (Store/Caller/Agent) sebelum invoke ini supaya productive commands
	// (/stats /tools dst.) bisa akses store via slashcmd.FromStore.
	kernelhost.SlashDispatcherFunc = func(ctx context.Context, pluginID, text, caller string) (string, string, error) {
		result, cmdName, err := slashcmd.Dispatch(ctx, text)
		return result.Text, cmdName, err
	}

	host, err := kernelhost.Boot(ctx)
	if err != nil {
		log.Fatalf("kernel boot: %v", err)
	}
	defer host.Close(context.Background())

	// Wire ConfigHandler → kernel reload callback. Tanpa ini, save config
	// dari popup ngga restart daemon → env baru ngga kebawa.
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

	host.AutoBootDaemons(ctx)
	if err := host.StartWatcher(ctx); err != nil {
		log.Printf("kernel watcher start failed: %v (hot-reload disabled)", err)
	}
	// Section 8: retention cron — sweep tiap 24h, hard-delete grace 90 hari.
	host.StartRetentionCron(ctx, 24*time.Hour)

	mux := http.NewServeMux()

	// Auth / system stubs — single-user owner mode.
	mux.HandleFunc("/api/auth/me", authMe)
	mux.HandleFunc("/api/auth/logout", authLogout)
	mux.HandleFunc("/api/owner/auto-verify", ownerAutoVerify)
	mux.HandleFunc("/api/system/health", systemHealth)

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

	// Catch-all stub utk path /api/* yang gak diregister.
	mux.HandleFunc("/api/", mockAPI)

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpx.NoCache(mux),
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

func authMe(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, map[string]any{
		"name":          "Mr.Dev",
		"role":          "owner",
		"authenticated": true,
	})
}

func authLogout(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

func ownerAutoVerify(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, map[string]any{"verified": true})
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

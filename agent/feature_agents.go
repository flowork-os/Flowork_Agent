// feature_agents.go — FASE-B: semua route /api/agents/* (manajemen agent, tools, mesh,
// finance, protector, scanner, codemap, cognitive/CGM, compact, zombie, self-prompt) +
// /api/compact/config. Self-register (PhaseRoute).
package main

import (
	"net/http"
	"strconv"

	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/httpx"
)

func init() {
	RegisterFeature(Feature{Name: "agents", Phase: PhaseRoute, Apply: func(d *Deps) {
		m := d.Mux
		// Agent manager (upload/config/lifecycle).
		m.HandleFunc("/api/agents/upload", agentmgr.UploadHandler)
		m.HandleFunc("/api/agents/download", agentmgr.DownloadHandler)
		m.HandleFunc("/api/agents/remove", agentmgr.RemoveHandler)
		m.HandleFunc("/api/agents/config", agentmgr.ConfigHandler)
		m.HandleFunc("/api/agents/mcp", agentmgr.AgentMCPHandler)
		m.HandleFunc("/api/agents/duplicate", agentDuplicateHandler)
		m.HandleFunc("/api/agents/toggle", agentmgr.ToggleHandler)
		m.HandleFunc("/api/agents/db/reset", agentmgr.DBResetHandler)
		m.HandleFunc("/api/agents/interactions", agentmgr.InteractionsHandler)
		m.HandleFunc("/api/agents/decisions", agentmgr.DecisionsHandler)
		m.HandleFunc("/api/agents/mistakes", agentmgr.MistakesHandler)
		m.HandleFunc("/api/agents/retention/sweep", agentmgr.RetentionSweepHandler)
		m.HandleFunc("/api/agents/death-letter", agentmgr.DeathLetterHandler)
		m.HandleFunc("/api/agents/karma", agentmgr.KarmaHandler)
		m.HandleFunc("/api/agents/workspace-meta", agentmgr.WorkspaceMetaHandler)
		m.HandleFunc("/api/agents/promote/run", agentmgr.PromoteRunHandler)
		m.HandleFunc("/api/agents/edu-errors", agentmgr.EduErrorsHandler)
		// Tools + slash + router-skills.
		m.HandleFunc("/api/agents/tools/registry", agentmgr.ToolRegistryHandler)
		m.HandleFunc("/api/agents/tool-invocations", agentmgr.ToolInvocationsHandler)
		m.HandleFunc("/api/agents/tools/run", agentmgr.ToolRunHandler)
		m.HandleFunc("/api/agents/tools/specs", agentmgr.ToolSpecsHandler)
		m.HandleFunc("/api/agents/slash/run", agentmgr.SlashRunHandler)
		m.HandleFunc("/api/agents/slash/registry", agentmgr.SlashRegistryHandler)
		m.HandleFunc("/api/agents/slash-invocations", agentmgr.SlashInvocationsHandler)
		m.HandleFunc("/api/agents/router-skills/list", agentmgr.RouterSkillsListHandler)
		m.HandleFunc("/api/agents/router-skills/get", agentmgr.RouterSkillsGetHandler)
		m.HandleFunc("/api/agents/tools/catalog", agentmgr.ToolCatalogHandler)
		m.HandleFunc("/api/agents/tools/my", agentmgr.ToolMyHandler)
		m.HandleFunc("/api/agents/tools/subscribe", agentmgr.ToolSubscribeHandler)
		m.HandleFunc("/api/agents/tools/unsubscribe", agentmgr.ToolUnsubscribeHandler)
		m.HandleFunc("/api/agents/tools/suggest", agentmgr.ToolSuggestHandler)
		// Curator skill per-agent.
		m.HandleFunc("/api/agents/skills", agentmgr.SkillsListHandler)
		m.HandleFunc("/api/agents/skills/curate", agentmgr.SkillsCurateHandler)
		// Scheduler + sneakernet + mesh.
		m.HandleFunc("/api/agents/scheduler/runs", agentmgr.SchedulerRunsHandler)
		m.HandleFunc("/api/agents/scheduler/trigger", agentmgr.SchedulerTriggerHandler)
		m.HandleFunc("/api/agents/sneakernet/export", agentmgr.SneakernetExportHandler)
		m.HandleFunc("/api/agents/sneakernet/import", agentmgr.SneakernetImportHandler)
		m.HandleFunc("/api/agents/mesh/identity", agentmgr.MeshIdentityHandler)
		m.HandleFunc("/api/agents/mesh/peers", agentmgr.MeshPeersHandler)
		m.HandleFunc("/api/agents/watchdog/tick", agentmgr.WatchdogTickHandler)
		// Finance + protector + scanner (per-agent).
		m.HandleFunc("/api/agents/finance/ledger", agentmgr.FinanceLedgerHandler)
		m.HandleFunc("/api/agents/finance/summary", agentmgr.FinanceSummaryHandler)
		m.HandleFunc("/api/agents/finance/budget", agentmgr.FinanceBudgetHandler)
		m.HandleFunc("/api/agents/finance/check_budget", agentmgr.FinanceCheckBudgetHandler)
		m.HandleFunc("/api/agents/protector/rules", agentmgr.ProtectorRulesHandler)
		m.HandleFunc("/api/agents/protector/test", agentmgr.ProtectorTestHandler)
		m.HandleFunc("/api/agents/protector/audit", agentmgr.ProtectorAuditHandler)
		m.HandleFunc("/api/agents/scanner/scan", agentmgr.ScannerScanHandler)
		m.HandleFunc("/api/agents/scanner/runs", agentmgr.ScannerRunsHandler)
		m.HandleFunc("/api/agents/scanner/findings", agentmgr.ScannerFindingsHandler)
		m.HandleFunc("/api/agents/scanner/auditors", agentmgr.ScannerAuditorsHandler)
		m.HandleFunc("/api/agents/audit/log", agentmgr.AuditLogHandler)
		m.HandleFunc("/api/agents/watchdog/alerts", agentmgr.WatchdogAlertsHandler)
		// Codemap + cognitive (CGM) + learning + compact.
		m.HandleFunc("/api/agents/codemap/index", agentmgr.CodemapIndexHandler)
		m.HandleFunc("/api/agents/codemap/nodes", agentmgr.CodemapNodesHandler)
		m.HandleFunc("/api/agents/cognitive/graph", agentmgr.CognitiveGraphHandler)
		m.HandleFunc("/api/agents/cognitive/tensions", agentmgr.CognitiveTensionsHandler)
		m.HandleFunc("/api/agents/cognitive/digest", agentmgr.CognitiveDigestHandler)
		m.HandleFunc("/api/agents/learning/digest", agentmgr.LearningDigestHandler)
		m.HandleFunc("/api/agents/compact", agentmgr.CompactAgentHandler)
		m.HandleFunc("/api/compact/config", agentmgr.CompactConfigHandler)
		m.HandleFunc("/api/agents/compact-all", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				httpx.WriteJSON(w, map[string]any{"error": "method not allowed (POST)"})
				return
			}
			force := r.URL.Query().Get("force") == "1"
			ids := d.Host.AgentIDs()
			go agentmgr.AutoCompactAllAgents(ids, force)
			httpx.WriteJSON(w, map[string]any{
				"ok": true, "async": true, "agents": len(ids),
				"note": "Compact All jalan di background buat " + strconv.Itoa(len(ids)) + " agent (digest→trim). Cek log / refresh nanti.",
			})
		})
		m.HandleFunc("/api/agents/zombie/findings", agentmgr.ZombieFindingsHandler)
		m.HandleFunc("/api/agents/zombie/ack", agentmgr.ZombieAckHandler)
		m.HandleFunc("/api/agents/zombie/scan", agentmgr.ZombieScanHandler)
		m.HandleFunc("/api/agents/self-prompt", agentmgr.SelfPromptHandler)
		m.HandleFunc("/api/agents/self-prompt/render", agentmgr.SelfPromptRenderHandler)
	}})
}

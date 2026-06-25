// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — audit pass surface review.
// 2026-06-17 (owner-approved, PRYORITY.MD P1): +3 route additive untuk
//   constitution AMENDMENT phase 2 (amend / amendments / amend/vote).
//   Handler di handlers_brain_amend.go, engine di internal/constitution/amendments.go.
// 2026-06-17 (owner-approved, PRYORITY.MD P2 fase-2a): +5 route additive untuk
//   signed skill pack (gerbang #1) + karma-gate publish (gerbang #3).
//   Handler di handlers_skillpack.go, engine di internal/skillpack + internal/mesh/sign.go.

// HTTP route registry.

package main

import (
	"io/fs"
	"log"
	"net/http"
	"time"
)

// registerRoutes wires every handler onto mux, grouped per domain.
func registerRoutes(mux *http.ServeMux) {
	registerStaticAndHealth(mux)
	registerChatRoutes(mux)
	registerProviderRoutes(mux)
	registerManagementRoutes(mux)
	registerInfraRoutes(mux)
	registerAuthRoutes(mux)
}

// ── Static dashboard + health ───────────────────────────────────────────
func registerStaticAndHealth(mux *http.ServeMux) {
	staticFS, err := fs.Sub(webFS, "web/static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"flow_router","status":"ok","version":"` + version + `"}`))
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "flow_router",
			"status":  "ok",
			"version": version,
			"uptime":  int64(time.Since(processStartedAt).Seconds()),
		})
	})
}

// ── Chat / inference endpoints (OpenAI · Anthropic · Gemini · media) ─────
func registerChatRoutes(mux *http.ServeMux) {
	// OpenAI-compat
	mux.HandleFunc("/v1/chat/completions", chatCompletionsHandler)
	mux.HandleFunc("/v1/models", modelsHandler)
	// Anthropic native + OpenAI Responses
	mux.HandleFunc("/v1/messages", messagesV1Handler)
	mux.HandleFunc("/v1/responses", responsesV1Handler)
	// Gemini
	mux.HandleFunc("/v1beta/models", v1betaModelsHandler)
	mux.HandleFunc("/v1beta/models/", v1betaGenerateContentHandler)
	// Multimodal (routed to media-providers)
	mux.HandleFunc("/v1/embeddings", embeddingsV1Handler)
	mux.HandleFunc("/v1/images", imagesV1Handler)
	mux.HandleFunc("/v1/images/", imagesV1Handler)
	mux.HandleFunc("/v1/audio", audioV1Handler)
	mux.HandleFunc("/v1/audio/", audioV1Handler)
	mux.HandleFunc("/v1/search", searchV1Handler)
	mux.HandleFunc("/v1/web", webV1Handler)
	mux.HandleFunc("/v1/web/", webV1Handler)
	// Skills (prompt templates) — invoke by name + variables.
	mux.HandleFunc("/v1/skills/", skillInvokeHandler)
	mux.HandleFunc("/v1/api/chat", apiChatHandler)
	mux.HandleFunc("/v1/api/", apiV1Handler)
	// v1 extras (parity)
	mux.HandleFunc("/v1", v1IndexHandler)
	mux.HandleFunc("/v1/messages/count_tokens", messagesCountTokensHandler)
	mux.HandleFunc("/v1/models/info", modelsInfoHandler)
	mux.HandleFunc("/v1/models/", modelsKindHandler)
	mux.HandleFunc("/v1/responses/compact", responsesCompactHandler)
	mux.HandleFunc("/v1/audio/voices", audioVoicesHandler)
}

// ── Provider registry + validation ───────────────────────────────────────
func registerProviderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/providers", providersListAddHandler)
	mux.HandleFunc("/api/providers/validate", providerValidateHandler)
	mux.HandleFunc("/api/providers/suggested-models", providerSuggestedModelsHandler)
	mux.HandleFunc("/api/providers/test-batch", providerTestBatchHandler)
	mux.HandleFunc("/api/providers/client", providersClientHandler)
	mux.HandleFunc("/api/providers/kilo/free-models", providersKiloFreeModelsHandler)
	mux.HandleFunc("/api/providers/", providerCRUDHandler) // /:id (+ /models /test /test-models) GET/PUT/DELETE
	// Provider nodes (distributed OpenAI-compat endpoints)
	mux.HandleFunc("/api/provider-nodes", providerNodesRouterHandler)
	mux.HandleFunc("/api/provider-nodes/", providerNodesRouterHandler)
	mux.HandleFunc("/api/presets", presetsHandler)
	mux.HandleFunc("/api/combos", combosListAddHandler)
	mux.HandleFunc("/api/combos/", comboCRUDHandler)
	mux.HandleFunc("/api/models", modelsListHandler)
	mux.HandleFunc("/api/models/", modelsRouterHandler)
	mux.HandleFunc("/api/pricing", pricingHandler)
	mux.HandleFunc("/api/pricing/lookup", pricingLookupHandler)
	mux.HandleFunc("/api/tags", tagsHandler)
	mux.HandleFunc("/api/tags/", tagCRUDHandler)
}

// ── Observability + management (usage / mitm / logs / settings / media) ──
func registerManagementRoutes(mux *http.ServeMux) {
	// Usage + quota
	mux.HandleFunc("/api/usage", usageHandler)
	mux.HandleFunc("/api/usage/", usageBreakdownRouter)
	mux.HandleFunc("/api/quota-tracker", quotaTrackerHandler)
	mux.HandleFunc("/api/quota-tracker/live", quotaLiveHandler)
	mux.HandleFunc("/api/kiro/models", kiroModelsHandler)
	mux.HandleFunc("/api/kiro/models/invalidate", kiroModelsInvalidateHandler)
	// Body capture / inspect — folded into the Console Log tab (the redundant
	// /api/mitm request-feed + server-side replay were removed; replay is now a
	// client-side re-POST of the captured body from the dashboard).
	mux.HandleFunc("/api/mitm/capture-toggle", mitmCaptureToggleHandler)
	// 3E/D13 loop-belajar auto-capture + local-AI autostart = SETTING GUI (owner 2026-06-21).
	mux.HandleFunc("/api/learn/capture-toggle", learnCaptureToggleHandler)
	mux.HandleFunc("/api/localai/autostart-toggle", localAIAutostartToggleHandler)
	mux.HandleFunc("/api/mitm/full/", mitmFullDetailHandler)
	mux.HandleFunc("/api/mitm/recent-full", mitmRecentFullHandler)
	// MITM TLS proxy control (cert + DNS + status)
	mux.HandleFunc("/api/mitm/status", mitmStatusHandler)
	mux.HandleFunc("/api/mitm/root-ca", mitmRootCADownloadHandler)
	mux.HandleFunc("/api/mitm/install-ca", mitmInstallCAHandler)
	mux.HandleFunc("/api/mitm/uninstall-ca", mitmUninstallCAHandler)
	mux.HandleFunc("/api/mitm/dns/add", mitmDNSAddHandler)
	mux.HandleFunc("/api/mitm/dns/remove", mitmDNSRemoveHandler)
	// MITM interceptor lifecycle (start/stop the TLS listener) — see handlers_mitm_control.go.
	mux.HandleFunc("/api/mitm/start", mitmStartHandler)
	mux.HandleFunc("/api/mitm/stop", mitmStopHandler)
	// Media providers
	mux.HandleFunc("/api/media-providers", mediaProvidersHandler)
	mux.HandleFunc("/api/media-providers/tts", mediaTTSHandler)
	mux.HandleFunc("/api/media-providers/tts/voices", ttsVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/deepgram/voices", deepgramVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/elevenlabs/voices", elevenlabsVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/inworld/voices", inworldVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/minimax/voices", minimaxVoicesHandler)
	mux.HandleFunc("/api/media-providers/", mediaProviderCRUDHandler)
	// Skills, keys, logs
	mux.HandleFunc("/api/skills", skillsListAddHandler)
	mux.HandleFunc("/api/skills/", skillCRUDHandler)
	mux.HandleFunc("/api/keys", apiKeysListAddHandler)
	mux.HandleFunc("/api/keys/", apiKeyCRUDHandler)
	mux.HandleFunc("/api/console-log", consoleLogHandler)
	// Translator
	mux.HandleFunc("/api/translator", translatorRouterHandler)
	mux.HandleFunc("/api/translator/", translatorRouterHandler)
	// Settings + lifecycle + locale + version
	mux.HandleFunc("/api/settings", settingsHandler)
	mux.HandleFunc("/api/settings/database", settingsDatabaseHandler)
	mux.HandleFunc("/api/settings/backups", settingsBackupsHandler)
	mux.HandleFunc("/api/settings/proxy-test", settingsProxyTestHandler)
	mux.HandleFunc("/api/settings/require-login", settingsRequireLoginHandler)
	// Brain — shared knowledge brain (RAG enrichment)
	mux.HandleFunc("/api/brain/status", brainStatusHandler)
	mux.HandleFunc("/api/brain/config", brainConfigHandler)
	mux.HandleFunc("/api/brain/test", brainTestHandler)
	mux.HandleFunc("/api/brain/explore", brainExploreHandler)
	mux.HandleFunc("/api/brain/constitution", brainConstitutionHandler)
	mux.HandleFunc("/api/brain/by-type", brainByTypeHandler)
	mux.HandleFunc("/api/brain/wing", brainWingHandler) // enumerate corpus per-wing (sumber topik distilasi)
	mux.HandleFunc("/api/brain/personas", brainPersonasHandler)
	mux.HandleFunc("/api/brain/contributions", brainContributionsHandler)
	mux.HandleFunc("/api/brain/contributions/ingest", brainContributionsIngestHandler)
	mux.HandleFunc("/api/brain/ingest/run", brainIngestRunHandler)
	mux.HandleFunc("/api/brain/ingest/submit", brainIngestSubmitHandler) // section 1 roadmap
	mux.HandleFunc("/api/brain/ingest/batch", brainIngestBatchHandler)   // section 1 roadmap
	mux.HandleFunc("/api/brain/rescore", brainRescoreHandler)            // section 2 roadmap
	mux.HandleFunc("/api/brain/quality/check", brainQualityCheckHandler) // section 5 roadmap
	mux.HandleFunc("/api/brain/pii/strip", brainPIIStripHandler)         // section 3 roadmap
	mux.HandleFunc("/api/brain/injection/check", brainInjectionCheckHandler) // section 4 roadmap
	mux.HandleFunc("/api/mistakes/submit", brainMistakesSubmitHandler)       // section 7 roadmap
	mux.HandleFunc("/api/mistakes", brainMistakesListHandler)                // section 7 roadmap
	mux.HandleFunc("/api/brain/skills/list", brainSkillsListHandler)         // section 8 roadmap
	mux.HandleFunc("/api/brain/skills/get", brainSkillsGetHandler)           // section 8 roadmap
	mux.HandleFunc("/api/brain/tool-patterns/learn", brainToolLearnHandler)  // section 6 roadmap
	mux.HandleFunc("/api/brain/tool-patterns", brainToolSuggestHandler)      // section 6 roadmap
	mux.HandleFunc("/api/brain/models", brainModelsHandler)                  // section 11 roadmap
	mux.HandleFunc("/api/brain/models/get", brainModelsGetHandler)           // section 11 roadmap
	mux.HandleFunc("/api/brain/constitution/propose", brainProposeHandler)   // section 12 roadmap
	mux.HandleFunc("/api/brain/constitution/proposals", brainProposalsListHandler) // section 12 roadmap
	mux.HandleFunc("/api/brain/constitution/vote", brainVoteHandler)         // section 12 roadmap
	mux.HandleFunc("/api/brain/constitution/amend", brainAmendProposeHandler)        // section 12 phase 2 (P1 amend)
	mux.HandleFunc("/api/brain/constitution/amendments", brainAmendListHandler)      // section 12 phase 2 (P1 amend)
	mux.HandleFunc("/api/brain/constitution/amend/vote", brainAmendVoteHandler)      // section 12 phase 2 (P1 amend)
	// P2 A2 fase-2a: signed skill pack (gerbang #1 sign/provenance) + karma-gate publish (gerbang #3)
	mux.HandleFunc("/api/skills/pack/export-signed", skillPackExportSignedHandler)
	mux.HandleFunc("/api/skills/pack/verify", skillPackVerifyHandler)
	mux.HandleFunc("/api/skills/karma", skillKarmaListHandler)
	mux.HandleFunc("/api/skills/karma/record", skillKarmaRecordHandler)
	mux.HandleFunc("/api/skills/karma/endorse", skillKarmaEndorseHandler)
	// P2 A2 fase-2b: transport registry komunitas (browse/pull public, publish loopback+token)
	mux.HandleFunc("/api/skills/registry/status", skillRegistryStatusHandler)
	mux.HandleFunc("/api/skills/registry/browse", skillRegistryBrowseHandler)
	mux.HandleFunc("/api/skills/registry/pull", skillRegistryPullHandler)
	mux.HandleFunc("/api/skills/registry/publish", skillRegistryPublishHandler)
	mux.HandleFunc("/api/sensors/webhook", sensorsWebhookHandler)            // section 9 roadmap
	mux.HandleFunc("/api/recordings", func(w http.ResponseWriter, r *http.Request) {
		// route POST → post handler, GET → list handler
		if r.Method == http.MethodPost {
			recordingsPostHandler(w, r)
		} else {
			recordingsListHandler(w, r)
		}
	}) // section 10 roadmap
	mux.HandleFunc("/api/recordings/get", recordingsGetHandler) // section 10 roadmap
	mux.HandleFunc("/api/brain/search-drawers", brainSearchDrawersHandler) // flowork-kernel-compatible RAG
	mux.HandleFunc("/api/brain/instincts", brainInstinctsHandler)          // list insting (GUI), lintas-wing room LIKE instinct% — selaras injeksi
	mux.HandleFunc("/api/brain/init", brainInitHandler)                    // bootstrap empty Memory Palace DB
	mux.HandleFunc("/api/brain/drawer", brainAddDrawerHandler)             // bring-your-own-knowledge manual add
	mux.HandleFunc("/api/brain/immune/add", securityImmuneAddHandler)      // sync scan finding → immune_system (defensif)
	mux.HandleFunc("/api/brain/pentest/add", securityPentestAddHandler)    // sync scan finding → pentest_karma (ofensif)
	mux.HandleFunc("/api/brain/immune/delete", trackerDeleteByID("immune_system"))  // owner buang entry (false-positive)
	mux.HandleFunc("/api/brain/pentest/delete", trackerDeleteByID("pentest_karma")) // owner buang entry
	mux.HandleFunc("/api/brain/immune/list", securityImmuneListHandler)             // dashboard laporan (read-only)
	mux.HandleFunc("/api/brain/pentest/list", securityPentestListHandler)           // dashboard laporan (read-only)
	// Cross-device config sync
	mux.HandleFunc("/api/sync/export", syncExportHandler)
	mux.HandleFunc("/api/sync/import", syncImportHandler)
	mux.HandleFunc("/api/sync/pull", syncPullHandler)
	mux.HandleFunc("/api/locale", localeHandler)
	mux.HandleFunc("/api/locale/catalog", localeCatalogHandler)
	mux.HandleFunc("/api/init", initHandler)
	mux.HandleFunc("/api/shutdown", shutdownHandler)
	mux.HandleFunc("/api/version", versionHandler)
	mux.HandleFunc("/api/version/update", versionUpdateHandler)
	mux.HandleFunc("/api/version/shutdown", versionShutdownHandler)
}

// ── Infrastructure (cli-tools / tunnel / proxy-pools / mcp) ──────────────
func registerInfraRoutes(mux *http.ServeMux) {
	// CLI tools (13 integrations + cowork-mcp helpers)
	mux.HandleFunc("/api/cli-tools", cliToolsRouterHandler)
	mux.HandleFunc("/api/cli-tools/", cliToolsRouterHandler)
	// Tunnel (cloudflared + tailscale)
	mux.HandleFunc("/api/tunnel/status", tunnelStatusHandler)
	mux.HandleFunc("/api/tunnel/enable", tunnelEnableHandler)
	mux.HandleFunc("/api/tunnel/disable", tunnelDisableHandler)
	mux.HandleFunc("/api/tunnel/tailscale-check", tailscaleCheckHandler)
	mux.HandleFunc("/api/tunnel/tailscale-install", tailscaleInstallHandler)
	mux.HandleFunc("/api/tunnel/tailscale-enable", tailscaleEnableHandler)
	mux.HandleFunc("/api/tunnel/tailscale-disable", tailscaleDisableHandler)
	// Proxy pools + edge deploy
	mux.HandleFunc("/api/proxy-pools", proxyPoolsListAddHandler)
	mux.HandleFunc("/api/proxy-pools/cloudflare-deploy", cloudflareDeployHandler)
	mux.HandleFunc("/api/proxy-pools/deno-deploy", denoDeployHandler)
	mux.HandleFunc("/api/proxy-pools/vercel-deploy", vercelDeployHandler)
	mux.HandleFunc("/api/proxy-pools/", proxyPoolCRUDHandler)
	// MCP registry
	mux.HandleFunc("/api/mcp", mcpRouterHandler)
	mux.HandleFunc("/api/mcp/catalog", mcpCatalogHandler)
	mux.HandleFunc("/api/mcp/", mcpRouterHandler)
}

// ── Auth + OAuth import/flows ────────────────────────────────────────────
func registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/status", authStatusHandler)
	mux.HandleFunc("/api/auth/login", authLoginHandler)
	mux.HandleFunc("/api/auth/logout", authLogoutHandler)
	mux.HandleFunc("/api/auth/oidc", authOIDCHandler)
	mux.HandleFunc("/api/auth/oidc/init", authOIDCInitHandler)
	mux.HandleFunc("/api/auth/oidc/start", oidcStartHandler)
	mux.HandleFunc("/api/auth/oidc/test", oidcTestHandler)
	mux.HandleFunc("/api/auth/oidc/callback", authOIDCCallbackHandler)
	// OAuth imports + provider flows
	mux.HandleFunc("/api/oauth/imports", oauthImportsHandler)
	mux.HandleFunc("/api/oauth", oauthRouterHandler)
	mux.HandleFunc("/api/oauth/", oauthRouterHandler)
	// Per-device Claude login (independent refresh token; no Claude Code needed).
	mux.HandleFunc("/api/claude-login/start", claudeLoginStartHandler)
	mux.HandleFunc("/api/claude-login/complete", claudeLoginCompleteHandler)

	// Section 13 mesh foundation (phase 1: identity + peer registry).
	mux.HandleFunc("/api/mesh/identity", meshIdentityHandler)
	mux.HandleFunc("/api/mesh/peers", meshPeersHandler)
	mux.HandleFunc("/api/mesh/discover", meshDiscoverHandler)
	mux.HandleFunc("/api/mesh/peer", meshUpsertPeerHandler)
	mux.HandleFunc("/api/mesh/peer/block", meshBlockHandler)

	// Section 14-23 mesh stack phase 1 (schema only — single-owner no real mesh).
	mux.HandleFunc("/api/mesh/stack/overview", MeshStackOverviewHandler)
	// Section 14 phase 2: signed packet transport (receive + send + list).
	mux.HandleFunc("/api/mesh/packet", MeshPacketReceiveHandler)
	mux.HandleFunc("/api/mesh/packet/send", MeshPacketSendHandler)
	mux.HandleFunc("/api/mesh/packets", MeshPacketsHandler)

	// Section 16-23 phase 2: CRDT + knowledge + toolshare + karma + filter
	// + LoRA + L3 + daemon status.
	mux.HandleFunc("/api/mesh/crdt", MeshCRDTHandler)
	mux.HandleFunc("/api/mesh/knowledge", MeshKnowledgeHandler)
	mux.HandleFunc("/api/mesh/tool-manifests", MeshToolManifestsHandler)
	mux.HandleFunc("/api/mesh/karma", MeshKarmaHandler)
	mux.HandleFunc("/api/mesh/karma/decay", MeshKarmaDecayHandler)
	mux.HandleFunc("/api/mesh/filter/test", MeshFilterTestHandler)
	mux.HandleFunc("/api/mesh/lora-deltas", MeshLoraDeltasHandler)
	mux.HandleFunc("/api/mesh/l3", MeshL3Handler)
	mux.HandleFunc("/api/mesh/daemon/status", MeshDaemonStatusHandler)

	// Section 24-27 LLM provider + LocalAI + pricing + policy.
	mux.HandleFunc("/api/provider/chains", ProviderChainsHandler)
	mux.HandleFunc("/api/provider/calls", ProviderCallsHandler)
	mux.HandleFunc("/api/localai/models", LocalAIModelsHandler)
	mux.HandleFunc("/api/pricing/rules", PricingRulesHandler)
	mux.HandleFunc("/api/policy/budgets", PolicyBudgetsHandler)
	mux.HandleFunc("/api/policy/violations", PolicyViolationsHandler)
	mux.HandleFunc("/api/policy/tick", PolicyTickHandler)

	// Section 24-26 phase 2 runtime endpoints.
	mux.HandleFunc("/api/provider/chain/run", ChainRunHandler)
	mux.HandleFunc("/api/localai/runtime", LocalAIRuntimeHandler)
	mux.HandleFunc("/api/pricing/calc", PricingCalcHandler)
	mux.HandleFunc("/api/pricing/log_call", PricingLogCallHandler)
}

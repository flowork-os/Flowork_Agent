// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI (multi-tab): peta dok di lock/gui/README.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah endpoint TANPA buka frozen: pakai SEAM routes_ext.go (RegisterExtraRoute)
// + SWITCH internal/fwswitch/registry.go. Pola lengkap: lock/frozen-core.md

package main

import (
	"io/fs"
	"log"
	"net/http"
	"time"
)

func registerRoutes(mux *http.ServeMux) {
	registerStaticAndHealth(mux)
	registerChatRoutes(mux)
	registerProviderRoutes(mux)
	registerManagementRoutes(mux)
	registerInfraRoutes(mux)
	registerAuthRoutes(mux)
	registerExtraRoutes(mux)
}

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

func registerChatRoutes(mux *http.ServeMux) {

	mux.HandleFunc("/v1/chat/completions", chatCompletionsHandler)
	mux.HandleFunc("/v1/models", modelsHandler)

	mux.HandleFunc("/v1/messages", messagesV1Handler)
	mux.HandleFunc("/v1/responses", responsesV1Handler)

	mux.HandleFunc("/v1beta/models", v1betaModelsHandler)
	mux.HandleFunc("/v1beta/models/", v1betaGenerateContentHandler)

	mux.HandleFunc("/v1/embeddings", embeddingsV1Handler)
	mux.HandleFunc("/v1/images", imagesV1Handler)
	mux.HandleFunc("/v1/images/", imagesV1Handler)
	mux.HandleFunc("/v1/audio", audioV1Handler)
	mux.HandleFunc("/v1/audio/", audioV1Handler)
	mux.HandleFunc("/v1/search", searchV1Handler)
	mux.HandleFunc("/v1/web", webV1Handler)
	mux.HandleFunc("/v1/web/", webV1Handler)

	mux.HandleFunc("/v1/skills/", skillInvokeHandler)
	mux.HandleFunc("/v1/api/chat", apiChatHandler)
	mux.HandleFunc("/v1/api/", apiV1Handler)

	mux.HandleFunc("/v1", v1IndexHandler)
	mux.HandleFunc("/v1/messages/count_tokens", messagesCountTokensHandler)
	mux.HandleFunc("/v1/models/info", modelsInfoHandler)
	mux.HandleFunc("/v1/models/", modelsKindHandler)
	mux.HandleFunc("/v1/responses/compact", responsesCompactHandler)
	mux.HandleFunc("/v1/audio/voices", audioVoicesHandler)
}

func registerProviderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/providers", providersListAddHandler)
	mux.HandleFunc("/api/providers/validate", providerValidateHandler)
	mux.HandleFunc("/api/providers/suggested-models", providerSuggestedModelsHandler)
	mux.HandleFunc("/api/providers/test-batch", providerTestBatchHandler)
	mux.HandleFunc("/api/providers/client", providersClientHandler)
	mux.HandleFunc("/api/providers/kilo/free-models", providersKiloFreeModelsHandler)
	mux.HandleFunc("/api/providers/", providerCRUDHandler)

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

func registerManagementRoutes(mux *http.ServeMux) {

	mux.HandleFunc("/api/usage", usageHandler)
	mux.HandleFunc("/api/usage/", usageBreakdownRouter)
	mux.HandleFunc("/api/quota-tracker", quotaTrackerHandler)
	mux.HandleFunc("/api/quota-tracker/live", quotaLiveHandler)
	mux.HandleFunc("/api/kiro/models", kiroModelsHandler)
	mux.HandleFunc("/api/kiro/models/invalidate", kiroModelsInvalidateHandler)

	mux.HandleFunc("/api/mitm/capture-toggle", mitmCaptureToggleHandler)

	mux.HandleFunc("/api/learn/capture-toggle", learnCaptureToggleHandler)
	mux.HandleFunc("/api/localai/autostart-toggle", localAIAutostartToggleHandler)
	mux.HandleFunc("/api/mitm/full/", mitmFullDetailHandler)
	mux.HandleFunc("/api/mitm/recent-full", mitmRecentFullHandler)

	mux.HandleFunc("/api/mitm/status", mitmStatusHandler)
	mux.HandleFunc("/api/mitm/root-ca", mitmRootCADownloadHandler)
	mux.HandleFunc("/api/mitm/install-ca", mitmInstallCAHandler)
	mux.HandleFunc("/api/mitm/uninstall-ca", mitmUninstallCAHandler)
	mux.HandleFunc("/api/mitm/dns/add", mitmDNSAddHandler)
	mux.HandleFunc("/api/mitm/dns/remove", mitmDNSRemoveHandler)

	mux.HandleFunc("/api/mitm/start", mitmStartHandler)
	mux.HandleFunc("/api/mitm/stop", mitmStopHandler)

	mux.HandleFunc("/api/media-providers", mediaProvidersHandler)
	mux.HandleFunc("/api/media-providers/tts", mediaTTSHandler)
	mux.HandleFunc("/api/media-providers/tts/voices", ttsVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/deepgram/voices", deepgramVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/elevenlabs/voices", elevenlabsVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/inworld/voices", inworldVoicesHandler)
	mux.HandleFunc("/api/media-providers/tts/minimax/voices", minimaxVoicesHandler)
	mux.HandleFunc("/api/media-providers/", mediaProviderCRUDHandler)

	mux.HandleFunc("/api/skills", skillsListAddHandler)
	mux.HandleFunc("/api/skills/", skillCRUDHandler)
	mux.HandleFunc("/api/keys", apiKeysListAddHandler)
	mux.HandleFunc("/api/keys/", apiKeyCRUDHandler)
	mux.HandleFunc("/api/console-log", consoleLogHandler)

	mux.HandleFunc("/api/translator", translatorRouterHandler)
	mux.HandleFunc("/api/translator/", translatorRouterHandler)

	mux.HandleFunc("/api/settings", settingsHandler)
	mux.HandleFunc("/api/settings/database", settingsDatabaseHandler)
	mux.HandleFunc("/api/settings/backups", settingsBackupsHandler)
	mux.HandleFunc("/api/settings/proxy-test", settingsProxyTestHandler)
	mux.HandleFunc("/api/settings/require-login", settingsRequireLoginHandler)

	mux.HandleFunc("/api/brain/status", brainStatusHandler)
	mux.HandleFunc("/api/brain/config", brainConfigHandler)
	mux.HandleFunc("/api/brain/test", brainTestHandler)
	mux.HandleFunc("/api/brain/explore", brainExploreHandler)
	mux.HandleFunc("/api/brain/constitution", editionGate(brainConstitutionHandler))
	mux.HandleFunc("/api/brain/by-type", brainByTypeHandler)
	mux.HandleFunc("/api/brain/wing", brainWingHandler)
	mux.HandleFunc("/api/brain/graph/sync", dreamGraphSyncHandler)
	mux.HandleFunc("/api/brain/mem-types", brainMemTypesHandler)
	mux.HandleFunc("/api/brain/personas", editionGate(brainPersonasHandler))
	mux.HandleFunc("/api/brain/contributions", brainContributionsHandler)
	mux.HandleFunc("/api/brain/contributions/ingest", brainContributionsIngestHandler)
	mux.HandleFunc("/api/brain/ingest/run", brainIngestRunHandler)
	mux.HandleFunc("/api/brain/ingest/submit", brainIngestSubmitHandler)
	mux.HandleFunc("/api/brain/ingest/batch", brainIngestBatchHandler)
	mux.HandleFunc("/api/brain/rescore", brainRescoreHandler)
	mux.HandleFunc("/api/brain/quality/check", brainQualityCheckHandler)
	mux.HandleFunc("/api/brain/pii/strip", brainPIIStripHandler)
	mux.HandleFunc("/api/brain/injection/check", brainInjectionCheckHandler)
	mux.HandleFunc("/api/mistakes/submit", brainMistakesSubmitHandler)
	mux.HandleFunc("/api/mistakes", brainMistakesListHandler)
	mux.HandleFunc("/api/brain/skills/list", brainSkillsListHandler)
	mux.HandleFunc("/api/brain/skills/get", brainSkillsGetHandler)
	mux.HandleFunc("/api/brain/tool-patterns/learn", brainToolLearnHandler)
	mux.HandleFunc("/api/brain/tool-patterns", brainToolSuggestHandler)
	mux.HandleFunc("/api/brain/models", brainModelsHandler)
	mux.HandleFunc("/api/brain/models/get", brainModelsGetHandler)
	mux.HandleFunc("/api/brain/constitution/propose", editionGate(brainProposeHandler))
	mux.HandleFunc("/api/brain/constitution/proposals", brainProposalsListHandler)
	mux.HandleFunc("/api/brain/constitution/vote", editionGate(brainVoteHandler))
	mux.HandleFunc("/api/brain/constitution/amend", editionGate(brainAmendProposeHandler))
	mux.HandleFunc("/api/brain/constitution/amendments", brainAmendListHandler)
	mux.HandleFunc("/api/brain/constitution/amend/vote", editionGate(brainAmendVoteHandler))

	mux.HandleFunc("/api/skills/pack/export-signed", skillPackExportSignedHandler)
	mux.HandleFunc("/api/skills/pack/verify", skillPackVerifyHandler)
	mux.HandleFunc("/api/skills/karma", skillKarmaListHandler)
	mux.HandleFunc("/api/skills/karma/record", skillKarmaRecordHandler)
	mux.HandleFunc("/api/skills/karma/endorse", skillKarmaEndorseHandler)

	mux.HandleFunc("/api/skills/registry/status", skillRegistryStatusHandler)
	mux.HandleFunc("/api/skills/registry/browse", skillRegistryBrowseHandler)
	mux.HandleFunc("/api/skills/registry/pull", skillRegistryPullHandler)
	mux.HandleFunc("/api/skills/registry/publish", skillRegistryPublishHandler)
	mux.HandleFunc("/api/sensors/webhook", sensorsWebhookHandler)
	mux.HandleFunc("/api/recordings", func(w http.ResponseWriter, r *http.Request) {

		if r.Method == http.MethodPost {
			recordingsPostHandler(w, r)
		} else {
			recordingsListHandler(w, r)
		}
	})
	mux.HandleFunc("/api/recordings/get", recordingsGetHandler)
	mux.HandleFunc("/api/brain/search-drawers", brainSearchDrawersHandler)
	mux.HandleFunc("/api/brain/instincts", brainInstinctsHandler)
	mux.HandleFunc("/api/brain/init", brainInitHandler)
	mux.HandleFunc("/api/brain/drawer", brainAddDrawerHandler)
	mux.HandleFunc("/api/brain/immune/add", securityImmuneAddHandler)
	mux.HandleFunc("/api/brain/pentest/add", securityPentestAddHandler)
	mux.HandleFunc("/api/brain/immune/delete", trackerDeleteByID("immune_system"))
	mux.HandleFunc("/api/brain/pentest/delete", trackerDeleteByID("pentest_karma"))
	mux.HandleFunc("/api/brain/immune/list", securityImmuneListHandler)
	mux.HandleFunc("/api/brain/pentest/list", securityPentestListHandler)

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

func registerInfraRoutes(mux *http.ServeMux) {

	mux.HandleFunc("/api/cli-tools", cliToolsRouterHandler)
	mux.HandleFunc("/api/cli-tools/", cliToolsRouterHandler)

	mux.HandleFunc("/api/tunnel/status", tunnelStatusHandler)
	mux.HandleFunc("/api/tunnel/enable", tunnelEnableHandler)
	mux.HandleFunc("/api/tunnel/disable", tunnelDisableHandler)
	mux.HandleFunc("/api/tunnel/tailscale-check", tailscaleCheckHandler)
	mux.HandleFunc("/api/tunnel/tailscale-install", tailscaleInstallHandler)
	mux.HandleFunc("/api/tunnel/tailscale-enable", tailscaleEnableHandler)
	mux.HandleFunc("/api/tunnel/tailscale-disable", tailscaleDisableHandler)

	mux.HandleFunc("/api/proxy-pools", proxyPoolsListAddHandler)
	mux.HandleFunc("/api/proxy-pools/cloudflare-deploy", cloudflareDeployHandler)
	mux.HandleFunc("/api/proxy-pools/deno-deploy", denoDeployHandler)
	mux.HandleFunc("/api/proxy-pools/vercel-deploy", vercelDeployHandler)
	mux.HandleFunc("/api/proxy-pools/", proxyPoolCRUDHandler)

	mux.HandleFunc("/api/mcp", mcpRouterHandler)
	mux.HandleFunc("/api/mcp/catalog", mcpCatalogHandler)
	mux.HandleFunc("/api/mcp/", mcpRouterHandler)
}

func registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/status", authStatusHandler)
	mux.HandleFunc("/api/auth/login", authLoginHandler)
	mux.HandleFunc("/api/auth/logout", authLogoutHandler)
	mux.HandleFunc("/api/auth/oidc", authOIDCHandler)
	mux.HandleFunc("/api/auth/oidc/init", authOIDCInitHandler)
	mux.HandleFunc("/api/auth/oidc/start", oidcStartHandler)
	mux.HandleFunc("/api/auth/oidc/test", oidcTestHandler)
	mux.HandleFunc("/api/auth/oidc/callback", authOIDCCallbackHandler)

	mux.HandleFunc("/api/oauth/imports", oauthImportsHandler)
	mux.HandleFunc("/api/oauth", oauthRouterHandler)
	mux.HandleFunc("/api/oauth/", oauthRouterHandler)

	mux.HandleFunc("/api/claude-login/start", claudeLoginStartHandler)
	mux.HandleFunc("/api/claude-login/complete", claudeLoginCompleteHandler)

	mux.HandleFunc("/api/mesh/identity", meshIdentityHandler)
	mux.HandleFunc("/api/mesh/peers", meshPeersHandler)
	mux.HandleFunc("/api/mesh/discover", meshDiscoverHandler)
	mux.HandleFunc("/api/mesh/peer", meshUpsertPeerHandler)
	mux.HandleFunc("/api/mesh/peer/block", meshBlockHandler)

	mux.HandleFunc("/api/mesh/stack/overview", MeshStackOverviewHandler)

	mux.HandleFunc("/api/mesh/packet", MeshPacketReceiveHandler)
	mux.HandleFunc("/api/mesh/packet/send", MeshPacketSendHandler)
	mux.HandleFunc("/api/mesh/packets", MeshPacketsHandler)

	mux.HandleFunc("/api/mesh/crdt", MeshCRDTHandler)
	mux.HandleFunc("/api/mesh/knowledge", MeshKnowledgeHandler)
	mux.HandleFunc("/api/mesh/tool-manifests", MeshToolManifestsHandler)
	mux.HandleFunc("/api/mesh/karma", MeshKarmaHandler)
	mux.HandleFunc("/api/mesh/karma/decay", MeshKarmaDecayHandler)
	mux.HandleFunc("/api/mesh/filter/test", MeshFilterTestHandler)
	mux.HandleFunc("/api/mesh/lora-deltas", MeshLoraDeltasHandler)
	mux.HandleFunc("/api/mesh/l3", MeshL3Handler)
	mux.HandleFunc("/api/mesh/daemon/status", MeshDaemonStatusHandler)

	mux.HandleFunc("/api/provider/chains", ProviderChainsHandler)
	mux.HandleFunc("/api/provider/calls", ProviderCallsHandler)
	mux.HandleFunc("/api/localai/models", LocalAIModelsHandler)
	mux.HandleFunc("/api/pricing/rules", PricingRulesHandler)
	mux.HandleFunc("/api/policy/budgets", PolicyBudgetsHandler)
	mux.HandleFunc("/api/policy/violations", PolicyViolationsHandler)
	mux.HandleFunc("/api/policy/tick", PolicyTickHandler)

	mux.HandleFunc("/api/provider/chain/run", ChainRunHandler)
	mux.HandleFunc("/api/localai/runtime", LocalAIRuntimeHandler)
	mux.HandleFunc("/api/pricing/calc", PricingCalcHandler)
	mux.HandleFunc("/api/pricing/log_call", PricingLogCallHandler)
}

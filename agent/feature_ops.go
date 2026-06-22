// feature_ops.go — FASE-B: scanner (defensif), triggers (otomasi event→aksi), guardian
// (integritas), reaper (apoptosis app), market-data, mcp-config.
package main

import (
	"flowork-gui/internal/marketdata"
	"flowork-gui/internal/scanapi"
)

func init() {
	RegisterFeature(Feature{Name: "ops", Phase: PhaseRoute, Apply: func(d *Deps) {
		// REAPER (apoptosis).
		d.Mux.HandleFunc("/api/reaper/candidates", reaperCandidatesHandler(d.Host, d.FDB))
		d.Mux.HandleFunc("/api/reaper/reap", reaperReapHandler(d.FDB))
		// SCANNER allowlist + run + findings + registry + distill + packs.
		_ = d.FDB.EnsureScanSchema()
		d.Mux.HandleFunc("/api/scanner/allowlist", scanapi.ScannerAllowlistHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/allowlist/delete", scanapi.ScannerAllowlistDeleteHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/allowlist/check", scanapi.ScannerAllowlistCheckHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/run", scanapi.ScannerRunHandler(d.FDB, d.Host.OpenAgentStore))
		d.Mux.HandleFunc("/api/scanner/runs", scanapi.ScannerRunsHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/findings", scanapi.ScannerFindingsHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/findings/verify", scanapi.ScannerFindingVerifyHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/findings/triage", scanapi.ScannerTriageHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/findings/push", scanapi.ScannerPushHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/trackers", scanapi.ScannerTrackersHandler())
		d.Mux.HandleFunc("/api/scanner/registry", scanapi.ScannerRegistryHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/registry/toggle", scanapi.ScannerRegistryToggleHandler(d.FDB))
		d.Mux.HandleFunc("/api/scanner/checks/add", scanapi.ScannerCheckAddHandler())
		d.Mux.HandleFunc("/api/scanner/checks/delete", scanapi.ScannerCheckDeleteHandler())
		d.Mux.HandleFunc("/api/scanner/distill", scanapi.ScannerDistillHandler())
		d.Mux.HandleFunc("/api/scanner/distill/corpus", scanapi.ScannerDistillCorpusHandler())
		d.Mux.HandleFunc("/api/scanner/bodyscan", scanapi.ScannerBodyScanHandler(d.Host.OpenAgentStore))
		d.Mux.HandleFunc("/api/scanner/efficacy", scanapi.ScannerEfficacyHandler())
		d.Mux.HandleFunc("/api/scanner/packs/install", scanapi.ScannerPackInstallHandler())
		d.Mux.HandleFunc("/api/scanner/packs/uninstall", scanapi.ScannerPackUninstallHandler())
		d.Mux.HandleFunc("/api/scanner/packs/installed", scanapi.ScannerPacksInstalledHandler())
		// MARKET DATA proxy.
		d.Mux.HandleFunc("/api/market/quote", marketdata.QuoteHandler())
		// TRIGGER (event→aksi).
		d.Mux.HandleFunc("/api/triggers", triggersHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/delete", triggersDeleteHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/duplicate", triggersDuplicateHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/toggle", triggersToggleHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/run", triggersRunHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/runs", triggersRunsHandler(d.TrigEngine))
		d.Mux.HandleFunc("/api/triggers/types", triggersTypesHandler())
		d.Mux.HandleFunc("/api/triggers/hook/", triggersHookHandler(d.TrigEngine))
		// GUARDIAN.
		d.Mux.HandleFunc("/api/guardian/status", guardianStatusHandler())
		d.Mux.HandleFunc("/api/guardian/arm", guardianArmHandler())
		d.Mux.HandleFunc("/api/guardian/disarm", guardianDisarmHandler(d.AuthMgr))
		// MCP config (copy-paste ke AI eksternal).
		d.Mux.HandleFunc("/api/mcp/config", mcpConfigHandler)
	}})
}

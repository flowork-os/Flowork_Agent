// feature_dev.go — FASE-B: dev/creation routes (taskflow/category+schedule, plugin install,
// coder agent-generator, architect team-builder, tool-pack, slash-pack, skill export/import).
package main

func init() {
	RegisterFeature(Feature{Name: "dev", Phase: PhaseRoute, Apply: func(d *Deps) {
		// Category Task + scheduler looping.
		d.Mux.HandleFunc("/api/taskflow/run", taskflowRunHandler(d.Host, d.FDB))
		d.Mux.HandleFunc("/api/taskflow/categories", taskflowCategoriesHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/category", taskflowCategoryHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/category/delete", taskflowCategoryDeleteHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/runs", taskflowRunsHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/run-detail", taskflowRunDetailHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/schedules", taskflowSchedulesHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/schedule", taskflowScheduleAddHandler(d.FDB))
		d.Mux.HandleFunc("/api/taskflow/schedule/delete", taskflowScheduleDeleteHandler(d.FDB))
		// Plugin (.fwpack) install/uninstall/export/verify.
		d.Mux.HandleFunc("/api/plugins/install", pluginInstallHandler(d.Host, d.FDB))
		d.Mux.HandleFunc("/api/plugins/uninstall", pluginUninstallHandler(d.FDB))
		d.Mux.HandleFunc("/api/plugins/export", pluginExportHandler(d.FDB))
		d.Mux.HandleFunc("/api/plugins/verify", pluginVerifyHandler())
		// CODER (agent generator).
		d.Mux.HandleFunc("/api/coder/generate", coderGenerateHandler(d.Host))
		d.Mux.HandleFunc("/api/coder/pending", coderPendingHandler())
		d.Mux.HandleFunc("/api/coder/approve", coderApproveHandler(d.Host, d.FDB, d.GroupsAPI))
		d.Mux.HandleFunc("/api/coder/reject", coderRejectHandler())
		// ARCHITECT (team builder).
		d.Mux.HandleFunc("/api/architect/build", architectBuildHandler(d.Host, d.FDB, d.GroupsAPI))
		// TOOL-PACK + SLASH-PACK.
		d.Mux.HandleFunc("/api/tools/install", toolInstallHandler(d.Host))
		d.Mux.HandleFunc("/api/tools/uninstall", toolUninstallHandler())
		d.Mux.HandleFunc("/api/tools/installed", toolInstalledHandler())
		d.Mux.HandleFunc("/api/slash/install", slashInstallHandler(d.Host))
		d.Mux.HandleFunc("/api/slash/uninstall", slashUninstallHandler())
		d.Mux.HandleFunc("/api/slash/installed", slashInstalledHandler())
		// Skill export/import (.fwskill).
		d.Mux.HandleFunc("/api/skills/export", skillsExportHandler)
		d.Mux.HandleFunc("/api/skills/import", skillsImportHandler)
	}})
}

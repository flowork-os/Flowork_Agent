// feature_apps.go — FASE-B: APPS launcher (1 pintu human GUI & agent tool) + state + aset GUI
// (iframe sandbox) + izin-app per-agent.
package main

func init() {
	RegisterFeature(Feature{Name: "apps", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/apps", appsListHandler(d.AppsMgr))
		d.Mux.HandleFunc("/api/apps/op", appsOpHandler(d.AppsMgr))
		d.Mux.HandleFunc("/api/apps/install", appsInstallHandler())
		d.Mux.HandleFunc("/api/apps/uninstall", appsUninstallHandler())
		d.Mux.HandleFunc("/api/apps/stop", appsStopHandler(d.AppsMgr))
		d.Mux.HandleFunc("/api/agents/apps", appGrantsHandler(d.AppsMgr, d.Host)) // izin-app per-agent
		d.Mux.HandleFunc("/api/apps/state", appsStateHandler(d.AppsMgr))
		d.Mux.HandleFunc("/api/apps/", appsUIHandler(d.AppsMgr)) // /api/apps/<id>/ui/* (iframe sandbox)
	}})
}

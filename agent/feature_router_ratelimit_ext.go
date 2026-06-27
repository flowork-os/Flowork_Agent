// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// feature_router_ratelimit_ext.go — SEAM (NON-frozen, DELETABLE). Proxy same-origin biar GUI
// (:1987) bisa baca SADAR-KUOTA dari router (:2402) tanpa CORS. State kuota langganan dihitung
// di router (1 sumber share). GUI badge polling endpoint ini.
//   GET /api/router/ratelimit → {util_5h, surpassed_5h, util_7d, seen, ...}
package main

import (
	"io"
	"net/http"
)

func routerRateLimitProxyHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get("http://127.0.0.1:2402/api/router/ratelimit")
	if err != nil {
		tfWriteJSON(w, 0, map[string]any{"seen": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 64<<10))
}

func init() {
	RegisterFeature(Feature{Name: "router-ratelimit-proxy", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/router/ratelimit", routerRateLimitProxyHandler)
	}})
}

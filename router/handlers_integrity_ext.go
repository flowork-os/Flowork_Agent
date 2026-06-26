// handlers_integrity_ext.go — endpoint diagnostik integritas frozen-core, didaftar
// lewat seam routes_ext.go (RegisterExtraRoute) → NOL buka frozen. Bukti seam evolusi
// rute beneran kepake. GET /api/integrity → {clean, root_hash, checked}.
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package main

import (
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/mesh"
)

func init() {
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/integrity", integrityStatusHandler)
	})
}

func integrityStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	checked := mesh.CoreCheckedCount()
	writeJSON(w, http.StatusOK, map[string]any{
		"clean":     mesh.CoreClean(),
		"root_hash": mesh.CoreRootHash(),
		"checked":   checked,
		"verified":  checked > 0,
	})
}

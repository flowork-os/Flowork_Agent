// feature_edits_stats.go — SIBLING feature (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): endpoint
// /api/edits/stats → baris-diubah (added/removed) per-sesi hasil interceptor
// lines-changed (internal/tools/builtins/lines_changed_ext.go). Buat kartu "cost"
// lines-changed di GUI. Self-register PhaseRoute. 📄 Dok: lock/lines-changed.md
package main

import (
	"encoding/json"
	"net/http"

	"flowork-gui/internal/tools/builtins"
)

func init() {
	RegisterFeature(Feature{Name: "edits-stats", Phase: PhaseRoute, Apply: func(d *Deps) {
		if d.Mux == nil {
			return
		}
		d.Mux.HandleFunc("/api/edits/stats", func(w http.ResponseWriter, r *http.Request) {
			total, perAgent := builtins.EditStatsSnapshot()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total":    total,
				"byAgent":  perAgent,
				"scope":    "since-boot", // reset tiap restart = per-sesi
			})
		})
	}})
}

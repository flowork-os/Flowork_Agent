package guiapi

// codemap_tour.go — guided tour endpoint untuk Code Map.
//
// Per Ayah 2026-05-06: adopt fitur 2 dari "Understand-Anything" (Lum1104).
// Endpoint return ordered list TourStep — warga AI baru / Ayah pakai
// untuk learning path: file mana baca dulu, lanjut mana, kenapa.

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
)

// CodemapTourHandler GET /api/codemap/tour?max=15
// Return ordered tour steps untuk onboarding codebase.
func CodemapTourHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		// Pastikan layer column udah backfilled supaya tour bisa filter per layer.
		_ = codeindex.EnsureLayerColumn(brainDB)

		maxSteps := 15
		if v := r.URL.Query().Get("max"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
				maxSteps = n
			}
		}

		steps, err := codeindex.BuildTour(brainDB, maxSteps)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "tour generation failed", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"steps":     steps,
			"count":     len(steps),
			"max_steps": maxSteps,
		})
	}
}

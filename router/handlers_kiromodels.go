// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"net/http"
	"time"

	"github.com/flowork-os/flowork_Router/internal/kiromodels"
)

func kiroModelsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		token := r.URL.Query().Get("token")
		if token == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "token query param required (Kiro OAuth access_token)",
			})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cat, err := kiromodels.Fetch(ctx, kiromodels.Params{
			Token:      token,
			ProfileArn: r.URL.Query().Get("profileArn"),
			Region:     r.URL.Query().Get("region"),
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cat)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func kiroModelsInvalidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kiromodels.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{"invalidated": true})
}

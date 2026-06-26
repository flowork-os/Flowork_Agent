// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func pricingHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		provider := r.URL.Query().Get("provider")
		items, err := store.ListPricing(d, provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var p store.Pricing
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if p.Provider == "" || p.Model == "" {
			http.Error(w, "provider + model required", http.StatusBadRequest)
			return
		}
		if err := store.UpsertPricing(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	case http.MethodDelete:
		provider := r.URL.Query().Get("provider")
		model := r.URL.Query().Get("model")
		if provider == "" || model == "" {
			http.Error(w, "provider + model query required", http.StatusBadRequest)
			return
		}
		if err := store.DeletePricing(d, provider, model); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func pricingLookupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if provider == "" || model == "" {
		http.Error(w, "provider + model required", http.StatusBadRequest)
		return
	}
	p, err := store.GetPricing(d, provider, model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no rate card", "provider": provider, "model": model})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

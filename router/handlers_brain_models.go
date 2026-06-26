// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/modelpool"
)

const maxBrainModelBodyBytes = 16 * 1024

func brainModelsHandler(w http.ResponseWriter, r *http.Request) {
	if !ensureBrainReady(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		brainModelsPost(w, r)
	case http.MethodGet:
		brainModelsList(w, r)
	case http.MethodDelete:
		brainModelsDelete(w, r)
	default:
		http.Error(w, "method not allowed (POST/GET/DELETE)", http.StatusMethodNotAllowed)
	}
}

func brainModelsPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBrainModelBodyBytes)
	var body modelpool.UpsertOpts
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	id, isNew, err := modelpool.Upsert(r.Context(), body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           id,
		"added":        isNew,
		"algo_version": modelpool.AlgoVersion,
	})
}

func brainModelsList(w http.ResponseWriter, r *http.Request) {
	opts := modelpool.ListOpts{
		Category:   strings.TrimSpace(r.URL.Query().Get("category")),
		IsFreeOnly: r.URL.Query().Get("is_free") == "1",
	}
	if s := strings.TrimSpace(r.URL.Query().Get("max_cost")); s != "" {
		if f, perr := strconv.ParseFloat(s, 64); perr == nil && f > 0 {
			opts.MaxCost = f
		}
	}
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			opts.Limit = n
		}
	}
	items, err := modelpool.List(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func brainModelsDelete(w http.ResponseWriter, r *http.Request) {
	modelID := strings.TrimSpace(r.URL.Query().Get("id"))
	if modelID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	n, err := modelpool.Delete(r.Context(), modelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if n == 0 {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n, "model_id": modelID})
}

func brainModelsGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed (use GET)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	modelID := strings.TrimSpace(r.URL.Query().Get("id"))
	if modelID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	m, err := modelpool.Get(r.Context(), modelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if m.ModelName == "" {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

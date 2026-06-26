// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/ingest"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const maxBatchItems = 1000

const maxIngestBodyBytes = 16 << 20

func brainIngestSubmitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)

	var req ingest.Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	res := ingest.Submit(r.Context(), req)
	if res.Error != "" {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func brainIngestBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)

	var body struct {
		Items []ingest.Req `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Items) == 0 {
		http.Error(w, "items required (non-empty)", http.StatusBadRequest)
		return
	}
	if len(body.Items) > maxBatchItems {
		http.Error(w, "items > max batch size", http.StatusRequestEntityTooLarge)
		return
	}

	results := ingest.SubmitBatch(r.Context(), body.Items)
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":   ingest.Summarize(results),
		"results": results,
	})
}

func ensureBrainReady(w http.ResponseWriter, _ *http.Request) bool {
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		http.Error(w, "brain DB not available", http.StatusServiceUnavailable)
		return false
	}
	return true
}

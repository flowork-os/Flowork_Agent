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
)

const maxRescoreLimit = 5000

func brainRescoreHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	var body struct {
		Wing          string `json:"wing"`
		Limit         int    `json:"limit"`
		ForceOverride bool   `json:"force_override"`
	}

	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Limit < 0 || body.Limit > maxRescoreLimit {
		http.Error(w, "limit out of range (0 = default 1000, max 5000)", http.StatusBadRequest)
		return
	}

	report, err := brain.RescoreBatch(r.Context(), brain.RescoreOpts{
		Wing:          body.Wing,
		Limit:         body.Limit,
		ForceOverride: body.ForceOverride,
	}, ingest.Score)
	if err != nil {

		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":  err.Error(),
			"report": report,
		})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

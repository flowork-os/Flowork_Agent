// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/piistrip"
)

const maxPIIBodyBytes = 256 * 1024

func brainPIIStripHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPIIBodyBytes)

	var body struct {
		Content string `json:"content"`
		Quiet   bool   `json:"quiet"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if body.Quiet {
		cleaned, counts, total := piistrip.StripQuiet(body.Content)
		writeJSON(w, http.StatusOK, map[string]any{
			"algo_version": piistrip.AlgoVersion,
			"cleaned":      cleaned,
			"counts":       counts,
			"total":        total,
		})
		return
	}
	result := piistrip.Strip(body.Content)
	writeJSON(w, http.StatusOK, result)
}

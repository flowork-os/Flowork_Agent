// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/constitution"
)

const maxAmendBodyBytes = 32 * 1024

func amendErrStatus(err error) int {
	if errors.Is(err, constitution.ErrInvalidInput) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func brainAmendProposeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAmendBodyBytes)

	var body struct {
		TargetID     int64   `json:"target_id"`
		Kind         string  `json:"kind"`
		NewContent   string  `json:"new_content"`
		NewAmplitude float64 `json:"new_amplitude"`
		Rationale    string  `json:"rationale"`
		Signer       string  `json:"signer"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	id, err := constitution.ProposeAmendment(r.Context(), constitution.ProposeAmendOpts{
		TargetID:     body.TargetID,
		Kind:         body.Kind,
		NewContent:   body.NewContent,
		NewAmplitude: body.NewAmplitude,
		Rationale:    body.Rationale,
		Signer:       body.Signer,
	})
	if err != nil {
		writeJSON(w, amendErrStatus(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"amendment_id": id,
		"status":       "pending_owner_review",
		"algo_version": constitution.AmendAlgoVersion,
	})
}

func brainAmendListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed (use GET)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	items, err := constitution.ListAmendments(r.Context(), status, limit)
	if err != nil {
		writeJSON(w, amendErrStatus(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func brainAmendVoteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))
	id, perr := strconv.ParseInt(idStr, 10, 64)
	if perr != nil || id <= 0 {
		http.Error(w, "id required (positive int)", http.StatusBadRequest)
		return
	}
	approveParam := r.URL.Query().Get("approve")
	if approveParam != "1" && approveParam != "0" {
		http.Error(w, "approve must be 0 (reject) or 1 (approve)", http.StatusBadRequest)
		return
	}
	action := "approve"
	if approveParam == "0" {
		action = "reject"
	}
	voter := strings.TrimSpace(r.Header.Get("X-Voter-ID"))
	if voter == "" {
		voter = "anonymous"
	}
	result, err := constitution.VoteAmendment(r.Context(), constitution.AmendVoteOpts{
		AmendmentID: id,
		Action:      action,
		VoterID:     voter,
	})
	if err != nil {
		writeJSON(w, amendErrStatus(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

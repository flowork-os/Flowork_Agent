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

	"github.com/flowork-os/flowork_Router/internal/constitution"
)

const maxProposalBodyBytes = 32 * 1024

func brainProposeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProposalBodyBytes)

	var body constitution.ProposeOpts
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	id, err := constitution.Propose(r.Context(), body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"proposal_id":  id,
		"status":       "pending_quorum_review",
		"algo_version": constitution.AlgoVersion,
	})
}

func brainProposalsListHandler(w http.ResponseWriter, r *http.Request) {
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
	includeContent := r.URL.Query().Get("include_content") == "1"
	items, err := constitution.ListPending(r.Context(), limit, includeContent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":           items,
		"count":           len(items),
		"include_content": includeContent,
	})
}

func brainVoteHandler(w http.ResponseWriter, r *http.Request) {
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
	result, err := constitution.Vote(r.Context(), constitution.VoteOpts{
		ProposalID: id,
		Action:     action,
		VoterID:    voter,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

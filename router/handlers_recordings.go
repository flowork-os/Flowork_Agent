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

	"github.com/flowork-os/flowork_Router/internal/recorder"
)

const maxRecordingBodyBytes = 128 * 1024

func recordingsPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed (use POST)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingBodyBytes)

	var body struct {
		Model        string          `json:"model"`
		RequestBody  json.RawMessage `json:"request_body"`
		ResponseText string          `json:"response_text"`
		InputTokens  int64           `json:"input_tokens"`
		OutputTokens int64           `json:"output_tokens"`
		CostUSD      float64         `json:"cost_usd"`
		BuildPass    int64           `json:"build_pass"`
		ToolCalls    []any           `json:"tool_calls"`
		Agent        string          `json:"agent"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return
	}

	var reqBody any
	if len(body.RequestBody) > 0 {
		var tmp any
		if err := json.Unmarshal(body.RequestBody, &tmp); err == nil {
			reqBody = tmp
		}
	}

	id, err := recorder.Save(r.Context(), recorder.RecordOpts{
		Model:        body.Model,
		RequestBody:  reqBody,
		ResponseText: body.ResponseText,
		InputTokens:  body.InputTokens,
		OutputTokens: body.OutputTokens,
		CostUSD:      body.CostUSD,
		BuildPass:    body.BuildPass,
		ToolCalls:    body.ToolCalls,
		Agent:        body.Agent,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "algo_version": recorder.AlgoVersion})
}

func recordingsListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed (use GET)", http.StatusMethodNotAllowed)
		return
	}
	if !ensureBrainReady(w, r) {
		return
	}
	opts := recorder.ListOpts{
		Model:       strings.TrimSpace(r.URL.Query().Get("model")),
		Agent:       strings.TrimSpace(r.URL.Query().Get("agent")),
		IncludeBody: r.URL.Query().Get("include_body") == "1",
	}
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			opts.Limit = n
		}
	}
	items, err := recorder.List(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func recordingsGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed (use GET)", http.StatusMethodNotAllowed)
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
	rec, err := recorder.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if rec.Model == "" {
		http.Error(w, "recording not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

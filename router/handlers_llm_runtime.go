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
	"sync"

	"github.com/flowork-os/flowork_Router/internal/localai"
	"github.com/flowork-os/flowork_Router/internal/pricing"
	"github.com/flowork-os/flowork_Router/internal/provider"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func ChainRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	chainName := strings.TrimSpace(r.URL.Query().Get("chain"))
	if chainName == "" {
		chainName = "default"
	}
	caller := strings.TrimSpace(r.Header.Get("X-Caller-ID"))
	if caller == "" {
		caller = "anonymous"
	}
	var req provider.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	orch := provider.NewChainOrchestrator(db)
	resp, rerr := orch.Run(r.Context(), chainName, req)
	if rerr != nil {
		_ = pricing.LogCall(db, caller, "chain:"+chainName, req.Model, 0, 0, 0, 0, "error")
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": rerr.Error()})
		return
	}

	cost, _ := pricing.Calc(db, resp.Provider, resp.Model, "",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	_ = pricing.LogCall(db, caller, resp.Provider, resp.Model,
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens,
		cost, resp.LatencyMS, "success")

	w.Header().Set("X-Router-Cost-USD", strconv.FormatFloat(cost, 'f', 6, 64))
	w.Header().Set("X-Router-Provider", resp.Provider)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":   resp.Provider,
		"model":      resp.Model,
		"choices":    resp.Choices,
		"usage":      resp.Usage,
		"latency_ms": resp.LatencyMS,
		"cost_usd":   cost,
	})
}

var (
	localAIRuntimeRef *localai.Runtime

	localAIRuntimeMu sync.Mutex
)

func LocalAIRuntimeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Action    string `json:"action"`
		ModelName string `json:"model_name"`
		GGUFPath  string `json:"gguf_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	localAIRuntimeMu.Lock()
	defer localAIRuntimeMu.Unlock()
	if localAIRuntimeRef == nil {
		localAIRuntimeRef = localai.NewRuntime("", 0)
	}
	switch body.Action {
	case "start":

		gguf := body.GGUFPath
		if gguf == "" && body.ModelName != "" {
			db, _ := store.Open()
			_ = db.QueryRow(
				`SELECT gguf_path FROM localai_models WHERE model_name = ?`,
				body.ModelName).Scan(&gguf)
		}
		if err := localAIRuntimeRef.Start(body.ModelName, gguf); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": localAIRuntimeRef.Status()})
	case "stop":
		_ = localAIRuntimeRef.Stop()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "status", "":
		writeJSON(w, http.StatusOK, localAIRuntimeRef.Status())
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid action"})
	}
}

func PricingCalcHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Provider     string `json:"provider"`
		Model        string `json:"model"`
		Tier         string `json:"tier"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cost, cerr := pricing.Calc(db, body.Provider, body.Model, body.Tier, body.InputTokens, body.OutputTokens)
	if cerr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": cerr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cost_usd":      cost,
		"input_tokens":  body.InputTokens,
		"output_tokens": body.OutputTokens,
	})
}

func PricingLogCallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		Caller       string  `json:"caller"`
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
		LatencyMS    int64   `json:"latency_ms"`
		Status       string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if body.CostUSD == 0 && body.InputTokens+body.OutputTokens > 0 {
		body.CostUSD, _ = pricing.Calc(db, body.Provider, body.Model, "",
			body.InputTokens, body.OutputTokens)
	}
	if err := pricing.LogCall(db, body.Caller, body.Provider, body.Model,
		body.InputTokens, body.OutputTokens, body.CostUSD, body.LatencyMS, body.Status); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cost_usd": body.CostUSD})
}

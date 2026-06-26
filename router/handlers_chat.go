// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — HTTP handler.
// 2026-06-21 (owner-approved, Phase 3E/D13): +captureLearningRecording(resp,req,r) di jalur
//   non-stream sukses → rekam experience model kuat ke recordings (loop-belajar). Logic di
//   file BARU handlers_chat_learn.go, opt-in ROUTER_AUTO_RECORD=1 (default OFF), async, skip
//   model lokal. 1 baris call. Re-locked.

// Core Inference Handlers (chat + models).

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/safego"
	"github.com/flowork-os/flowork_Router/internal/store"
)

// chatCompletionsHandler — main dispatch entry. POST OpenAI-format,
// router routes to best matching provider (stream or non-stream).
func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req router.OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Claude-CLI bypass — short-circuit no-op patterns (Warmup/count/title/
	// isNewTopic/skip-patterns) with a local stub. Saves upstream tokens
	// and round-trip latency. Detector gates on User-Agent so other clients
	// pass through untouched.
	if tryClaudeCliBypass(w, r, &req) {
		return
	}

	// System-awareness: sisipin [STATUS_PC] (spek/OS/CPU/GPU/temp + WAKTU) ke tiap chat (switch GUI).
	InjectSystemStatus(&req)

	// Streaming branch — SSE relay.
	if req.Stream {
		status, _, err := router.DispatchChatCompletionStream(r.Context(), req, w)
		if err != nil && status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"type": "router_error", "message": err.Error()},
			})
		}
		return
	}

	start := time.Now()
	resp, status, err := router.DispatchChatCompletion(r.Context(), req)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		errBody := map[string]any{"error": map[string]any{"type": "router_error", "message": err.Error()}}
		raw, _ := json.Marshal(errBody)
		captureMITM(req.Model, r, body, status, err.Error(), durationMs, raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(errBody)
		return
	}
	respBody, _ := json.Marshal(resp)
	captureMITM(resp.Model, r, body, status, "", durationMs, respBody)
	captureLearningRecording(resp, req, r) // 3E/D13 auto-capture → recordings (opt-in ROUTER_AUTO_RECORD=1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

// captureMITM records full request/response bodies when MITM capture is on.
// No-op otherwise. Keeps the dispatch hot path clean.
func captureMITM(model string, r *http.Request, reqBody []byte, status int, errMsg string, durationMs int64, respBody []byte) {
	if !MITMCaptureEnabled() {
		return
	}
	safego.GoLabel("captureMITM", func() {
		recordMITMRequest("", model, r.RemoteAddr, r.UserAgent(), reqBody, status, errMsg, durationMs, respBody)
	})
}

// modelsHandler — aggregate semua models dari active providers (OpenAI shape).
func modelsHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	providers, _ := store.ListProviders(d)
	seen := map[string]bool{}
	var models []map[string]any
	for _, p := range providers {
		if !p.IsActive {
			continue
		}
		ms, _ := p.Data[store.CfgModels].([]any)
		for _, m := range ms {
			s, ok := m.(string)
			if !ok || s == "*" || s == "" {
				continue
			}
			if seen[s] {
				continue
			}
			// Hide models disabled for this provider (Models manager).
			if store.IsModelDisabled(d, p.Provider, s) || store.IsModelDisabled(d, p.ID, s) {
				continue
			}
			seen[s] = true
			models = append(models, map[string]any{
				"id": s, "object": "model", "owned_by": p.Provider, "provider": p.Name,
			})
		}
	}
	// Custom models (user-added) — discoverable + routable via provider pin.
	if customs, err := store.ListCustomModels(d); err == nil {
		for _, c := range customs {
			if c.Model == "" || seen[c.Model] {
				continue
			}
			seen[c.Model] = true
			models = append(models, map[string]any{"id": c.Model, "object": "model", "owned_by": "custom", "provider": c.DisplayName})
		}
	}
	// Aliases — list each alias as a usable model id.
	if aliases, err := store.ListModelAliases(d); err == nil {
		for _, a := range aliases {
			if a.Alias == "" || seen[a.Alias] {
				continue
			}
			seen[a.Alias] = true
			models = append(models, map[string]any{"id": a.Alias, "object": "model", "owned_by": "alias", "provider": a.Model})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
}

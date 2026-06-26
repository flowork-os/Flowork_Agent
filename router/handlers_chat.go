// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

	if tryClaudeCliBypass(w, r, &req) {
		return
	}

	InjectSystemStatus(&req)

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
	captureLearningRecording(resp, req, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

func captureMITM(model string, r *http.Request, reqBody []byte, status int, errMsg string, durationMs int64, respBody []byte) {
	if !MITMCaptureEnabled() {
		return
	}
	safego.GoLabel("captureMITM", func() {
		recordMITMRequest("", model, r.RemoteAddr, r.UserAgent(), reqBody, status, errMsg, durationMs, respBody)
	})
}

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

			if store.IsModelDisabled(d, p.Provider, s) || store.IsModelDisabled(d, p.ID, s) {
				continue
			}
			seen[s] = true
			models = append(models, map[string]any{
				"id": s, "object": "model", "owned_by": p.Provider, "provider": p.Name,
			})
		}
	}

	if customs, err := store.ListCustomModels(d); err == nil {
		for _, c := range customs {
			if c.Model == "" || seen[c.Model] {
				continue
			}
			seen[c.Model] = true
			models = append(models, map[string]any{"id": c.Model, "object": "model", "owned_by": "custom", "provider": c.DisplayName})
		}
	}

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

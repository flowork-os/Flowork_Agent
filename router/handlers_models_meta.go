// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Models → dok lock/gui/Models.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func modelsAliasHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListModelAliases(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var a store.ModelAlias
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if a.Alias == "" || a.Model == "" {
			http.Error(w, "alias + model required", http.StatusBadRequest)
			return
		}
		if err := store.UpsertModelAlias(d, &a); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, a)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func modelsAliasCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	alias := r.URL.Path[len("/api/models/alias/"):]
	if alias == "" {
		http.Error(w, "missing alias", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := store.DeleteModelAlias(d, alias); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func modelsAvailabilityHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListModelAvailability(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var a store.ModelAvailability
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.RecordAvailability(d, &a); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, a)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func modelsCustomHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListCustomModels(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var m store.ModelCustom
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if m.Model == "" {
			http.Error(w, "model required", http.StatusBadRequest)
			return
		}
		if err := store.UpsertCustomModel(d, &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func modelsCustomCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	id := r.URL.Path[len("/api/models/custom/"):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := store.DeleteCustomModel(d, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func modelsDisabledHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListDisabledModels(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var body struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.Provider == "" || body.Model == "" {
			http.Error(w, "provider + model required", http.StatusBadRequest)
			return
		}
		if err := store.DisableModel(d, body.Provider, body.Model, body.Reason); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, body)
	case http.MethodDelete:
		provider := r.URL.Query().Get("provider")
		model := r.URL.Query().Get("model")
		if provider == "" || model == "" {
			http.Error(w, "provider + model query required", http.StatusBadRequest)
			return
		}
		if err := store.EnableModel(d, provider, model); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func modelsTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Prompt   string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	prompt := body.Prompt
	if prompt == "" {
		prompt = "ping"
	}
	req := router.OpenAIRequest{
		Model: body.Model,
		Messages: []router.OpenAIMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 16,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	t0 := time.Now()
	_, status, err := router.DispatchChatCompletion(ctx, req)
	dur := time.Since(t0)
	d, _ := store.Open()
	availStatus := "up"
	errMsg := ""
	if err != nil {
		availStatus = "down"
		errMsg = err.Error()
	} else if status >= 400 {
		availStatus = "degraded"
		errMsg = http.StatusText(status)
	}
	_ = store.RecordAvailability(d, &store.ModelAvailability{
		Provider:     body.Provider,
		Model:        body.Model,
		Status:       availStatus,
		LatencyMs:    int(dur.Milliseconds()),
		ErrorMessage: errMsg,
	})
	resp := map[string]any{
		"provider":  body.Provider,
		"model":     body.Model,
		"status":    availStatus,
		"latencyMs": dur.Milliseconds(),
		"httpCode":  status,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	writeJSON(w, http.StatusOK, resp)
}

func modelsRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/models/")
	if rest == "" {
		modelsListHandler(w, r)
		return
	}

	if rest == "alias" {
		modelsAliasHandler(w, r)
		return
	}
	if strings.HasPrefix(rest, "alias/") {
		modelsAliasCRUDHandler(w, r)
		return
	}
	if rest == "availability" {
		modelsAvailabilityHandler(w, r)
		return
	}
	if rest == "custom" {
		modelsCustomHandler(w, r)
		return
	}
	if strings.HasPrefix(rest, "custom/") {
		modelsCustomCRUDHandler(w, r)
		return
	}
	if rest == "disabled" {
		modelsDisabledHandler(w, r)
		return
	}
	if rest == "test" {
		modelsTestHandler(w, r)
		return
	}
	http.Error(w, "unknown sub-route", http.StatusNotFound)
}

func modelsListHandler(w http.ResponseWriter, r *http.Request) {
	modelsHandler(w, r)
}

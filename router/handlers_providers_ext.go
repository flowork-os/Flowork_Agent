// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Providers → dok lock/gui/Providers.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/creds"
	"github.com/flowork-os/flowork_Router/internal/store"
)

var providerProbeClient = &http.Client{Timeout: 10 * time.Second}

func providerValidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Format  string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.BaseURL == "" {
		http.Error(w, "baseUrl required", http.StatusBadRequest)
		return
	}
	if _, gerr := blockMetadataURL(r.Context(), body.BaseURL); gerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "statusCode": 0, "detail": gerr.Error()})
		return
	}
	valid, status, detail := probeProvider(r.Context(), body.BaseURL, body.APIKey, body.Format)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      valid,
		"statusCode": status,
		"detail":     detail,
	})
}

func probeProvider(ctx context.Context, baseURL, apiKey, format string) (bool, int, string) {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	applyProbeAuth(req, apiKey, format)
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		return false, 0, "unreachable: " + err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return false, resp.StatusCode, "auth rejected"
	}

	return resp.StatusCode < 500, resp.StatusCode, "reachable"
}

func applyProbeAuth(req *http.Request, apiKey, format string) {
	if apiKey == "" {
		return
	}
	if format == "anthropic" {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func probeProviderConn(ctx context.Context, p *store.ProviderConnection) (bool, int, string) {
	baseURL, _ := p.Data[store.CfgBaseURL].(string)
	format, _ := p.Data[store.CfgFormat].(string)
	if baseURL == "" {
		return false, 0, "missing baseUrl"
	}

	if _, gerr := blockMetadataURL(ctx, baseURL); gerr != nil {
		return false, 0, gerr.Error()
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	switch p.AuthType {
	case store.AuthTypeNone:

	case store.AuthTypeAPIKey:
		apiKey, _ := p.Data[store.CfgAPIKey].(string)
		applyProbeAuth(req, apiKey, format)
	case store.AuthTypeSubscription:
		src, _ := p.Data[store.CfgTokenSource].(string)
		switch src {
		case "claude_credentials":
			c, err := creds.Load()
			if err != nil {
				return false, 0, "claude creds: " + err.Error()
			}
			if c.IsExpired() {
				return false, 401, "claude credentials expired — re-login Claude Code"
			}
			req.Header.Set("Authorization", "Bearer "+c.ClaudeAiOauth.AccessToken)
			req.Header.Set("anthropic-version", "2023-06-01")
		default:
			return false, 0, "unknown tokenSource: " + src
		}
	}
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		return false, 0, "unreachable: " + err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return false, resp.StatusCode, "auth rejected"
	}
	return resp.StatusCode < 500, resp.StatusCode, "reachable"
}

func providerSuggestedModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Format  string `json:"format"`
		Preset  string `json:"preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.BaseURL == "" {
		http.Error(w, "baseUrl required", http.StatusBadRequest)
		return
	}
	if _, gerr := blockMetadataURL(r.Context(), body.BaseURL); gerr != nil {
		http.Error(w, gerr.Error(), http.StatusBadRequest)
		return
	}
	endpoint := strings.TrimRight(body.BaseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	applyProbeAuth(req, body.APIKey, body.Format)
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	var parsed struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	_ = json.Unmarshal(raw, &parsed)
	models := parsed.Data
	if len(models) == 0 {
		models = parsed.Models
	}
	if len(models) == 0 {

		var bare []map[string]any
		if json.Unmarshal(raw, &bare) == nil {
			models = bare
		}
	}
	filtered := applyModelPreset(models, body.Preset)
	writeJSON(w, http.StatusOK, map[string]any{"data": filtered, "count": len(filtered), "preset": body.Preset})
}

func applyModelPreset(models []map[string]any, preset string) []map[string]any {
	var out []map[string]any
	switch preset {
	case "openrouter-free":
		for _, m := range models {
			pricing, _ := m["pricing"].(map[string]any)
			if pricing == nil {
				continue
			}
			prompt, _ := pricing["prompt"].(string)
			completion, _ := pricing["completion"].(string)
			ctx := toFloat(m["context_length"])
			if prompt == "0" && completion == "0" && ctx >= 200000 {
				out = append(out, map[string]any{
					"id":            m["id"],
					"name":          firstNonEmpty(m["name"], m["id"]),
					"contextLength": ctx,
				})
			}
		}
		sort.Slice(out, func(i, j int) bool {
			return toFloat(out[i]["contextLength"]) > toFloat(out[j]["contextLength"])
		})
	case "opencode-free":
		for _, m := range models {
			id, _ := m["id"].(string)
			if strings.HasSuffix(id, "-free") {
				out = append(out, map[string]any{"id": id, "name": id})
			}
		}
	default:
		for _, m := range models {
			out = append(out, map[string]any{
				"id":   m["id"],
				"name": firstNonEmpty(m["name"], m["id"]),
			})
		}
	}
	return out
}

func providerTestBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProviderIDs []string `json:"providerIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	d, _ := store.Open()

	if len(body.ProviderIDs) == 0 {
		all, _ := store.ListProviders(d)
		for _, p := range all {
			body.ProviderIDs = append(body.ProviderIDs, p.ID)
		}
	}
	type result struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Valid      bool   `json:"valid"`
		StatusCode int    `json:"statusCode"`
		Detail     string `json:"detail"`
		AuthType   string `json:"authType"`
	}
	results := make([]result, 0, len(body.ProviderIDs))
	for _, id := range body.ProviderIDs {
		p, err := store.GetProvider(d, id)
		if err != nil || p == nil {
			results = append(results, result{ID: id, Valid: false, Detail: "not found"})
			continue
		}
		valid, status, detail := probeProviderConn(r.Context(), p)
		results = append(results, result{
			ID: id, Name: p.Name, Valid: valid, StatusCode: status, Detail: detail,
			AuthType: string(p.AuthType),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)})
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func firstNonEmpty(vals ...any) any {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
		if v != nil {
			return v
		}
	}
	return ""
}

func init() { _ = fmt.Sprintf }

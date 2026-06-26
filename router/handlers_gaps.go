// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
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

func indexByte(s string, b byte) int { return strings.IndexByte(s, b) }

func providerSubActionHandler(w http.ResponseWriter, r *http.Request, id, action string) {
	d, _ := store.Open()
	p, err := store.GetProvider(d, id)
	if err != nil || p == nil {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}
	baseURL, _ := p.Data[store.CfgBaseURL].(string)
	apiKey, _ := p.Data[store.CfgAPIKey].(string)
	format, _ := p.Data[store.CfgFormat].(string)
	switch action {
	case "models":

		models := fetchProviderModels(r.Context(), baseURL, apiKey, format)
		writeJSON(w, http.StatusOK, map[string]any{"data": models, "count": len(models)})
	case "test":
		valid, status, detail := probeProviderConn(r.Context(), p)
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "valid": valid, "statusCode": status, "detail": detail})
	case "test-models":

		valid, _, _ := probeProviderConn(r.Context(), p)
		var results []map[string]any
		if ms, ok := p.Data[store.CfgModels].([]any); ok {
			for _, m := range ms {
				if s, ok := m.(string); ok && s != "" && s != "*" {
					results = append(results, map[string]any{"model": s, "reachable": valid})
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "results": results, "count": len(results)})
	default:
		http.Error(w, "unknown provider sub-action: "+action, http.StatusNotFound)
	}
}

func fetchProviderModels(ctx context.Context, baseURL, apiKey, format string) []map[string]any {
	if baseURL == "" {
		return nil
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	if _, verr := blockMetadataURL(ctx, endpoint); verr != nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, http.MethodGet, endpoint, nil)
	applyProbeAuth(req, apiKey, format)
	resp, err := providerProbeClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var parsed struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if len(parsed.Data) > 0 {
		return parsed.Data
	}
	return parsed.Models
}

func providersClientHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"baseURL":    "http://127.0.0.1:2402/v1",
		"apiKeyEnv":  "FLOW_ROUTER_API_KEY",
		"compatible": []string{"openai", "anthropic", "gemini"},
		"endpoints": map[string]string{
			"chat":      "/v1/chat/completions",
			"messages":  "/v1/messages",
			"responses": "/v1/responses",
			"models":    "/v1/models",
		},
	})
}

func providersKiloFreeModelsHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	providers, _ := store.ListProviders(d)
	var free []map[string]any
	for _, p := range providers {
		if !p.IsActive {
			continue
		}

		if p.AuthType != store.AuthTypeNone {
			continue
		}
		if ms, ok := p.Data[store.CfgModels].([]any); ok {
			for _, m := range ms {
				if s, ok := m.(string); ok && s != "" && s != "*" {
					free = append(free, map[string]any{"id": s, "name": s, "provider": p.Name, "free": true})
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": free, "count": len(free)})
}

func messagesCountTokensHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		System json.RawMessage `json:"system"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	chars := len(body.System)
	for _, m := range body.Messages {
		chars += len(m.Content)
	}
	writeJSON(w, http.StatusOK, map[string]any{"input_tokens": (chars / 4) + 1})
}

func modelsInfoHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	providers, _ := store.ListProviders(d)
	seen := map[string]bool{}
	var out []map[string]any
	for _, p := range providers {
		if !p.IsActive {
			continue
		}
		ms, _ := p.Data[store.CfgModels].([]any)
		for _, m := range ms {
			s, ok := m.(string)
			if !ok || s == "" || s == "*" || seen[s] {
				continue
			}
			seen[s] = true
			info := map[string]any{"id": s, "provider": p.Name, "owned_by": p.Provider}
			if pr, _ := store.GetPricing(d, p.Provider, s); pr != nil {
				info["pricing"] = map[string]any{"inputUsdPer1M": pr.InputUsdPer1M, "outputUsdPer1M": pr.OutputUsdPer1M}
			}
			out = append(out, info)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": out})
}

func modelsKindHandler(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimPrefix(r.URL.Path, "/v1/models/")
	if kind == "info" {
		modelsInfoHandler(w, r)
		return
	}
	d, _ := store.Open()
	var out []map[string]any

	switch kind {
	case "embedding", "text-to-image", "tts", "stt", "web-fetch-search":
		mps, _ := store.ListMediaProviders(d, kind)
		for _, mp := range mps {
			for _, m := range mp.Models {
				out = append(out, map[string]any{"id": m, "kind": kind, "provider": mp.Name})
			}
		}
	default:
		providers, _ := store.ListProviders(d)
		seen := map[string]bool{}
		for _, p := range providers {
			if !p.IsActive {
				continue
			}
			if ms, ok := p.Data[store.CfgModels].([]any); ok {
				for _, m := range ms {
					if s, ok := m.(string); ok && s != "" && s != "*" && !seen[s] {
						seen[s] = true
						out = append(out, map[string]any{"id": s, "kind": "chat", "provider": p.Name})
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "kind": kind, "data": out})
}

func responsesCompactHandler(w http.ResponseWriter, r *http.Request) {
	responsesV1Handler(w, r)
}

func audioVoicesHandler(w http.ResponseWriter, r *http.Request) {
	ttsVoicesHandler(w, r)
}

func apiChatHandler(w http.ResponseWriter, r *http.Request) {
	chatCompletionsHandler(w, r)
}

func v1IndexHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":  "flow_router",
		"dialects": []string{"openai", "anthropic", "gemini"},
		"endpoints": []string{
			"/v1/chat/completions", "/v1/messages", "/v1/responses",
			"/v1/models", "/v1beta/models", "/v1/embeddings", "/v1/images",
			"/v1/audio", "/v1/search", "/v1/web",
		},
	})
}

func tryFetchVoicesUpstream(parentCtx context.Context, w http.ResponseWriter, endpoint, apiKey string) bool {
	ctx, cancel := context.WithTimeout(parentCtx, 8*time.Second)
	defer cancel()

	if _, verr := blockMetadataURL(ctx, endpoint); verr != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := mediaHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = copyBody(w, resp)
	return true
}

func ttsVoicesHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	providers, _ := store.ListMediaProviders(d, store.MediaCategoryTTS)
	for i := range providers {
		if !providers[i].IsActive {
			continue
		}
		endpoint := strings.TrimRight(providers[i].BaseURL, "/") + "/audio/voices"

		if served := tryFetchVoicesUpstream(r.Context(), w, endpoint, providers[i].APIKey); served {
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"voices": []map[string]any{
			{"id": "alloy", "name": "Alloy"}, {"id": "echo", "name": "Echo"},
			{"id": "fable", "name": "Fable"}, {"id": "onyx", "name": "Onyx"},
			{"id": "nova", "name": "Nova"}, {"id": "shimmer", "name": "Shimmer"},
		},
		"source": "builtin-default",
	})
}

func oidcStartHandler(w http.ResponseWriter, r *http.Request) {
	authOIDCInitHandler(w, r)
}

func oidcTestHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	issuer, clientID, _, _, _ := oidcConfigFromSettings(settings)
	if issuer == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "issuer not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	disc, err := discoverOIDC(ctx, issuer)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "issuer": disc.Issuer,
		"authorizationEndpoint": disc.AuthorizationEndpoint,
		"tokenEndpoint":         disc.TokenEndpoint,
		"clientIdSet":           clientID != "",
	})
}

func proxyPoolTestHandler(w http.ResponseWriter, _ *http.Request, id string) {
	d, _ := store.Open()
	pools, _ := store.ListProxyPools(d)
	for _, p := range pools {
		if p.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{
				"id": id, "name": p.Name, "rotation": p.Rotation,
				"reachable": true, "note": "config present; live egress test Phase 3",
			})
			return
		}
	}
	http.Error(w, "proxy pool not found", http.StatusNotFound)
}

func copyBody(w http.ResponseWriter, resp *http.Response) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			m, werr := w.Write(buf[:n])
			total += int64(m)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			return total, nil
		}
	}
}

var _ = router.OpenAIRequest{}

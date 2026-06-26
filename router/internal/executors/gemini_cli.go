// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&geminiCLIExecutor{}) }

type geminiCLIExecutor struct{}

func (g *geminiCLIExecutor) Name() string { return "gemini-cli" }

func (g *geminiCLIExecutor) endpoint(p *store.ProviderConnection, stream bool) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://cloudcode-pa.googleapis.com/v1internal"
	}
	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}
	return trimRightSlash(base) + ":" + action
}

func (g *geminiCLIExecutor) headers(p *store.ProviderConnection, stream bool) map[string]string {
	h := map[string]string{
		"User-Agent":        "GeminiCLI/0.1.7",
		"X-Goog-Api-Client": "gl-node/22.10.0",
	}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	if stream {
		h["Accept"] = "text/event-stream"
	} else {
		h["Accept"] = "application/json"
	}
	return h
}

func (g *geminiCLIExecutor) body(p *store.ProviderConnection, req Request) []byte {
	contents := make([]map[string]any, len(req.Messages))
	for i, m := range req.Messages {
		contents[i] = map[string]any{"role": m.Role, "parts": []map[string]any{{"text": m.Content}}}
	}
	out := map[string]any{
		"project": ProviderString(p, "projectId"),
		"model":   req.Model,
		"request": map[string]any{"contents": contents},
	}
	b, _ := json.Marshal(out)
	return b
}

func (g *geminiCLIExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p, true), g.body(p, req), g.headers(p, true))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (g *geminiCLIExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p, false), g.body(p, req), g.headers(p, false))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

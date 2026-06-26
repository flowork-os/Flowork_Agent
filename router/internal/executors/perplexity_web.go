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

func init() { Register(&perplexityWebExecutor{}) }

type perplexityWebExecutor struct{}

func (p *perplexityWebExecutor) Name() string { return "perplexity-web" }

func (p *perplexityWebExecutor) endpoint(pc *store.ProviderConnection) string {
	base := ProviderString(pc, store.CfgBaseURL)
	if base == "" {
		base = "https://www.perplexity.ai/rest/sse/perplexity_ask"
	}
	return base
}

func (p *perplexityWebExecutor) headers(pc *store.ProviderConnection) map[string]string {
	h := map[string]string{
		"User-Agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
		"Content-Type": "application/json",
		"Accept":       "text/event-stream",
	}
	if cookie, ok := pc.Data["cookie"].(string); ok && cookie != "" {
		h["Cookie"] = cookie
	}
	return h
}

func (p *perplexityWebExecutor) body(req Request) []byte {
	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Content + "\n"
	}
	out := map[string]any{
		"query":            prompt,
		"sources":          []string{"web"},
		"language":         "en-US",
		"model_preference": req.Model,
	}
	b, _ := json.Marshal(out)
	return b
}

func (p *perplexityWebExecutor) Stream(ctx context.Context, pc *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, p.endpoint(pc), p.body(req), p.headers(pc))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (p *perplexityWebExecutor) NonStream(ctx context.Context, pc *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, p.endpoint(pc), p.body(req), p.headers(pc))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

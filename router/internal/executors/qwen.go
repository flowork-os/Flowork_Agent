// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&qwenExecutor{}) }

type qwenExecutor struct{}

func (q *qwenExecutor) Name() string { return "qwen" }

func (q *qwenExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://chat.qwen.ai/v1"
	}
	return trimRightSlash(base) + "/chat/completions"
}

func (q *qwenExecutor) headers(p *store.ProviderConnection) map[string]string {
	h := map[string]string{"Accept": "text/event-stream"}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	if src, ok := p.Data["source"].(string); ok && src != "" {
		h["source"] = src
	} else {
		h["source"] = "web"
	}
	return h
}

func (q *qwenExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, q.endpoint(p), body, q.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (q *qwenExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, q.endpoint(p), body, q.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

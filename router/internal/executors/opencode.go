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

func init() { Register(&opencodeExecutor{}) }

type opencodeExecutor struct{}

func (o *opencodeExecutor) Name() string { return "opencode" }

func (o *opencodeExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://api.opencode.ai/v1"
	}
	return trimRightSlash(base) + "/chat/completions"
}

func (o *opencodeExecutor) headers(p *store.ProviderConnection) map[string]string {
	h := map[string]string{}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

func (o *opencodeExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, o.endpoint(p), MarshalRequest(req), o.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (o *opencodeExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, o.endpoint(p), MarshalRequest(req), o.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

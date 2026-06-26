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

func init() {
	Register(&jetbrainsAIExecutor{name: "jetbrains_ai"})
	Register(&jetbrainsAIExecutor{name: "grazie"})
}

type jetbrainsAIExecutor struct{ name string }

func (j *jetbrainsAIExecutor) Name() string { return j.name }

func (j *jetbrainsAIExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://api.grazie.ai/api/v5"
	}
	return trimRightSlash(base) + "/chat/completions"
}

func (j *jetbrainsAIExecutor) headers(p *store.ProviderConnection) map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {

		h["Grazie-Authenticate-JWT"] = tok
	}
	return h
}

func (j *jetbrainsAIExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, j.endpoint(p), body, j.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (j *jetbrainsAIExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, j.endpoint(p), body, j.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

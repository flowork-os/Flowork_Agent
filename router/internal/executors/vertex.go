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

func init() {
	Register(&vertexExecutor{variant: "vertex"})
	Register(&vertexExecutor{variant: "vertex-partner"})
}

type vertexExecutor struct {
	variant string
}

func (v *vertexExecutor) Name() string { return v.variant }

func (v *vertexExecutor) endpoint(p *store.ProviderConnection, stream bool) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://us-central1-aiplatform.googleapis.com"
	}
	project := ProviderString(p, "projectId")
	if project == "" {
		project = "flow-router"
	}
	location, _ := p.Data["location"].(string)
	if location == "" {
		location = "us-central1"
	}
	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}

	publisherModel, _ := p.Data["publisherModel"].(string)
	return trimRightSlash(base) +
		"/v1/projects/" + project +
		"/locations/" + location +
		"/publishers/google/models/" + publisherModel +
		":" + action
}

func (v *vertexExecutor) headers(p *store.ProviderConnection, stream bool) map[string]string {
	h := map[string]string{"User-Agent": "vertex-ai-go/1.0"}
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

func (v *vertexExecutor) body(req Request) []byte {
	contents := make([]map[string]any, len(req.Messages))
	for i, m := range req.Messages {
		contents[i] = map[string]any{"role": m.Role, "parts": []map[string]any{{"text": m.Content}}}
	}
	b, _ := json.Marshal(map[string]any{"contents": contents})
	return b
}

func (v *vertexExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	if _, ok := p.Data["publisherModel"].(string); !ok {
		return Usage{}, 0, fmt.Errorf("vertex provider missing publisherModel in connection data")
	}
	httpReq, err := BuildRequest(ctx, http.MethodPost, v.endpoint(p, true), v.body(req), v.headers(p, true))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (v *vertexExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	if _, ok := p.Data["publisherModel"].(string); !ok {
		return nil, Usage{}, 0, fmt.Errorf("vertex provider missing publisherModel in connection data")
	}
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, v.endpoint(p, false), v.body(req), v.headers(p, false))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}

// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func init() { Register(&openaiProvider{}) }

type openaiProvider struct{}

func (o *openaiProvider) Name() string { return "openai" }

func (o *openaiProvider) Embed(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	payload := map[string]any{
		"model": defaultStr(req.Model, "text-embedding-3-small"),
		"input": req.Input,
	}
	if req.Dimensions > 0 {
		payload["dimensions"] = req.Dimensions
	}
	body, _ := json.Marshal(payload)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doEmbedRequest(r)
}

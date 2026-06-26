// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package image

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func init() { Register(&huggingfaceProvider{}) }

type huggingfaceProvider struct{}

func (h *huggingfaceProvider) Name() string { return "huggingface" }

func (h *huggingfaceProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	model := defaultStr(req.Model, "black-forest-labs/FLUX.1-schnell")
	base := req.BaseURL
	if base == "" {
		base = "https://api-inference.huggingface.co/models/" + model
	}
	body, _ := json.Marshal(map[string]any{
		"inputs": req.Prompt,
		"parameters": map[string]any{
			"negative_prompt": req.NegativePrompt,
		},
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doImageRequest(r)
}

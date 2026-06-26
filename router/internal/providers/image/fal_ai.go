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

func init() { Register(&falAiProvider{}) }

type falAiProvider struct{}

func (f *falAiProvider) Name() string { return "falAi" }

func (f *falAiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	model := defaultStr(req.Model, "fal-ai/flux/schnell")
	if base == "" {
		base = "https://fal.run/" + model
	}
	w, h := splitSize(req.Size, 1024)
	body, _ := json.Marshal(map[string]any{
		"prompt":     req.Prompt,
		"image_size": map[string]any{"width": w, "height": h},
		"num_images": defaultInt(req.N, 1),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Key "+req.APIKey)
	}
	return doImageRequest(r)
}

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

func init() { Register(&stabilityAiProvider{}) }

type stabilityAiProvider struct{}

func (s *stabilityAiProvider) Name() string { return "stabilityAi" }

func (s *stabilityAiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.stability.ai/v2beta"
	}
	body, _ := json.Marshal(map[string]any{
		"prompt":          req.Prompt,
		"negative_prompt": req.NegativePrompt,
		"aspect_ratio":    defaultStr(req.Size, "1:1"),
		"output_format":   "png",
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/stable-image/generate/sd3", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doImageRequest(r)
}

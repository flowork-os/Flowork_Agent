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

func init() { Register(&runwaymlProvider{}) }

type runwaymlProvider struct{}

func (r *runwaymlProvider) Name() string { return "runwayml" }

func (rw *runwaymlProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.runwayml.com/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"model":      defaultStr(req.Model, "gen3a_turbo"),
		"promptText": req.Prompt,
		"size":       defaultStr(req.Size, "1024x1024"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/text_to_image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Runway-Version", "2024-11-06")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doImageRequest(r)
}

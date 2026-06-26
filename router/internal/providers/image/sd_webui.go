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

func init() { Register(&sdwebuiProvider{}) }

type sdwebuiProvider struct{}

func (s *sdwebuiProvider) Name() string { return "sdwebui" }

func (s *sdwebuiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "http://127.0.0.1:7860"
	}
	w, h := splitSize(req.Size, 512)
	body, _ := json.Marshal(map[string]any{
		"prompt":          req.Prompt,
		"negative_prompt": req.NegativePrompt,
		"width":           w,
		"height":          h,
		"batch_size":      defaultInt(req.N, 1),
		"steps":           20,
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/sdapi/v1/txt2img", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	return doImageRequest(r)
}

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

func init() { Register(&nanobananaProvider{}) }

type nanobananaProvider struct{}

func (n *nanobananaProvider) Name() string { return "nanobanana" }

func (n *nanobananaProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.nanobanana.io/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"prompt":         req.Prompt,
		"negativePrompt": req.NegativePrompt,
		"size":           defaultStr(req.Size, "1024x1024"),
		"count":          defaultInt(req.N, 1),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/images/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doImageRequest(r)
}

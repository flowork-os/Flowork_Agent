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

func init() { Register(&cloudflareAiProvider{}) }

type cloudflareAiProvider struct{}

func (c *cloudflareAiProvider) Name() string { return "cloudflareAi" }

func (c *cloudflareAiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	account, _ := req.Extra["accountId"].(string)
	model := defaultStr(req.Model, "@cf/black-forest-labs/flux-1-schnell")
	if base == "" {
		base = "https://api.cloudflare.com/client/v4/accounts/" + account + "/ai/run/" + model
	}
	body, _ := json.Marshal(map[string]any{
		"prompt":          req.Prompt,
		"num_steps":       4,
		"negative_prompt": req.NegativePrompt,
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

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

func init() { Register(&geminiImageProvider{}) }

type geminiImageProvider struct{}

func (g *geminiImageProvider) Name() string { return "gemini" }

func (g *geminiImageProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	model := defaultStr(req.Model, "imagen-3.0-generate-001")
	url := base + "/models/" + model + ":predict?key=" + req.APIKey
	body, _ := json.Marshal(map[string]any{
		"instances": []map[string]any{{"prompt": req.Prompt}},
		"parameters": map[string]any{
			"sampleCount": defaultInt(req.N, 1),
		},
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	return doImageRequest(r)
}

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

func init() { Register(&comfyuiProvider{}) }

type comfyuiProvider struct{}

func (c *comfyuiProvider) Name() string { return "comfyui" }

func (c *comfyuiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "http://127.0.0.1:8188"
	}
	body, _ := json.Marshal(map[string]any{
		"prompt":   req.Prompt,
		"workflow": defaultStr(stringFromExtra(req.Extra, "workflow"), "default"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	return doImageRequest(r)
}

func stringFromExtra(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

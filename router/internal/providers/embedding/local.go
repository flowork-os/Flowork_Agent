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
	"os"
	"strings"
)

func init() { Register(&localProvider{}) }

type localProvider struct{}

func (localProvider) Name() string { return "local" }

func LocalEmbedBaseURL(override string) string {
	if s := strings.TrimSpace(override); s != "" {
		return strings.TrimRight(s, "/")
	}
	if s := strings.TrimSpace(os.Getenv("FLOWORK_LOCAL_EMBED_URL")); s != "" {
		return strings.TrimRight(s, "/")
	}
	return "http://127.0.0.1:11434/v1"
}

func DefaultLocalEmbedModel() string {
	if s := strings.TrimSpace(os.Getenv("FLOWORK_LOCAL_EMBED_MODEL")); s != "" {
		return s
	}
	return "bge-m3"
}

func (localProvider) Embed(ctx context.Context, req Request) (*Result, error) {
	base := LocalEmbedBaseURL(req.BaseURL)
	model := defaultStr(req.Model, DefaultLocalEmbedModel())
	payload := map[string]any{"model": model, "input": req.Input}
	body, _ := json.Marshal(payload)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")

	return doEmbedRequest(r)
}

// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package fetch

import (
	"context"
	"fmt"
	"net/http"
)

func init() { Register(&jinaProvider{}) }

type jinaProvider struct{}

func (j *jinaProvider) Name() string { return "jina" }

func (j *jinaProvider) Fetch(ctx context.Context, req Request) (Result, error) {
	if req.URL == "" {
		return Result{}, fmt.Errorf("jina: url required")
	}
	base := defaultStr(req.BaseURL, "https://r.jina.ai")
	endpoint := base + "/" + req.URL

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Accept", "text/markdown, text/plain, */*")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	body, resp, err := doHTTPRequest(httpReq)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("jina %d", resp.StatusCode)
	}
	return Result{
		URL:         req.URL,
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
		StatusCode:  resp.StatusCode,
	}, nil
}

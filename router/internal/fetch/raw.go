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

func init() { Register(&rawProvider{}) }

type rawProvider struct{}

func (r *rawProvider) Name() string { return "raw" }

func (r *rawProvider) Fetch(ctx context.Context, req Request) (Result, error) {
	if req.URL == "" {
		return Result{}, fmt.Errorf("raw: url required")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; flow_router/1.0)")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	body, resp, err := doHTTPRequest(httpReq)
	if err != nil {
		return Result{}, err
	}
	return Result{
		URL:         req.URL,
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
		StatusCode:  resp.StatusCode,
	}, nil
}

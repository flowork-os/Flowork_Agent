// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func init() { Register(&firecrawlProvider{}) }

type firecrawlProvider struct{}

func (f *firecrawlProvider) Name() string { return "firecrawl" }

func (f *firecrawlProvider) Fetch(ctx context.Context, req Request) (Result, error) {
	if req.URL == "" {
		return Result{}, fmt.Errorf("firecrawl: url required")
	}
	if req.APIKey == "" {
		return Result{}, fmt.Errorf("firecrawl: api key required")
	}

	formats := []string{"markdown"}
	if req.Mode == "html" {
		formats = []string{"html"}
	}
	body := map[string]any{
		"url":     req.URL,
		"formats": formats,
	}
	raw, _ := json.Marshal(body)

	endpoint := defaultStr(req.BaseURL, "https://api.firecrawl.dev/v1/scrape")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, resp, err := doHTTPRequest(httpReq)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("firecrawl %d: %s", resp.StatusCode, head(respBody))
	}

	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
			HTML     string `json:"html"`
			Metadata struct {
				Title string `json:"title"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("parse firecrawl: %w", err)
	}
	content := parsed.Data.Markdown
	ct := "text/markdown; charset=utf-8"
	if req.Mode == "html" {
		content = parsed.Data.HTML
		ct = "text/html; charset=utf-8"
	}
	return Result{
		URL:         req.URL,
		Title:       parsed.Data.Metadata.Title,
		Body:        []byte(content),
		ContentType: ct,
		StatusCode:  resp.StatusCode,
	}, nil
}

func head(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

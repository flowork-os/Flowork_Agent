// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package search

import (
	"context"
	"net/http"
	"net/url"
)

func init() { Register(&duckduckgoProvider{}) }

type duckduckgoProvider struct{}

func (d *duckduckgoProvider) Name() string { return "duckduckgo" }

func (d *duckduckgoProvider) Search(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.duckduckgo.com"
	}
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("format", "json")
	q.Set("no_html", "1")
	q.Set("skip_disambig", "1")
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	var resp struct {
		AbstractText  string `json:"AbstractText"`
		AbstractURL   string `json:"AbstractURL"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	if err := doRequest(r, &resp); err != nil {
		return nil, err
	}
	out := &Result{Provider: "duckduckgo", Query: req.Query}
	if resp.AbstractText != "" {
		out.Results = append(out.Results, SearchResult{
			Title: resp.Heading, URL: resp.AbstractURL, Snippet: resp.AbstractText,
		})
	}
	cap := defaultInt(req.MaxResults, 5)
	for i, t := range resp.RelatedTopics {
		if i >= cap {
			break
		}
		if t.FirstURL == "" {
			continue
		}
		out.Results = append(out.Results, SearchResult{Title: t.Text, URL: t.FirstURL, Snippet: t.Text})
	}
	return out, nil
}

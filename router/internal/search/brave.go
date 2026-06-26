// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package search

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func init() { Register(&braveProvider{}) }

type braveProvider struct{}

func (b *braveProvider) Name() string { return "brave" }

func (b *braveProvider) Search(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.search.brave.com/res/v1/web/search"
	}
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("count", strconv.Itoa(defaultInt(req.MaxResults, 5)))
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	if req.APIKey != "" {
		r.Header.Set("X-Subscription-Token", req.APIKey)
	}
	var resp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := doRequest(r, &resp); err != nil {
		return nil, err
	}
	out := &Result{Provider: "brave", Query: req.Query}
	for _, x := range resp.Web.Results {
		out.Results = append(out.Results, SearchResult{Title: x.Title, URL: x.URL, Snippet: x.Description})
	}
	return out, nil
}

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

func init() { Register(&serpapiProvider{}) }

type serpapiProvider struct{}

func (s *serpapiProvider) Name() string { return "serpapi" }

func (s *serpapiProvider) Search(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://serpapi.com/search.json"
	}
	q := url.Values{}
	q.Set("q", req.Query)
	q.Set("api_key", req.APIKey)
	q.Set("engine", "google")
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := doRequest(r, &resp); err != nil {
		return nil, err
	}
	out := &Result{Provider: "serpapi", Query: req.Query}
	cap := defaultInt(req.MaxResults, 5)
	for i, x := range resp.OrganicResults {
		if i >= cap {
			break
		}
		out.Results = append(out.Results, SearchResult{Title: x.Title, URL: x.Link, Snippet: x.Snippet})
	}
	return out, nil
}

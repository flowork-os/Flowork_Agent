// brain_search.go — Section 11 phase 1e extension: brain search method.
//
// Reuse existing locked Client (routerclient.go). Add new method untuk
// query brain drawers via Router /api/brain/search-drawers.
//
// Source: Flowork_Agent/roadmap.md Section 11 phase 1e + Router existing
// brainSearchDrawersHandler (handlers_brain_views.go).

package routerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// DrawerHit — single search result. Mirror Router response shape.
type DrawerHit struct {
	Wing     string  `json:"wing"`
	Room     string  `json:"room"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	DrawerID string  `json:"drawer_id"`
}

// SearchBrainResp — full response.
type SearchBrainResp struct {
	Query string      `json:"query"`
	Hits  []DrawerHit `json:"hits"`
	Count int         `json:"count"`
}

// SearchBrain — GET /api/brain/search-drawers?query=&k= ke Router.
// k default 5, max 20. Return DrawerHit slice via Router's BM25/FTS rank.
func (c *Client) SearchBrain(ctx context.Context, query string, k int) (SearchBrainResp, error) {
	if c == nil {
		return SearchBrainResp{}, fmt.Errorf("router client nil")
	}
	if query == "" {
		return SearchBrainResp{}, fmt.Errorf("query required")
	}
	if k <= 0 {
		k = 5
	}
	if k > 20 {
		k = 20
	}

	u := c.BaseURL + "/api/brain/search-drawers?query=" +
		url.QueryEscape(query) + "&k=" + strconv.Itoa(k)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return SearchBrainResp{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return SearchBrainResp{}, fmt.Errorf("search brain: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SearchBrainResp{}, fmt.Errorf("router status %d", resp.StatusCode)
	}

	// Body cap 512KB — 20 hit × 32KB max should fit.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	var out SearchBrainResp
	if uerr := json.Unmarshal(body, &out); uerr != nil {
		return SearchBrainResp{}, fmt.Errorf("decode: %w", uerr)
	}
	return out, nil
}

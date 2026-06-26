// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package quotalive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() {
	Register(&minimaxFetcher{
		name: "minimax",
		urls: []string{
			"https://www.minimax.io/v1/token_plan/remains",
			"https://api.minimax.chat/v1/token_plan/remains",
		},
	})
	Register(&minimaxFetcher{
		name: "minimax-cn",
		urls: []string{
			"https://api.minimaxi.com/v1/token_plan/remains",
		},
	})
}

type minimaxFetcher struct {
	name string
	urls []string
}

func (m *minimaxFetcher) Name() string { return m.name }

func (m *minimaxFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("%s: api key required", m.name)
	}
	var lastErr error
	for _, url := range m.urls {
		snap, err := m.tryOne(ctx, p.Token, url)
		if err == nil {
			snap.Provider = m.name
			return snap, nil
		}
		lastErr = err
	}
	return Snapshot{}, fmt.Errorf("%s: all endpoints failed: %v", m.name, lastErr)
}

func (m *minimaxFetcher) tryOne(ctx context.Context, token, url string) (Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("%d: %s", resp.StatusCode, snip(body))
	}

	var parsed struct {
		Data struct {
			Remaining float64 `json:"remaining"`
			Total     float64 `json:"total"`
			Plan      string  `json:"plan,omitempty"`
		} `json:"data,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Snapshot{}, err
	}
	used := parsed.Data.Total - parsed.Data.Remaining
	if used < 0 {
		used = 0
	}
	rp := 0.0
	if parsed.Data.Total > 0 {
		rp = (parsed.Data.Remaining / parsed.Data.Total) * 100
	}
	return Snapshot{
		Plan:      parsed.Data.Plan,
		FetchedAt: time.Now().UTC(),
		Raw:       body,
		Windows: []Window{{
			Label:            "tokens",
			Used:             used,
			Total:            parsed.Data.Total,
			Remaining:        parsed.Data.Remaining,
			RemainingPercent: rp,
			Unit:             "tokens",
		}},
	}, nil
}

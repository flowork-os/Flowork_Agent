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
	Register(&glmFetcher{name: "glm", url: "https://api.z.ai/api/monitor/usage/quota/limit"})
	Register(&glmFetcher{name: "glm-cn", url: "https://open.bigmodel.cn/api/monitor/usage/quota/limit"})
}

type glmFetcher struct{ name, url string }

func (g *glmFetcher) Name() string { return g.name }

func (g *glmFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("%s: api key required", g.name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("%s %d: %s", g.name, resp.StatusCode, snip(body))
	}

	var parsed struct {
		Data struct {
			Quotas []struct {
				Model     string  `json:"model"`
				Used      float64 `json:"used"`
				Total     float64 `json:"total"`
				ResetTime string  `json:"resetTime,omitempty"`
			} `json:"quotas,omitempty"`
		} `json:"data,omitempty"`
		Plan string `json:"plan,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{
		Provider:  g.name,
		Plan:      parsed.Plan,
		FetchedAt: time.Now().UTC(),
		Raw:       body,
	}
	for _, q := range parsed.Data.Quotas {
		remaining := q.Total - q.Used
		if remaining < 0 {
			remaining = 0
		}
		rp := 0.0
		if q.Total > 0 {
			rp = (remaining / q.Total) * 100
		}
		win := Window{
			Label:            q.Model,
			Used:             q.Used,
			Total:            q.Total,
			Remaining:        remaining,
			RemainingPercent: rp,
			Unit:             "tokens",
		}
		if q.ResetTime != "" {
			if t, err := time.Parse(time.RFC3339, q.ResetTime); err == nil {
				win.ResetAt = t
			}
		}
		snap.Windows = append(snap.Windows, win)
	}
	return snap, nil
}

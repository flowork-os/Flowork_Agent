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
	"strings"
	"time"
)

func init() { Register(&claudeFetcher{}) }

type claudeFetcher struct{}

func (c *claudeFetcher) Name() string { return "claude" }

type claudeWindow struct {
	Utilization float64 `json:"utilization,omitempty"`
	ResetsAt    string  `json:"resets_at,omitempty"`
}

func (c *claudeFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("claude: oauth access token required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("claude oauth/usage %d: %s", resp.StatusCode, snip(body))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{Provider: "claude", FetchedAt: time.Now().UTC(), Raw: body}
	addWindow := func(label string, w *claudeWindow) {
		if w == nil {
			return
		}
		used := w.Utilization
		remaining := 100 - used
		if remaining < 0 {
			remaining = 0
		}
		win := Window{
			Label:            label,
			Used:             used,
			Total:            100,
			Remaining:        remaining,
			RemainingPercent: remaining,
			Unit:             "percent",
		}
		if w.ResetsAt != "" {
			if t, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
				win.ResetAt = t
			}
		}
		snap.Windows = append(snap.Windows, win)
	}

	parseWindow := func(v any) *claudeWindow {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		w := &claudeWindow{}
		if u, ok := m["utilization"].(float64); ok {
			w.Utilization = u
		} else {
			return nil
		}
		if s, ok := m["resets_at"].(string); ok {
			w.ResetsAt = s
		}
		return w
	}

	if w := parseWindow(raw["five_hour"]); w != nil {
		addWindow("session (5h)", w)
	}
	if w := parseWindow(raw["seven_day"]); w != nil {
		addWindow("weekly (7d)", w)
	}
	for k, v := range raw {
		if !strings.HasPrefix(k, "seven_day_") || k == "seven_day" {
			continue
		}
		if w := parseWindow(v); w != nil {
			addWindow("weekly "+strings.TrimPrefix(k, "seven_day_")+" (7d)", w)
		}
	}

	if plan, ok := raw["plan"].(string); ok {
		snap.Plan = plan
	}
	return snap, nil
}

func snip(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

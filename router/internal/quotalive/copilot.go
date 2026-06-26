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

func init() { Register(&copilotFetcher{}) }

type copilotFetcher struct{}

func (c *copilotFetcher) Name() string { return "copilot" }

func (c *copilotFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("copilot: github oauth token required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/copilot_internal/user", nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "token "+p.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "flow_router/1.0")
	req.Header.Set("Editor-Version", "vscode/1.100.0")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("copilot %d: %s", resp.StatusCode, snip(body))
	}

	var parsed struct {
		QuotaResetDate string                     `json:"quota_reset_date,omitempty"`
		QuotaSnapshots map[string]copilotSnapshot `json:"quota_snapshots,omitempty"`
		CopilotPlan    string                     `json:"copilot_plan,omitempty"`
		AccessTypeSku  string                     `json:"access_type_sku,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{
		Provider:  "copilot",
		Plan:      firstNonEmpty(parsed.CopilotPlan, parsed.AccessTypeSku),
		FetchedAt: time.Now().UTC(),
		Raw:       body,
	}

	resetAt, _ := parseFlexibleTime(parsed.QuotaResetDate)
	for label, q := range parsed.QuotaSnapshots {
		used := q.EntitlementUsed
		total := q.Entitlement
		remaining := total - used
		if remaining < 0 {
			remaining = 0
		}
		rp := 0.0
		if total > 0 {
			rp = (remaining / total) * 100
		}
		win := Window{
			Label:            label,
			Used:             used,
			Total:            total,
			Remaining:        remaining,
			RemainingPercent: rp,
			Unlimited:        q.Unlimited,
			ResetAt:          resetAt,
			Unit:             "requests",
		}
		snap.Windows = append(snap.Windows, win)
	}
	return snap, nil
}

type copilotSnapshot struct {
	Entitlement     float64 `json:"entitlement"`
	EntitlementUsed float64 `json:"entitlement_used"`
	Unlimited       bool    `json:"unlimited,omitempty"`
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseFlexibleTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", time.RFC1123} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparsed time %q", s)
}

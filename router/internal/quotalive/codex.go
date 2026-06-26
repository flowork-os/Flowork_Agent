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

func init() { Register(&codexFetcher{}) }

type codexFetcher struct{}

func (c *codexFetcher) Name() string { return "codex" }

func (c *codexFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("codex: bearer token required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
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
		return Snapshot{}, fmt.Errorf("codex %d: %s", resp.StatusCode, snip(body))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{
		Provider:  "codex",
		Plan:      pickCodexPlan(raw),
		FetchedAt: time.Now().UTC(),
		Raw:       body,
	}
	appendCodexWindows(&snap, "", raw)
	if review := findCodexReviewRateLimit(raw); review != nil {
		appendCodexWindows(&snap, "review", review)
	}
	return snap, nil
}

func pickCodexPlan(raw map[string]any) string {
	if s, _ := raw["plan_type"].(string); s != "" {
		return s
	}
	if summary, ok := raw["summary"].(map[string]any); ok {
		if s, _ := summary["plan"].(string); s != "" {
			return s
		}
	}
	return ""
}

func getCodexRateLimitBody(snapshot map[string]any) map[string]any {
	if rl, ok := snapshot["rate_limit"].(map[string]any); ok {
		return rl
	}
	return snapshot
}

func findCodexReviewRateLimit(raw map[string]any) map[string]any {
	if v, ok := raw["code_review_rate_limit"].(map[string]any); ok {
		return v
	}
	if v, ok := raw["review_rate_limit"].(map[string]any); ok {
		return v
	}
	if byID, ok := raw["rate_limits_by_limit_id"].(map[string]any); ok {
		for _, key := range []string{"code_review", "codex_review", "review"} {
			if v, ok := byID[key].(map[string]any); ok {
				return v
			}
		}
	}
	if extra, ok := raw["additional_rate_limits"].([]any); ok {
		for _, e := range extra {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			label := strings.ToLower(strings.TrimSpace(firstStringField(
				entry, "limit_name", "metered_feature", "id",
			)))
			if label == "" {
				continue
			}
			if label == "code_review" || label == "codex_review" || label == "review" || strings.Contains(label, "review") {
				return entry
			}
		}
	}
	return nil
}

func appendCodexWindows(snap *Snapshot, prefix string, snapshot map[string]any) {
	if snapshot == nil {
		return
	}
	rate := getCodexRateLimitBody(snapshot)
	primary := pickCodexWindow(rate, snapshot, "primary_window", "primary")
	secondary := pickCodexWindow(rate, snapshot, "secondary_window", "secondary")

	if primary != nil {
		label := "session"
		if prefix != "" {
			label = prefix + "_session"
		}
		snap.Windows = append(snap.Windows, formatCodexWindow(label, primary))
	}
	if secondary != nil {
		label := "weekly"
		if prefix != "" {
			label = prefix + "_weekly"
		}
		snap.Windows = append(snap.Windows, formatCodexWindow(label, secondary))
	}
}

func pickCodexWindow(rate, snapshot map[string]any, keyA, keyB string) map[string]any {
	for _, src := range []map[string]any{rate, snapshot} {
		if src == nil {
			continue
		}
		for _, k := range []string{keyA, keyB} {
			if v, ok := src[k].(map[string]any); ok {
				return v
			}
		}
	}
	return nil
}

func formatCodexWindow(label string, window map[string]any) Window {
	used := toCodexNumber(window["used_percent"])
	if used == 0 {
		used = toCodexNumber(window["percent_used"])
	}
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	remaining := 100 - used
	w := Window{
		Label:            label,
		Used:             used,
		Total:            100,
		Remaining:        remaining,
		RemainingPercent: remaining,
		Unit:             "percent",
	}
	for _, k := range []string{"reset_at", "resets_at", "resetAt"} {
		if v, ok := window[k]; ok {
			if t, ok := parseCodexResetTime(v); ok {
				w.ResetAt = t
				break
			}
		}
	}
	return w
}

func parseCodexResetTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return time.Time{}, false
		}
		if t < 1e12 {
			return time.Unix(int64(t), 0).UTC(), true
		}
		return time.Unix(0, int64(t)*int64(time.Millisecond)).UTC(), true
	case string:
		if t == "" {
			return time.Time{}, false
		}
		if isAllDigits(t) {
			n := atoi64(t)
			if n < 1e12 {
				return time.Unix(n, 0).UTC(), true
			}
			return time.Unix(0, n*int64(time.Millisecond)).UTC(), true
		}
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func atoi64(s string) int64 {
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	return n
}

func firstStringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, _ := m[k].(string); v != "" {
			return v
		}
	}
	return ""
}

func toCodexNumber(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if isAllDigits(t) {
			return float64(atoi64(t))
		}
	}
	return 0
}

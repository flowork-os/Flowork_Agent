// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package quotalive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() { Register(&antigravityFetcher{}) }

type antigravityFetcher struct{}

func (a *antigravityFetcher) Name() string { return "antigravity" }

func (a *antigravityFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("antigravity: bearer token required")
	}
	projectID, _ := p.Extra["projectId"].(string)

	body := map[string]any{}
	if projectID != "" {
		body["project"] = projectID
	}
	raw, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://cloudcode-pa.googleapis.com/v1internal:countQuotaUsage",
		bytes.NewReader(raw))
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "google-cloud-code-assist/1.16.0")
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", "1.107.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("antigravity %d: %s", resp.StatusCode, snip(respBody))
	}

	var parsed struct {
		Quotas []struct {
			Name      string  `json:"name"`
			Used      float64 `json:"used"`
			Total     float64 `json:"total"`
			Remaining float64 `json:"remaining"`
			ResetAt   string  `json:"resetAt,omitempty"`
		} `json:"quotas,omitempty"`
		CurrentTier struct {
			Name string `json:"name,omitempty"`
		} `json:"currentTier,omitempty"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{
		Provider:  "antigravity",
		Plan:      parsed.CurrentTier.Name,
		FetchedAt: time.Now().UTC(),
		Raw:       respBody,
	}
	for _, q := range parsed.Quotas {
		rp := 0.0
		if q.Total > 0 {
			rp = (q.Remaining / q.Total) * 100
		}
		win := Window{
			Label:            q.Name,
			Used:             q.Used,
			Total:            q.Total,
			Remaining:        q.Remaining,
			RemainingPercent: rp,
			Unit:             "requests",
		}
		if q.ResetAt != "" {
			if t, err := time.Parse(time.RFC3339, q.ResetAt); err == nil {
				win.ResetAt = t
			}
		}
		snap.Windows = append(snap.Windows, win)
	}
	return snap, nil
}

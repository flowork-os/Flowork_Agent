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
	"strings"
	"time"
)

func init() { Register(&kiroFetcher{}) }

type kiroFetcher struct{}

func (k *kiroFetcher) Name() string { return "kiro" }

func (k *kiroFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("kiro: bearer token required")
	}
	profileArn, _ := p.Extra["profileArn"].(string)
	region := regionFromKiroArn(profileArn)

	url := fmt.Sprintf("https://q.%s.amazonaws.com/GetUsageLimits", region)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.0 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererruntime#1.0.0")
	if profileArn != "" {
		req.Header.Set("X-Amzn-CodeWhisperer-Profile-Arn", profileArn)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("kiro %d: %s", resp.StatusCode, snip(body))
	}

	var parsed struct {
		UsageBreakdownList []struct {
			ResourceType                string  `json:"resourceType"`
			CurrentUsageWithPrecision   float64 `json:"currentUsageWithPrecision"`
			OverallLimitWithPrecision   float64 `json:"overallLimitWithPrecision"`
			RemainingUsageWithPrecision float64 `json:"remainingUsageWithPrecision"`
		} `json:"usageBreakdownList"`
		NextDateReset string `json:"nextDateReset,omitempty"`
		Plan          string `json:"plan,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}

	snap := Snapshot{
		Provider:  "kiro",
		Plan:      parsed.Plan,
		FetchedAt: time.Now().UTC(),
		Raw:       body,
	}
	resetAt, _ := time.Parse(time.RFC3339, parsed.NextDateReset)
	for _, q := range parsed.UsageBreakdownList {
		rp := 0.0
		if q.OverallLimitWithPrecision > 0 {
			rp = (q.RemainingUsageWithPrecision / q.OverallLimitWithPrecision) * 100
		}
		snap.Windows = append(snap.Windows, Window{
			Label:            strings.ToLower(q.ResourceType),
			Used:             q.CurrentUsageWithPrecision,
			Total:            q.OverallLimitWithPrecision,
			Remaining:        q.RemainingUsageWithPrecision,
			RemainingPercent: rp,
			ResetAt:          resetAt,
			Unit:             "requests",
		})
	}
	return snap, nil
}

func regionFromKiroArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 && parts[3] != "" {
		return parts[3]
	}
	return "us-east-1"
}

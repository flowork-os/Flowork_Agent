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
	"runtime"
	"time"
)

func init() { Register(&geminiCLIFetcher{}) }

type geminiCLIFetcher struct{}

func (g *geminiCLIFetcher) Name() string { return "gemini-cli" }

func (g *geminiCLIFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("gemini-cli: bearer token required")
	}
	body := map[string]any{
		"metadata": map[string]any{
			"ideType":    "ANTIGRAVITY",
			"platform":   gcpPlatform(),
			"pluginType": "GEMINI",
		},
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist",
		bytes.NewReader(raw))
	if err != nil {
		return Snapshot{}, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("gemini-cli %d: %s", resp.StatusCode, snip(respBody))
	}

	var parsed struct {
		CurrentTier struct {
			Name string `json:"name,omitempty"`
		} `json:"currentTier,omitempty"`
		AllowedTiers []struct {
			ID        string `json:"id,omitempty"`
			Name      string `json:"name,omitempty"`
			IsDefault bool   `json:"isDefault,omitempty"`
		} `json:"allowedTiers,omitempty"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Snapshot{}, fmt.Errorf("parse: %w", err)
	}
	plan := parsed.CurrentTier.Name
	if plan == "" {
		for _, t := range parsed.AllowedTiers {
			if t.IsDefault {
				plan = t.Name
				break
			}
		}
	}
	return Snapshot{
		Provider:  "gemini-cli",
		Plan:      plan,
		FetchedAt: time.Now().UTC(),
		Raw:       respBody,
	}, nil
}

func gcpPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "DARWIN_AMD64"
	case "windows":
		return "WINDOWS_AMD64"
	default:
		return "LINUX_AMD64"
	}
}

// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package kiromodels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultRegion = "us-east-1"
	fetchTimeout  = 30 * time.Second
	cacheTTL      = 5 * time.Minute

	sdkVersion = "1.0.0"
	agentOS    = "windows"
	agentOSVer = "10.0.26200"
	nodeVer    = "22.21.1"
	kiroVer    = "0.10.32"
)

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Synthetic   bool   `json:"synthetic,omitempty"`
}

type Catalog struct {
	Region    string    `json:"region"`
	FetchedAt time.Time `json:"fetchedAt"`
	Models    []Model   `json:"models"`
}

type Params struct {
	Token      string
	ProfileArn string
	Region     string
}

type cacheEntry struct {
	expiresAt time.Time
	cat       Catalog
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

func Fetch(ctx context.Context, p Params) (Catalog, error) {
	if p.Token == "" {
		return Catalog{}, fmt.Errorf("kiro: bearer token required")
	}
	region := p.Region
	if region == "" {
		region = regionFromProfileArn(p.ProfileArn)
	}

	key := cacheKey(p.Token, region)
	cacheMu.Lock()
	if e, ok := cache[key]; ok && time.Now().Before(e.expiresAt) {
		cat := e.cat
		cacheMu.Unlock()
		return cat, nil
	}
	cacheMu.Unlock()

	base, err := fetchUpstream(ctx, p, region)
	if err != nil {
		return Catalog{}, err
	}
	cat := Catalog{
		Region:    region,
		FetchedAt: time.Now().UTC(),
		Models:    expandVariants(base),
	}

	cacheMu.Lock()
	cache[key] = cacheEntry{expiresAt: time.Now().Add(cacheTTL), cat: cat}
	cacheMu.Unlock()

	return cat, nil
}

func InvalidateCache() {
	cacheMu.Lock()
	cache = map[string]cacheEntry{}
	cacheMu.Unlock()
}

func expandVariants(base []Model) []Model {
	out := make([]Model, 0, len(base)*4)
	for _, m := range base {
		id := stripSyntheticSuffixes(m.ID)
		out = append(out, Model{ID: id, DisplayName: m.DisplayName, Vendor: m.Vendor})
		out = append(out, Model{ID: id + "-thinking", DisplayName: m.DisplayName, Vendor: m.Vendor, Synthetic: true})
		out = append(out, Model{ID: id + "-agentic", DisplayName: m.DisplayName, Vendor: m.Vendor, Synthetic: true})
		out = append(out, Model{ID: id + "-thinking-agentic", DisplayName: m.DisplayName, Vendor: m.Vendor, Synthetic: true})
	}
	return out
}

func stripSyntheticSuffixes(id string) string {
	out := id
	for _, suffix := range []string{"-thinking-agentic", "-agentic", "-thinking"} {
		if strings.HasSuffix(out, suffix) {
			out = out[:len(out)-len(suffix)]
		}
	}
	return out
}

func regionFromProfileArn(arn string) string {
	if arn == "" {
		return defaultRegion
	}
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 && parts[3] != "" {
		return parts[3]
	}
	return defaultRegion
}

func cacheKey(tok, region string) string {
	h := sha256.Sum256([]byte(tok + "|" + region))
	return hex.EncodeToString(h[:])
}

var httpClient = &http.Client{Timeout: fetchTimeout}

func fetchUpstream(ctx context.Context, p Params, region string) ([]Model, error) {
	u, err := url.Parse(fmt.Sprintf("https://q.%s.amazonaws.com/ListAvailableModels", region))
	if err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}
	if p.ProfileArn != "" {
		q := u.Query()
		q.Set("profileArn", p.ProfileArn)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", buildUserAgent())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro ListAvailableModels %d: %s", resp.StatusCode, snip(body))
	}

	var parsed struct {
		Models []struct {
			ModelId   string `json:"modelId"`
			ModelName string `json:"modelName,omitempty"`
			Provider  string `json:"provider,omitempty"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse kiro models: %w", err)
	}
	out := make([]Model, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if m.ModelId == "" {
			continue
		}
		out = append(out, Model{
			ID:          m.ModelId,
			DisplayName: m.ModelName,
			Vendor:      m.Provider,
		})
	}
	return out, nil
}

func buildUserAgent() string {
	return fmt.Sprintf(
		"aws-sdk-js/%s ua/2.1 os/%s#%s lang/js md/nodejs#%s api/codewhispererruntime#%s m/N,E kiro/%s",
		sdkVersion, agentOS, agentOSVer, nodeVer, sdkVersion, kiroVer,
	)
}

func snip(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

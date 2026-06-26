// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package cloudcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"
)

const (
	urlLoadCodeAssist = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	urlOnboardUser    = "https://cloudcode-pa.googleapis.com/v1internal:onboardUser"

	cacheTTL          = 1 * time.Hour
	fetchTimeout      = 30 * time.Second
	onboardMaxRetries = 5
)

type cacheEntry struct {
	projectID string
	fetchedAt time.Time
}

type pendingFetch struct {
	done chan struct{}
	id   string
	err  error
}

var (
	cacheMu  sync.Mutex
	cache             = map[string]cacheEntry{}
	pending           = map[string]*pendingFetch{}
	httpDoer HTTPDoer = &http.Client{Timeout: fetchTimeout}
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func SetHTTPDoer(d HTTPDoer) { httpDoer = d }

func GetProjectID(ctx context.Context, connectionID, token string) (string, error) {
	if connectionID == "" || token == "" {
		return "", errors.New("cloudcode: connectionID and token required")
	}

	cacheMu.Lock()
	if e, ok := cache[connectionID]; ok && time.Since(e.fetchedAt) < cacheTTL {
		cacheMu.Unlock()
		return e.projectID, nil
	}
	if p, ok := pending[connectionID]; ok {
		cacheMu.Unlock()
		select {
		case <-p.done:
			return p.id, p.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	p := &pendingFetch{done: make(chan struct{})}
	pending[connectionID] = p
	cacheMu.Unlock()

	pid, err := fetchOnce(ctx, token)
	p.id, p.err = pid, err
	close(p.done)

	cacheMu.Lock()
	delete(pending, connectionID)
	if err == nil && pid != "" {
		cache[connectionID] = cacheEntry{projectID: pid, fetchedAt: time.Now()}
	}
	cacheMu.Unlock()
	return pid, err
}

func Invalidate(connectionID string) {
	cacheMu.Lock()
	delete(cache, connectionID)
	cacheMu.Unlock()
}

func InvalidateAll() {
	cacheMu.Lock()
	cache = map[string]cacheEntry{}
	cacheMu.Unlock()
}

func fetchOnce(ctx context.Context, token string) (string, error) {
	loadResp, err := postCloudCode(ctx, urlLoadCodeAssist, token, map[string]any{
		"metadata": defaultMetadata(),
	})
	if err != nil {
		return "", err
	}
	if pid := extractProjectID(loadResp); pid != "" {
		return pid, nil
	}

	tier := "legacy-tier"
	if tiers, ok := loadResp["allowedTiers"].([]any); ok {
		for _, t := range tiers {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if def, _ := tm["isDefault"].(bool); def {
				if id, _ := tm["id"].(string); id != "" {
					tier = id
					break
				}
			}
		}
	}

	for attempt := 1; attempt <= onboardMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		onboardResp, err := postCloudCode(ctx, urlOnboardUser, token, map[string]any{
			"tierId":   tier,
			"metadata": defaultMetadata(),
		})
		if err != nil {
			return "", fmt.Errorf("onboardUser attempt %d: %w", attempt, err)
		}
		if pid := extractProjectID(onboardResp); pid != "" {
			return pid, nil
		}

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "", errors.New("onboardUser: no project after retries")
}

func postCloudCode(ctx context.Context, url, token string, body map[string]any) (map[string]any, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
	req.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	req.Header.Set("Client-Metadata", clientMetadataHeader())

	resp, err := httpDoer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%d: %s", resp.StatusCode, snip(respBody))
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return parsed, nil
}

func extractProjectID(m map[string]any) string {
	if v, _ := m["cloudaicompanionProject"].(string); v != "" {
		return v
	}
	if v, _ := m["project"].(string); v != "" {
		return v
	}
	if obj, ok := m["cloudaicompanionProject"].(map[string]any); ok {
		if v, _ := obj["id"].(string); v != "" {
			return v
		}
	}
	return ""
}

func defaultMetadata() map[string]any {
	return map[string]any{
		"ideType":    "ANTIGRAVITY",
		"platform":   platformEnum(),
		"pluginType": "GEMINI",
	}
}

func clientMetadataHeader() string {
	raw, _ := json.Marshal(defaultMetadata())
	return string(raw)
}

func platformEnum() string {
	switch runtime.GOOS {
	case "darwin":
		return "DARWIN_AMD64"
	case "windows":
		return "WINDOWS_AMD64"
	case "linux":
		return "LINUX_AMD64"
	default:
		return "LINUX_AMD64"
	}
}

func snip(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}

package brainbridge

// cache.go — bridge ke /api/brain/v2/cache-lookup + /cache-record.
//
// Konteks Ayah audit 2026-05-06: cached_reasoning table 266K entries
// tapi hit_count=0 forever karena kernel chat path ngga lookup cache
// sebelum LLM call. Wrapper ini fix gap — kernel chat middleware bisa
// CacheLookupOrEmpty() before LLM, lalu CacheRecord() after.
//
// Toggle: env FLOWORK_CACHE_ENABLED=1 default. Set 0 untuk disable
// (skip lookup + record, fallback langsung LLM call).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// CacheLookupResult — hasil cache lookup.
type CacheLookupResult struct {
	Hit        bool    `json:"hit"`
	ID         string  `json:"id,omitempty"`
	Response   string  `json:"response,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	HitCount   int     `json:"hit_count,omitempty"`
	ModelUsed  string  `json:"model_used,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

// CacheEnabled return true kalau caching aktif (default ON).
func CacheEnabled() bool {
	return os.Getenv("FLOWORK_CACHE_ENABLED") != "0"
}

// CacheLookup query cached_reasoning via GUI bridge. Return hit + response
// kalau ada match high-confidence. Network error → return Hit=false (silent).
func CacheLookup(ctx context.Context, query, category string) CacheLookupResult {
	if !CacheEnabled() || query == "" {
		return CacheLookupResult{Hit: false}
	}
	u := guiBaseURL() + "/api/brain/v2/cache-lookup?q=" + url.QueryEscape(query)
	if category != "" {
		u += "&category=" + url.QueryEscape(category)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return CacheLookupResult{Hit: false, Reason: "build req"}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return CacheLookupResult{Hit: false, Reason: "gui unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CacheLookupResult{Hit: false, Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out CacheLookupResult
	if err := json.Unmarshal(body, &out); err != nil {
		return CacheLookupResult{Hit: false, Reason: "decode"}
	}
	return out
}

// CacheRecord post-LLM call: save response ke cached_reasoning + recordings
// (training data). Fire-and-forget (kernel chat ngga harus block kalau bridge
// fail). Return error untuk audit.
//
// Sejak 2026-05-06 body extended dengan input_tokens + output_tokens supaya
// Health dashboard cost calc akurat per warga (sebelumnya semua merpati chat
// tampil 0/0 token + $0 karena field ngga di-forward).
func CacheRecord(ctx context.Context, query, response, model, warga, category string, inputTokens, outputTokens int) error {
	if !CacheEnabled() || query == "" || response == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"query":         query,
		"response":      response,
		"model":         model,
		"warga":         warga,
		"category":      category,
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		guiBaseURL()+"/api/brain/v2/cache-record", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cache-record build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// CSRF middleware require Origin header untuk mutative request — set
	// ke GUI base URL biar same-origin bypass.
	req.Header.Set("Origin", guiBaseURL())
	req.Header.Set("User-Agent", "flowork-kernel/cache-bridge")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cache-record gui unreachable: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cache-record HTTP %d", resp.StatusCode)
	}
	return nil
}

// CacheRecordAsync fire-and-forget version. Spawn goroutine dengan timeout 5s.
// Caller (kernel chat handler) ngga block — return immediately setelah LLM call.
func CacheRecordAsync(query, response, model, warga, category string, inputTokens, outputTokens int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = CacheRecord(ctx, query, response, model, warga, category, inputTokens, outputTokens)
	}()
}

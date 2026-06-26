// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Request struct {
	Query      string
	MaxResults int
	APIKey     string
	BaseURL    string
	Extra      map[string]any
}

type Result struct {
	Provider string         `json:"provider"`
	Query    string         `json:"query"`
	Results  []SearchResult `json:"results"`
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type SearchProvider interface {
	Name() string
	Search(ctx context.Context, req Request) (*Result, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]SearchProvider{}
)

func Register(p SearchProvider) {
	if p == nil || p.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Name()] = p
}

func Get(name string) SearchProvider {
	regMu.RLock()
	defer regMu.RUnlock()
	return registry[name]
}

func List() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

var searchHTTPClient = &http.Client{Timeout: 30 * time.Second}

func doRequest(req *http.Request, into any) error {
	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, head(body))
	}
	return json.Unmarshal(body, into)
}

func head(b []byte) string {
	if len(b) > 240 {
		return string(b[:240]) + "…"
	}
	return string(b)
}

func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

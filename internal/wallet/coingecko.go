// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 21 phase 1 — copy-adapt CoinGecko USD price fetch
//   dengan 5-min cache. Free tier 30 calls/min. Phase 2 (paid tier API
//   key, alt providers) → tambah file baru.

package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// priceCacheTTL bounds how stale a cached price can be before refetch.
// 5 minutes is plenty for a dashboard — stablecoins barely move, and ETH
// price swings matter at minute scale only for traders (which Ayah isn't).
const priceCacheTTL = 5 * time.Minute

// CoinGecko fetches USD prices for the CoinGecko IDs used by our tokens.
// Free tier: 30 calls/min, no API key required.
type CoinGecko struct {
	http  *http.Client
	mu    sync.Mutex
	cache map[string]cachedPrice
}

type cachedPrice struct {
	usd       float64
	fetchedAt time.Time
}

// NewCoinGecko creates an in-memory caching CoinGecko client.
func NewCoinGecko() *CoinGecko {
	return &CoinGecko{
		http:  &http.Client{Timeout: 10 * time.Second},
		cache: make(map[string]cachedPrice),
	}
}

// Prices returns USD price for each CoinGecko ID. Uses cache when entries
// are <5min old, fetches fresh for any stale or missing IDs.
// Errors on network issues; returns partial cached data if possible.
func (c *CoinGecko) Prices(ctx context.Context, ids []string) (map[string]float64, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Figure out which IDs are still fresh in cache.
	c.mu.Lock()
	now := time.Now()
	out := make(map[string]float64, len(ids))
	var stale []string
	for _, id := range ids {
		entry, ok := c.cache[id]
		if ok && now.Sub(entry.fetchedAt) < priceCacheTTL {
			out[id] = entry.usd
			continue
		}
		stale = append(stale, id)
	}
	c.mu.Unlock()

	if len(stale) == 0 {
		return out, nil
	}

	// Fetch only stale IDs.
	fresh, err := c.fetch(ctx, stale)
	if err != nil {
		// Network failure — return what we have cached (stale is OK for display).
		c.mu.Lock()
		for _, id := range stale {
			if entry, ok := c.cache[id]; ok {
				out[id] = entry.usd
			}
		}
		c.mu.Unlock()
		if len(out) == 0 {
			return nil, err
		}
		return out, nil
	}

	// Merge fresh into cache + output.
	c.mu.Lock()
	for id, p := range fresh {
		c.cache[id] = cachedPrice{usd: p, fetchedAt: now}
		out[id] = p
	}
	c.mu.Unlock()
	return out, nil
}

// fetch does the actual HTTP call to CoinGecko simple/price endpoint.
func (c *CoinGecko) fetch(ctx context.Context, ids []string) (map[string]float64, error) {
	params := url.Values{}
	params.Set("ids", strings.Join(ids, ","))
	params.Set("vs_currencies", "usd")

	u := "https://api.coingecko.com/api/v3/simple/price?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "flowork/wallet")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coingecko: %w", err)
	}
	defer resp.Body.Close()

	// rc121 fortress fix: cap 10MB — CoinGecko price JSON <1KB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("coingecko http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	// Response: { "ethereum": { "usd": 3450.21 }, ... }
	var parsed map[string]map[string]float64
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("coingecko parse: %w", err)
	}

	out := make(map[string]float64, len(parsed))
	for id, v := range parsed {
		if p, ok := v["usd"]; ok {
			out[id] = p
		}
	}
	return out, nil
}

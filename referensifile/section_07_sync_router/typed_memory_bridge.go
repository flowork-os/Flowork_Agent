// Package brainbridge — typed_memory_bridge.go: fetch drawers by mem_type
// dari GUI bridge endpoint /api/brain/by-type.
//
// Tier 1.1 Memory Typed System (2026-05-11): provides AlwaysInject pipeline
// untuk user-type memories (Ayah preferences, kondisi, role). Caller
// (BuildPrompt) panggil FetchUserMemoriesOrEmpty(ctx) tiap turn, inject
// hasilnya sebagai User Profile section di system prompt.
//
// Cache: 5 min (lebih lama dari drawer search 60s) — user memories berubah
// pelan, ngga perlu re-fetch tiap turn.

package brainbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const typedMemCacheTTL = 5 * time.Minute

type typedMemHit struct {
	DrawerID   string  `json:"drawer_id"`
	Wing       string  `json:"wing"`
	Room       string  `json:"room"`
	Content    string  `json:"content"`
	Importance float64 `json:"importance"`
	FiledAt    string  `json:"filed_at"`
	MemType    string  `json:"mem_type"`
}

type typedMemResp struct {
	MemType string        `json:"mem_type"`
	Hits    []typedMemHit `json:"hits"`
	Count   int           `json:"count"`
}

type typedMemCacheEntry struct {
	hits      []typedMemHit
	expiresAt time.Time
}

var (
	typedMemCacheMu sync.RWMutex
	typedMemCache   = map[string]typedMemCacheEntry{} // key = memType
)

// FetchByType — call /api/brain/by-type?type=<memType>&limit=<limit>.
//
// Args:
//   - ctx: cancellation
//   - memType: user / feedback / project / reference
//   - limit: max hits (default 10, capped 50)
//
// Returns []typedMemHit + error. Cached 5min per memType.
func FetchByType(ctx context.Context, memType string, limit int) ([]typedMemHit, error) {
	if !IsValidMemType(memType) {
		return nil, fmt.Errorf("brainbridge: invalid mem_type %q", memType)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	cacheKey := fmt.Sprintf("%s:%d", memType, limit)
	typedMemCacheMu.RLock()
	if e, ok := typedMemCache[cacheKey]; ok && time.Now().Before(e.expiresAt) {
		hits := append([]typedMemHit(nil), e.hits...)
		typedMemCacheMu.RUnlock()
		return hits, nil
	}
	typedMemCacheMu.RUnlock()

	u := fmt.Sprintf("%s/api/brain/by-type?type=%s&limit=%d", guiBaseURL(), memType, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("brainbridge by-type: build req: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brainbridge by-type: gui unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brainbridge by-type: gui %d", resp.StatusCode)
	}

	var tr typedMemResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("brainbridge by-type: decode: %w", err)
	}

	typedMemCacheMu.Lock()
	typedMemCache[cacheKey] = typedMemCacheEntry{
		hits:      append([]typedMemHit(nil), tr.Hits...),
		expiresAt: time.Now().Add(typedMemCacheTTL),
	}
	typedMemCacheMu.Unlock()

	return tr.Hits, nil
}

// FetchUserMemoriesOrEmpty — anti-fatal wrapper for fetching user-type
// memories. Return formatted markdown ke caller (BuildPrompt) buat inject.
//
// 2026-05-17 Phase 0.1 (Ayah arahan): TRY DIRECT SQLite FIRST (bypass GUI
// HTTP /api/brain/by-type endpoint timeout). Fallback ke HTTP cuma kalau
// direct SQLite init gagal.
//
// Empty string kalau ngga ada user-type drawer atau bridge fail.
//
// Format output (siap di-prepend ke prompt):
//
//	[Ayah profile - sticky user-type memory]
//	- Ayah suka response pendek dengan struktur jelas.
//	- Bahasa internal Ayah: lo/gw/bro casual.
func FetchUserMemoriesOrEmpty(ctx context.Context) string {
	// Phase 0.1: prioritize direct SQLite (1000x faster + immune ke GUI down).
	if direct := FetchUserMemoriesDirectOrEmpty(); direct != "" {
		return direct
	}
	// Fallback HTTP bridge kalau direct SQLite init gagal.
	hits, err := FetchByType(ctx, MemTypeUser, 10)
	if err != nil {
		// Silent fail — caller prompt build tetep jalan.
		return ""
	}
	if len(hits) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Ayah profile - sticky user-type memory]\n")
	for _, h := range hits {
		// 1-line summary kalau content panjang; preserve newlines minimum.
		content := strings.TrimSpace(h.Content)
		// Drop excessive blank lines (compactify)
		lines := strings.Split(content, "\n")
		nonEmpty := lines[:0]
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				nonEmpty = append(nonEmpty, l)
			}
		}
		content = strings.Join(nonEmpty, " ")
		// Cap 400 chars per memory (compact pipeline).
		if len([]rune(content)) > 400 {
			runes := []rune(content)
			content = string(runes[:400]) + "..."
		}
		sb.WriteString("- ")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// FetchFeedbackMemoriesOrEmpty — fetch high-priority feedback (lesson learned).
//
// Use case di build_prompt: inject sebagai "Lessons Learned" section setelah
// User Profile (per-type retrieval rule: feedback PriorityWeight 1.5 dengan
// VerifyBeforeUse — caller responsibility verify before cite specific facts).
func FetchFeedbackMemoriesOrEmpty(ctx context.Context, limit int) string {
	if limit <= 0 {
		limit = 5
	}
	hits, err := FetchByType(ctx, MemTypeFeedback, limit)
	if err != nil || len(hits) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Feedback - lessons learned from past corrections]\n")
	for _, h := range hits {
		content := strings.TrimSpace(h.Content)
		if len([]rune(content)) > 300 {
			runes := []rune(content)
			content = string(runes[:300]) + "..."
		}
		sb.WriteString("- ")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return sb.String()
}

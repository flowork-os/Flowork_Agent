// Package brainbridge — bridge ke floworkos-go gui /api/brain/search-drawers.
//
// hunting_bug 2026-04-30 BUG-029 fix: Brain V4 doctrine sudah di-ingest
// (drawers populated 23K+ row) tapi BuildPrompt ngga query top-K untuk inject
// → Merpati "lupa" detail Aola Sahidin walaupun data ada di brain.
//
// Pattern persis dengan wargacaps + capabilitytier bridge:
//   - HTTP GET ke gui :3101/api/brain/search-drawers?query=X&k=5
//   - Cache TTL 60s (lebih lama karena query natural lang varied per request)
//   - Fallback graceful: return [] kalau gui down — caller skip injection
//
// Usage di BuildPrompt: pass userMsg sebagai query, inject result sebagai
// "## Memory Context" di system prompt.

package brainbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	CacheTTL = 60 * time.Second
	// HTTPTimeout — bumped 2s → 10s (2026-05-01) karena FTS5 query dengan
	// long natural language input (e.g. "Sebutin nama lengkap pencipta
	// Flowork. Pakai brain.search...") di GUI engine kadang ambil >2s
	// (SQLite tokenize + score). 10s comfortable margin, masih jauh di
	// bawah typical LLM call timeout (60s). Override env FLOWORK_BRAIN_BRIDGE_TIMEOUT_MS.
	HTTPTimeout = 10 * time.Second
	// DefaultK — top-K drawer hits di-inject ke system prompt. Doctrine
	// quality_control.md §24 (FQ-Brain Cascade): "top-15 drawers memori
	// penting (L1) tiap turn". Sebelumnya 5 (BUG-127 root cause #1) — too
	// few, banyak factual query miss relevant drawer.
	//
	// Trade-off: 15 * ~500 char/drawer = +7.5K char system prompt ≈ +1.5K
	// token. Acceptable cost untuk grounding richness. Override via env
	// FLOWORK_BRAIN_INJECT_K kalau perlu sesuaikan budget.
	DefaultK = 15
)

// EffectiveK return K dari env override atau DefaultK. Caller (BuildPrompt)
// pakai ini bukan hardcode angka.
func EffectiveK() int {
	if v := os.Getenv("FLOWORK_BRAIN_INJECT_K"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 && n <= 30 {
			return n
		}
	}
	return DefaultK
}

// Hit — single drawer search result.
//
// Tier 1.1 Memory Typed System (2026-05-11): MemType + FiledAt fields added.
// `omitempty` JSON tag keep backward-compat: GUI side ngga selalu populate
// (older GUI return tanpa field). Caller (SearchDrawersOrEmpty) apply typed
// scoring post-fetch — empty MemType fallback ke 'project' rules.
type Hit struct {
	Wing     string  `json:"wing"`
	Room     string  `json:"room"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	DrawerID string  `json:"drawer_id"`
	MemType  string  `json:"mem_type,omitempty"`
	FiledAt  string  `json:"filed_at,omitempty"`
}

type searchResp struct {
	Query string `json:"query"`
	Hits  []Hit  `json:"hits"`
	Count int    `json:"count"`
}

type cacheEntry struct {
	hits      []Hit
	expiresAt time.Time
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]cacheEntry{}

	httpClient = &http.Client{Timeout: HTTPTimeout}
)

func guiBaseURL() string {
	if v := os.Getenv("FLOWORK_GUI_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:3101"
}

// SearchDrawers — fetch top-K relevant drawer untuk query.
//
// Returns:
//   - []Hit: top-K drawer (sorted by score desc), max k=20
//   - error: bridge fail (caller bisa pakai SearchDrawersOrEmpty buat fallback)
//
// Cached 60s per query string (case-sensitive).
func SearchDrawers(ctx context.Context, query string, k int) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("brainbridge: empty query")
	}
	if k <= 0 {
		k = DefaultK
	}
	if k > 20 {
		k = 20
	}

	cacheKey := fmt.Sprintf("%d::%s", k, query)

	cacheMu.RLock()
	if e, ok := cache[cacheKey]; ok && time.Now().Before(e.expiresAt) {
		hits := append([]Hit(nil), e.hits...)
		cacheMu.RUnlock()
		return hits, nil
	}
	cacheMu.RUnlock()

	u := guiBaseURL() + "/api/brain/search-drawers?query=" + url.QueryEscape(query) + "&k=" + fmt.Sprintf("%d", k)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("brainbridge: build req: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brainbridge: gui unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brainbridge: gui returned %d", resp.StatusCode)
	}

	var sr searchResp
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("brainbridge: decode: %w", err)
	}

	cacheMu.Lock()
	cache[cacheKey] = cacheEntry{
		hits:      append([]Hit(nil), sr.Hits...),
		expiresAt: time.Now().Add(CacheTTL),
	}
	cacheMu.Unlock()

	return sr.Hits, nil
}

// SearchDrawersOrEmpty — anti-fatal wrapper. Return empty slice kalau bridge fail.
// Caller (BuildPrompt) cek len() > 0 sebelum render section.
//
// BUG-127 fix 2026-05-01: tambah diagnostic stderr log untuk visibility.
// Sebelumnya silent fail → BuildPrompt skip injection → LLM jawab tanpa
// grounding → Merpati klaim "ngga punya brain memory" walaupun drawer ada.
//
// Tier 1.1 Memory Typed System (2026-05-11): post-fetch, apply typed scoring
// per-hit via ApplyTypedScoring (priority weight + age decay). Drop hits di
// bawah MinScoreKeep threshold per-type. Toggle via env
// FLOWORK_TYPED_SCORING=0 untuk disable kalau perlu rollback.
func SearchDrawersOrEmpty(ctx context.Context, query string, k int) []Hit {
	hits, err := SearchDrawers(ctx, query, k)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[brainbridge] search ERROR query=%q k=%d err=%v — prompt akan tanpa Memory Context section\n",
			truncForLog(query, 80), k, err)
		return nil
	}
	if len(hits) == 0 {
		fmt.Fprintf(os.Stderr, "[brainbridge] search OK query=%q k=%d hits=0 — kemungkinan FTS5 ngga match, prompt tanpa Memory Context\n",
			truncForLog(query, 80), k)
		return hits
	}

	// Tier 1.1: apply typed scoring + drop below threshold
	if os.Getenv("FLOWORK_TYPED_SCORING") != "0" {
		filtered := applyTypedReranking(hits)
		droppedCount := len(hits) - len(filtered)
		if droppedCount > 0 {
			fmt.Fprintf(os.Stderr, "[brainbridge] typed-scoring dropped %d/%d hits below MinScoreKeep threshold\n",
				droppedCount, len(hits))
		}
		hits = filtered
	}

	if len(hits) > 0 {
		fmt.Fprintf(os.Stderr, "[brainbridge] search OK query=%q k=%d hits=%d (top score=%.2f)\n",
			truncForLog(query, 80), k, len(hits), hits[0].Score)
	} else {
		fmt.Fprintf(os.Stderr, "[brainbridge] search OK query=%q k=%d hits=0 post-typed-filter\n",
			truncForLog(query, 80), k)
	}
	return hits
}

// applyTypedReranking — apply per-type scoring + drop below MinScoreKeep.
// Sorted by post-typed-score desc. Hits dengan mem_type kosong (legacy row)
// fallback ke 'project' rules (decay 30d, neutral weight).
func applyTypedReranking(hits []Hit) []Hit {
	now := time.Now()
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		ageDays := 0
		if h.FiledAt != "" {
			// FiledAt expected ISO format; tolerate parse fail (fallback to 0).
			if t, err := time.Parse("2006-01-02 15:04:05", h.FiledAt); err == nil {
				ageDays = int(now.Sub(t).Hours() / 24)
			} else if t, err := time.Parse(time.RFC3339, h.FiledAt); err == nil {
				ageDays = int(now.Sub(t).Hours() / 24)
			}
		}
		newScore := ApplyTypedScoring(h.Score, h.MemType, ageDays)
		rules := RetrievalRulesFor(h.MemType)
		if newScore < rules.MinScoreKeep {
			continue // drop below threshold
		}
		h.Score = newScore
		out = append(out, h)
	}
	// Re-sort by new score desc.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

// truncForLog cap log line length to avoid stderr spam.
func truncForLog(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// FormatAsContext — render hits sebagai markdown section untuk inject ke
// system prompt. Empty kalau no hits.
//
// BUG-127 fix 2026-05-01: ganti instruksi closing dari "jujur bilang 'belum
// ada di brain'" jadi instruksi PROAKTIF pakai brain.search tool. Sebelumnya
// LLM literal interpret "belum ada di brain" → klaim "ngga punya brain
// memory" walaupun untuk query lain bisa ada hit. Sekarang: kalau context
// ngga cover, panggil brain.search tool buat dig deeper.
func FormatAsContext(hits []Hit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Memory Context (top ")
	b.WriteString(fmt.Sprintf("%d", len(hits)))
	b.WriteString(" drawer dari Brain Flowork)\n\n")
	b.WriteString("Gunakan info di bawah ini sebagai grounding factual untuk menjawab — JANGAN halu/karang. Kalau topik di luar context ini, PANGGIL tool `brain.search` dengan query lebih spesifik buat dig deeper SEBELUM jawab. JANGAN langsung tolak dengan 'ngga punya brain memory' — tool brain.search tersedia, pakai itu.\n\n")
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("### [%d] wing=%s room=%s (score %.2f)\n", i+1, h.Wing, h.Room, h.Score))
		b.WriteString(h.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func InvalidateAll() {
	cacheMu.Lock()
	cache = map[string]cacheEntry{}
	cacheMu.Unlock()
}

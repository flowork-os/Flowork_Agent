// Package tools — kernel/tools/categories.go
//
// DB-backed taxonomy lookup per README §4.2 plug-and-play (config via DB,
// BUKAN const di code). Source-of-truth = settings DB tables:
//   - tool_categories(category, tool_name)
//   - division_tool_priors(division, tool_name, weight)
//   - warga_division_map(warga_id, division)
//
// Seeder: scripts/seed_tool_categories.py (re-runnable + idempotent).
//
// Cache: dalam-memory 60s TTL (taxonomy ngga sering berubah, hot-reload kalau
// Ayah edit via GUI lalu cache TTL expire).

package tools

import (
	"sort"
	"sync"
	"time"

	"github.com/flowork/kernel/kernel/settings"
)

const categoryCacheTTL = 60 * time.Second

type categoryCache struct {
	mu              sync.RWMutex
	categoryToTools map[string][]string // category → []tool_name
	toolToCategory  map[string]string   // tool_name → first category found
	categoryList    []string            // sorted unique
	wargaDivisions  map[string]string   // warga_id → division
	divisionPriors  map[string][]string // division → []tool_name (ordered by weight desc)
	expiresAt       time.Time
}

var sharedCatCache = &categoryCache{}

// reloadCategoryCache fetch taxonomy + priors + warga divisions dari settings DB.
// Call internal — invoked by lookup methods saat cache expire.
func (c *categoryCache) reloadIfStale() {
	c.mu.RLock()
	fresh := time.Now().Before(c.expiresAt) && c.categoryToTools != nil
	c.mu.RUnlock()
	if fresh {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check after lock acquire (double-check pattern).
	if time.Now().Before(c.expiresAt) && c.categoryToTools != nil {
		return
	}

	store := settings.Shared()
	if store == nil {
		// No store → empty cache, brief TTL biar retry cepat.
		c.categoryToTools = map[string][]string{}
		c.toolToCategory = map[string]string{}
		c.categoryList = nil
		c.wargaDivisions = map[string]string{}
		c.divisionPriors = map[string][]string{}
		c.expiresAt = time.Now().Add(5 * time.Second)
		return
	}

	db := store.DB()
	if db == nil {
		c.expiresAt = time.Now().Add(5 * time.Second)
		return
	}

	cToT := map[string][]string{}
	tToC := map[string]string{}

	rows, err := db.Query("SELECT category, tool_name FROM tool_categories ORDER BY category, tool_name")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat, tn string
			if err := rows.Scan(&cat, &tn); err != nil {
				continue
			}
			cToT[cat] = append(cToT[cat], tn)
			if _, exists := tToC[tn]; !exists {
				tToC[tn] = cat // first wins (deterministic from ORDER BY)
			}
		}
	}

	wDiv := map[string]string{}
	if rows2, err := db.Query("SELECT warga_id, division FROM warga_division_map"); err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var w, d string
			if err := rows2.Scan(&w, &d); err == nil {
				wDiv[w] = d
			}
		}
	}

	dPriors := map[string][]string{}
	if rows3, err := db.Query("SELECT division, tool_name FROM division_tool_priors ORDER BY division, weight DESC, tool_name"); err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var div, tn string
			if err := rows3.Scan(&div, &tn); err == nil {
				dPriors[div] = append(dPriors[div], tn)
			}
		}
	}

	catList := make([]string, 0, len(cToT))
	for k := range cToT {
		catList = append(catList, k)
	}
	sort.Strings(catList)

	c.categoryToTools = cToT
	c.toolToCategory = tToC
	c.categoryList = catList
	c.wargaDivisions = wDiv
	c.divisionPriors = dPriors
	c.expiresAt = time.Now().Add(categoryCacheTTL)
}

// GetCategoryList return semua kategori yang tersedia (sorted alfabetik).
// Empty slice kalau DB kosong / unreachable.
func GetCategoryList() []string {
	sharedCatCache.reloadIfStale()
	sharedCatCache.mu.RLock()
	defer sharedCatCache.mu.RUnlock()
	out := make([]string, len(sharedCatCache.categoryList))
	copy(out, sharedCatCache.categoryList)
	return out
}

// GetToolsByCategory return list tool_name yang tagged ke category. Empty
// kalau category ngga ada di DB.
func GetToolsByCategory(category string) []string {
	sharedCatCache.reloadIfStale()
	sharedCatCache.mu.RLock()
	defer sharedCatCache.mu.RUnlock()
	src := sharedCatCache.categoryToTools[category]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// GetWargaDivision return division string untuk warga (e.g. "coder", "comm",
// "security", "analytics", "planner"). Default "comm" kalau warga ngga ada di
// map (defensive default — comm = generic chat assistant).
func GetWargaDivision(wargaID string) string {
	sharedCatCache.reloadIfStale()
	sharedCatCache.mu.RLock()
	defer sharedCatCache.mu.RUnlock()
	if d, ok := sharedCatCache.wargaDivisions[wargaID]; ok {
		return d
	}
	return "comm"
}

// GetDivisionPriors return ordered tool list (by weight DESC) sebagai SHORTCUT
// seed untuk warga di division tsb. Untuk cold-start warga baru (Ayah patch
// 2026-05-09 "untuk anak baru kita kasih recent tools sesuai divisi dia").
//
// Empty slice kalau division ngga ada di priors table.
func GetDivisionPriors(division string) []string {
	sharedCatCache.reloadIfStale()
	sharedCatCache.mu.RLock()
	defer sharedCatCache.mu.RUnlock()
	src := sharedCatCache.divisionPriors[division]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// InvalidateCategoryCache force-clear cache (untuk test atau saat Ayah edit
// taxonomy via GUI dan mau langsung apply).
func InvalidateCategoryCache() {
	sharedCatCache.mu.Lock()
	defer sharedCatCache.mu.Unlock()
	sharedCatCache.expiresAt = time.Time{}
}

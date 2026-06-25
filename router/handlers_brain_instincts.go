// handlers_brain_instincts.go — GET /api/brain/instincts: list insting buat halaman
// Brain→Instincts (GUI). File BARU, NON-frozen (host evolve).
//
// AKAR (2026-06-25, owner: "list insting di GUI ga muncul, analisa dulu"): halaman lama
// query `/api/brain/wing?wing=training_data&room_like=%instinct` → 0 hit karena (a) insting
// nyatanya wing `doctrine`/`capability` (bukan `training_data`) + (b) `%instinct` = SQL
// ENDS-with, padahal room-nya `instinct_*` = STARTS-with. Dua-duanya meleset → "No instincts found".
//
// CABUT-AKAR (no migrasi data, no break injeksi): endpoint ini pakai SUMBER YANG SAMA dengan
// injeksi proaktif `maybeInjectInstinct` → `brain.ListInstinctDrawers` (query `room LIKE 'instinct%'`
// LINTAS-WING, non-deleted, non-quarantined, urut importance). Jadi yang KE-LIHAT di GUI = persis
// yang KE-INJECT ke agent. ZERO sentuh data + ZERO sentuh file frozen (cuma MANGGIL ListInstinctDrawers).
package main

import (
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

// brainInstinctsHandler — GET /api/brain/instincts?limit=N&offset=M
// Balik {available, count, drawers:[{id,wing,room,content,importance}]} (shape drop-in buat
// loadBrainInstincts() yang baca `r.drawers`). Lintas-wing (instinct_*), urut importance DESC.
func brainInstinctsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s) // samain DB sama dispatcher/injeksi
	if !brain.Available() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "drawers": []any{}})
		return
	}

	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	// Ambil semua insting hidup (skala kecil; cap di ListInstinctDrawers), lalu slice buat pager GUI.
	all, err := brain.ListInstinctDrawers(r.Context(), 1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total := len(all)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	page := all[offset:end]

	drawers := make([]map[string]any, 0, len(page))
	for _, it := range page {
		drawers = append(drawers, map[string]any{
			"id":         it.ID,
			"drawer_id":  it.ID,
			"wing":       it.Wing,
			"room":       it.Room,
			"content":    it.Content,
			"importance": it.Importance,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"count":     len(drawers),
		"total":     total,
		"offset":    offset,
		"drawers":   drawers,
	})
}

// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without explicit owner (Mr.Dev) approval.
// Owner: Aola Sahidin (Mr.Dev). Frozen: 2026-06-25 (chattr +i + hash KERNEL_FREEZE.md).
// ⚠️ Query insting sengaja STABIL. Filter/scoping insting → lakukan di sisi router
//   (RegisterInstinctSelector di instinctenrich_ext.go), JANGAN ngedit query ini.
//   Tumbuh awareness = tambah drawer room=instinct_* (ga sentuh kode).
//
// instincts.go — SUMBER INSTING buat injeksi proaktif (router/instinctenrich.go).
//
// Mirror pola ListMistakes (mistakes.go): OpenRW + plain SQL + filter
// deleted/quarantine. Insting di shared-brain disimpen
// sbg DRAWER room=`instinct_*` (universal/bisnis/kehidupan/security/...). Ranking
// TIDAK di sini — pemanggil (router) yang rank token-overlap (deterministik, no-vindex).
//
// Kenapa ada: insting selama ini PULL-ONLY (tool instinct_recall) → jarang kepanggil
// di momen tepat → agent "ga sadar kapan pakai kapabilitasnya". File ini bikin insting
// bisa di-LIST murah biar router maksa-inject (sejajar doktrin & antibodi).

package brain

import (
	"context"
	"fmt"
)

// InstinctDrawer — satu insting (drawer room=instinct_*) di shared brain.
type InstinctDrawer struct {
	ID         string
	Content    string
	Room       string // instinct_universal / instinct_bisnis / instinct_security / ...
	Wing       string
	Importance float64 // dipakai router sbg bobot (mirip karma antibodi)
}

// ListInstinctDrawers — narik insting HIDUP (deleted_at NULL, quarantined=0,
// room LIKE 'instinct%') dari shared brain, urut importance DESC. Fails-safe:
// db error → (nil, err); pemanggil skip (request tetap jalan). limit di-clamp.
func ListInstinctDrawers(ctx context.Context, limit int) ([]InstinctDrawer, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	db, err := OpenRW()
	if err != nil {
		return nil, err
	}
	const q = `SELECT id, content, room, COALESCE(wing, ''), COALESCE(importance, 3.0)
	           FROM drawers
	           WHERE deleted_at IS NULL AND quarantined = 0
	             AND room LIKE 'instinct%'
	           ORDER BY importance DESC
	           LIMIT ?`
	rows, qerr := db.QueryContext(ctx, q, limit)
	if qerr != nil {
		return nil, fmt.Errorf("query instinct drawers: %w", qerr)
	}
	defer rows.Close()

	var out []InstinctDrawer
	for rows.Next() {
		var d InstinctDrawer
		if err := rows.Scan(&d.ID, &d.Content, &d.Room, &d.Wing, &d.Importance); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

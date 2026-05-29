package codeindex

// layerclassify.go — auto-grouping path → architectural layer.
//
// Per Ayah audit 2026-05-06: adopt selektif fitur "Understand-Anything"
// (Lum1104/Understand-Anything). Layer auto-classifier = bikin Code Map
// node punya warna grup (UI / Service / Data / Util / API / Test) tanpa
// dependency external. Pure heuristic path-based, native Go, store ke
// kolom `codemap_nodes.layer`.
//
// Heuristic priority (top match wins):
//   1. Test — path contains _test.go atau /tests/ atau /testdata/
//   2. UI — path contains /static/, /tabs/, /js/, /css/, .html
//   3. API — path contains /api/, /handlers/, /routes/, /guiapi/
//   4. Data — path contains /db/, /brain/, /storage/, /sqlite/
//   5. Service — path contains /service/, /daemon/, /worker/, /cmd/
//   6. Util — path contains /utils/, /helpers/, /common/, /pkg/
//   7. default — Core
//
// Layer kolom dipakai frontend codemap.js untuk color-by-layer mode.

import (
	"database/sql"
	"strings"
)

// Layer constant.
const (
	LayerUI      = "UI"
	LayerAPI     = "API"
	LayerData    = "Data"
	LayerService = "Service"
	LayerUtil    = "Util"
	LayerTest    = "Test"
	LayerCore    = "Core"
)

// ClassifyLayer return architectural layer untuk file path.
// Pure deterministic — same path always returns same layer.
func ClassifyLayer(path string) string {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	switch {
	case strings.Contains(p, "_test.go") || strings.Contains(p, "/tests/") || strings.Contains(p, "/testdata/"):
		return LayerTest
	case strings.Contains(p, "/static/") || strings.Contains(p, "/tabs/") || strings.Contains(p, "/js/") || strings.Contains(p, "/css/") || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".css"):
		return LayerUI
	case strings.Contains(p, "/api/") || strings.Contains(p, "/handlers/") || strings.Contains(p, "/routes/") || strings.Contains(p, "/guiapi/"):
		return LayerAPI
	case strings.Contains(p, "/db/") || strings.Contains(p, "/brain/") || strings.Contains(p, "/storage/") || strings.Contains(p, "/sqlite/") || strings.HasSuffix(p, ".sql"):
		return LayerData
	case strings.Contains(p, "/service/") || strings.Contains(p, "/daemon/") || strings.Contains(p, "/worker/") || strings.Contains(p, "/cmd/"):
		return LayerService
	case strings.Contains(p, "/utils/") || strings.Contains(p, "/helpers/") || strings.Contains(p, "/common/") || strings.Contains(p, "/fsutil/") || strings.Contains(p, "/internal/util"):
		return LayerUtil
	}
	return LayerCore
}

// EnsureLayerColumn ALTER codemap_nodes tambah `layer` column kalau belum
// ada. Idempotent — ALTER fail kalau kolom exist (silent).
func EnsureLayerColumn(db *sql.DB) error {
	_, _ = db.Exec(`ALTER TABLE codemap_nodes ADD COLUMN layer TEXT NOT NULL DEFAULT ''`)
	// Backfill layer column untuk row existing yang kosong.
	rows, err := db.Query(`SELECT path FROM codemap_nodes WHERE layer = '' OR layer IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths = append(paths, p)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range paths {
		layer := ClassifyLayer(p)
		_, _ = db.Exec(`UPDATE codemap_nodes SET layer = ? WHERE path = ?`, layer, p)
	}
	return nil
}

// LayerCounts return distribution layer untuk dashboard.
func LayerCounts(db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	rows, err := db.Query(`SELECT layer, COUNT(*) FROM codemap_nodes GROUP BY layer`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var l string
		var n int
		if err := rows.Scan(&l, &n); err == nil {
			if l == "" {
				l = LayerCore
			}
			out[l] = n
		}
	}
	return out, rows.Err()
}

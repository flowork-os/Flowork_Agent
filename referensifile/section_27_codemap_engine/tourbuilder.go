package codeindex

// tourbuilder.go — guided tour generator untuk Code Map.
//
// Per Ayah 2026-05-06: adopt fitur "Understand-Anything" (Lum1104). Tour
// = ordered list of files untuk warga AI baru / Ayah belajar codebase.
// Path optimization: dependency-first (entry-point → leaves), grouped by
// layer biar warga ngga jump-around.
//
// Output: list TourStep dengan order, path, layer, summary, why_this_step.
// Caller (handler) serialize ke JSON.

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// TourStep — satu step di guided tour.
type TourStep struct {
	Order   int    `json:"order"`
	Path    string `json:"path"`
	Layer   string `json:"layer"`
	Pkg     string `json:"pkg"`
	Summary string `json:"summary"` // doc_comment short OR auto-derived
	WhyStep string `json:"why_this_step"`
}

// BuildTour generate tour dengan max N step. Strategy:
//  1. Roots dulu (entry-point, no incoming edges) — UI/API layer prioritas
//     biar warga tau "ini gateway".
//  2. Per layer (UI → API → Service → Data → Core → Util → Test) ambil
//     top-3 by line_count (yang paling padat = paling penting).
//  3. Limit total ke max N.
//
// Note: Tour ngga panggil LLM — pure SQL + heuristic supaya sovereign +
// fast. Summary diturunkan dari doc_comment kolom existing (kalau kosong,
// fallback ke "<filename> di package <pkg>").
func BuildTour(db *sql.DB, maxSteps int) ([]TourStep, error) {
	if maxSteps <= 0 {
		maxSteps = 15
	}
	// 1. Find entry-point roots (no incoming edge atau path contains /cmd/)
	rootRows, err := db.Query(`
		SELECT n.path, COALESCE(n.layer, ''), n.pkg, COALESCE(n.doc_comment, ''), n.line_count
		FROM codemap_nodes n
		WHERE n.path LIKE '%/cmd/%/main.go'
		   OR NOT EXISTS (SELECT 1 FROM codemap_edges e WHERE e.target_path = n.path)
		ORDER BY n.line_count DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, fmt.Errorf("tour: roots query: %w", err)
	}
	defer rootRows.Close()
	var steps []TourStep
	step := 1
	for rootRows.Next() {
		var path, layer, pkg, doc string
		var lineCount int
		_ = rootRows.Scan(&path, &layer, &pkg, &doc, &lineCount)
		if layer == "" {
			layer = ClassifyLayer(path)
		}
		steps = append(steps, TourStep{
			Order:   step,
			Path:    path,
			Layer:   layer,
			Pkg:     pkg,
			Summary: shortenDoc(doc, path, pkg),
			WhyStep: fmt.Sprintf("Entry point — gerbang masuk arsitektur (%s, %d baris).", layer, lineCount),
		})
		step++
		if step > maxSteps {
			return steps, nil
		}
	}
	if err := rootRows.Err(); err != nil {
		return steps, err
	}

	// 2. Per layer ambil top-3 by line_count.
	layerOrder := []string{LayerAPI, LayerService, LayerData, LayerCore, LayerUI, LayerUtil}
	for _, layer := range layerOrder {
		if step > maxSteps {
			break
		}
		layerRows, err := db.Query(`
			SELECT path, COALESCE(layer, ''), pkg, COALESCE(doc_comment, ''), line_count
			FROM codemap_nodes
			WHERE layer = ?
			  AND path NOT IN (SELECT path FROM codemap_nodes WHERE path LIKE '%/cmd/%/main.go')
			ORDER BY line_count DESC
			LIMIT 3
		`, layer)
		if err != nil {
			continue
		}
		for layerRows.Next() {
			if step > maxSteps {
				break
			}
			var path, lyr, pkg, doc string
			var lineCount int
			_ = layerRows.Scan(&path, &lyr, &pkg, &doc, &lineCount)
			// Skip kalau udah masuk dari roots loop
			already := false
			for _, s := range steps {
				if s.Path == path {
					already = true
					break
				}
			}
			if already {
				continue
			}
			steps = append(steps, TourStep{
				Order:   step,
				Path:    path,
				Layer:   lyr,
				Pkg:     pkg,
				Summary: shortenDoc(doc, path, pkg),
				WhyStep: fmt.Sprintf("Layer %s — top-3 file padat (%d baris) dari layer ini.", lyr, lineCount),
			})
			step++
		}
		layerRows.Close()
	}

	// Sort by order untuk konsistensi
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })
	return steps, nil
}

// shortenDoc trim doc_comment ke max 200 char + fallback kalau empty.
func shortenDoc(doc, path, pkg string) string {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		base := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			base = path[idx+1:]
		}
		return fmt.Sprintf("File %s di package %s (no doc comment).", base, pkg)
	}
	if len(doc) > 200 {
		doc = doc[:200] + "..."
	}
	return doc
}

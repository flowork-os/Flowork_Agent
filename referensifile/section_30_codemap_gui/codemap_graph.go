// codemap_graph.go — Core graph handlers: Graph, Node, Impact.
package guiapi

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
)

// CodemapGraphHandler GET /api/codemap/graph
// Return semua nodes + edges untuk planet visualization.
func CodemapGraphHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		// 2026-05-06: ensure layer column exist + backfill (idempotent first call).
		_ = codeindex.EnsureLayerColumn(brainDB)
		// Compute recently-touched set via git log (last 5 commits).
		touched := codeindex.RecentlyTouchedSet(ws, 5)

		nodeRows, err := brainDB.Query(`
			SELECT n.path, n.name, n.pkg, n.file_type, n.line_count, n.size_bytes,
			       n.exported_symbols, n.doc_comment, n.health_score, n.has_tests,
			       n.has_docs, n.issues, n.last_indexed, COALESCE(n.layer, '') as layer,
			       (SELECT COUNT(*) FROM codemap_edges WHERE from_path = n.path) as dep_count,
			       (SELECT COUNT(*) FROM codemap_edges WHERE to_path = n.path) as rev_count
			FROM codemap_nodes n`)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		defer nodeRows.Close()

		var nodes []codemapNode
		for nodeRows.Next() {
			var n codemapNode
			var symJSON, issJSON string
			var hasTests, hasDocs int
			if err := nodeRows.Scan(
				&n.Path, &n.Name, &n.Pkg, &n.FileType, &n.LineCount, &n.SizeBytes,
				&symJSON, &n.DocComment, &n.HealthScore, &hasTests, &hasDocs,
				&issJSON, &n.LastIndexed, &n.Layer, &n.DependencyCount, &n.DependentCount,
			); err != nil {
				continue
			}
			n.HasTests = hasTests == 1
			n.HasDocs = hasDocs == 1
			if err := json.Unmarshal([]byte(symJSON), &n.ExportedSymbols); err != nil {
				log.Printf("codemap: unmarshal exported_symbols %q: %v", n.Path, err)
			}
			if err := json.Unmarshal([]byte(issJSON), &n.Issues); err != nil {
				log.Printf("codemap: unmarshal issues %q: %v", n.Path, err)
			}
			// 2026-05-06: layer fallback kalau backfill belum jalan + apply
			// recently-touched flag dari git log.
			if n.Layer == "" {
				n.Layer = codeindex.ClassifyLayer(n.Path)
			}
			n.RecentlyTouched = touched[n.Path]
			nodes = append(nodes, n)
		}
		if err := nodeRows.Err(); err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		edgeRows, err := brainDB.Query(`SELECT from_path, to_path, edge_type FROM codemap_edges`)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		defer edgeRows.Close()

		var edges []codemapEdge
		for edgeRows.Next() {
			var e codemapEdge
			if err := edgeRows.Scan(&e.From, &e.To, &e.EdgeType); err != nil {
				continue
			}
			edges = append(edges, e)
		}
		if err := edgeRows.Err(); err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"nodes":        nodes,
			"edges":        edges,
			"node_count":   len(nodes),
			"edge_count":   len(edges),
			"generated_at": time.Now().Format(time.RFC3339),
		})
	}
}

// CodemapNodeHandler GET /api/codemap/node?path=...
func CodemapNodeHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", 400)
			return
		}
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		var n codemapNode
		var symJSON, issJSON string
		var hasTests, hasDocs int
		err = brainDB.QueryRow(`
			SELECT n.path, n.name, n.pkg, n.file_type, n.line_count, n.size_bytes,
			       n.exported_symbols, n.doc_comment, n.health_score, n.has_tests,
			       n.has_docs, n.issues, n.last_indexed,
			       (SELECT COUNT(*) FROM codemap_edges WHERE from_path = n.path),
			       (SELECT COUNT(*) FROM codemap_edges WHERE to_path = n.path)
			FROM codemap_nodes n WHERE n.path = ?`, path).Scan(
			&n.Path, &n.Name, &n.Pkg, &n.FileType, &n.LineCount, &n.SizeBytes,
			&symJSON, &n.DocComment, &n.HealthScore, &hasTests, &hasDocs,
			&issJSON, &n.LastIndexed, &n.DependencyCount, &n.DependentCount,
		)
		if err != nil {
			http.Error(w, "node not found", 404)
			return
		}
		n.HasTests = hasTests == 1
		n.HasDocs = hasDocs == 1
		if err := json.Unmarshal([]byte(symJSON), &n.ExportedSymbols); err != nil {
			log.Printf("codemap: unmarshal exported_symbols %q: %v", path, err)
		}
		if err := json.Unmarshal([]byte(issJSON), &n.Issues); err != nil {
			log.Printf("codemap: unmarshal issues %q: %v", path, err)
		}

		depRows, _ := brainDB.Query(`SELECT to_path FROM codemap_edges WHERE from_path = ?`, path)
		var deps []string
		if depRows != nil {
			for depRows.Next() {
				var d string
				if err := depRows.Scan(&d); err != nil {
					continue
				}
				deps = append(deps, d)
			}
			_ = depRows.Err()
			depRows.Close()
		}

		revRows, _ := brainDB.Query(`SELECT from_path FROM codemap_edges WHERE to_path = ?`, path)
		var revDeps []string
		if revRows != nil {
			for revRows.Next() {
				var d string
				if err := revRows.Scan(&d); err != nil {
					continue
				}
				revDeps = append(revDeps, d)
			}
			_ = revRows.Err()
			revRows.Close()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"node":       n,
			"deps":       deps,
			"dependents": revDeps,
		})
	}
}

// CodemapImpactHandler GET /api/codemap/impact?path=...
// Return transitive dependents — "jika file ini berubah, apa yang terdampak?"
func CodemapImpactHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", 400)
			return
		}
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		visited := map[string]bool{path: true}
		queue := []string{path}
		type impactNode struct {
			Path   string `json:"path"`
			Degree int    `json:"degree"`
		}
		var impact []impactNode

		for degree := 1; len(queue) > 0 && degree <= 5; degree++ {
			var next []string
			for _, cur := range queue {
				rows, _ := brainDB.Query(`SELECT from_path FROM codemap_edges WHERE to_path = ?`, cur)
				if rows == nil {
					continue
				}
				for rows.Next() {
					var dep string
					if err := rows.Scan(&dep); err != nil {
						continue
					}
					if !visited[dep] {
						visited[dep] = true
						impact = append(impact, impactNode{Path: dep, Degree: degree})
						next = append(next, dep)
					}
				}
				_ = rows.Err()
				rows.Close()
			}
			queue = next
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"source":         path,
			"impact":         impact,
			"total_impacted": len(impact),
		})
	}
}

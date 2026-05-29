// codemap_health.go — Health, Docs, Roots, Expand, Zombies handlers.
package guiapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/teetah2402/flowork/brain/db"
)

// CodemapHealthHandler GET /api/codemap/health
// Return health report semua file, sorted by score ascending (worst first).
func CodemapHealthHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		rows, err := brainDB.Query(`
			SELECT path, name, file_type, health_score, line_count, has_tests, has_docs, issues
			FROM codemap_nodes
			ORDER BY health_score ASC`)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		defer rows.Close()

		type healthRow struct {
			Path        string   `json:"path"`
			Name        string   `json:"name"`
			FileType    string   `json:"file_type"`
			HealthScore float64  `json:"health_score"`
			LineCount   int      `json:"line_count"`
			HasTests    bool     `json:"has_tests"`
			HasDocs     bool     `json:"has_docs"`
			Issues      []string `json:"issues"`
		}

		var report []healthRow
		for rows.Next() {
			var hr healthRow
			var issJSON string
			var hasTests, hasDocs int
			if err := rows.Scan(&hr.Path, &hr.Name, &hr.FileType, &hr.HealthScore,
				&hr.LineCount, &hasTests, &hasDocs, &issJSON); err != nil {
				continue
			}
			hr.HasTests = hasTests == 1
			hr.HasDocs = hasDocs == 1
			if err := json.Unmarshal([]byte(issJSON), &hr.Issues); err != nil {
				log.Printf("codemap: unmarshal issues %q: %v", hr.Path, err)
			}
			report = append(report, hr)
		}
		if err := rows.Err(); err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		var total float64
		for _, hr := range report {
			total += hr.HealthScore
		}
		avg := 0.0
		if len(report) > 0 {
			avg = total / float64(len(report))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"files":        report,
			"total_files":  len(report),
			"avg_health":   avg,
			"generated_at": time.Now().Format(time.RFC3339),
		})
	}
}

// CodemapDocsHandler GET /api/codemap/docs?path=...
// Return auto-generated markdown docs untuk satu file.
func CodemapDocsHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", 400)
			return
		}
		docPath := filepath.Join(ws, "docs", "auto", replaceSlash(path)+".md")
		data, err := readFileOrEmpty(docPath)
		if err != nil {
			brainDB, _ := db.Shared(ws)
			if brainDB != nil {
				var docCmt string
				brainDB.QueryRow(`SELECT doc_comment FROM codemap_nodes WHERE path = ?`, path).Scan(&docCmt)
				if docCmt != "" {
					w.Header().Set("Content-Type", "text/plain")
					w.Write([]byte(docCmt))
					return
				}
			}
			http.Error(w, "docs not found — run reindex first", 404)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		w.Write(data)
	}
}

// CodemapRootsHandler GET /api/codemap/roots
// Return entry-point files — file yang tidak di-import siapapun (dependent_count = 0).
func CodemapRootsHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		rows, err := brainDB.Query(`
			SELECT n.path, n.name, n.pkg, n.file_type, n.line_count, n.size_bytes,
			       n.exported_symbols, n.doc_comment, n.health_score, n.has_tests,
			       n.has_docs, n.issues, n.last_indexed,
			       (SELECT COUNT(*) FROM codemap_edges WHERE from_path = n.path) as dep_count,
			       (SELECT COUNT(*) FROM codemap_edges WHERE to_path   = n.path) as rev_count
			FROM codemap_nodes n
			WHERE (SELECT COUNT(*) FROM codemap_edges WHERE to_path = n.path) = 0
			ORDER BY dep_count DESC, n.path`)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		defer rows.Close()

		var nodes []codemapNode
		for rows.Next() {
			var n codemapNode
			var symJSON, issJSON string
			var hasTests, hasDocs int
			if err := rows.Scan(
				&n.Path, &n.Name, &n.Pkg, &n.FileType, &n.LineCount, &n.SizeBytes,
				&symJSON, &n.DocComment, &n.HealthScore, &hasTests, &hasDocs,
				&issJSON, &n.LastIndexed, &n.DependencyCount, &n.DependentCount,
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
			nodes = append(nodes, n)
		}
		if err := rows.Err(); err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"roots": nodes,
			"count": len(nodes),
		})
	}
}

// CodemapExpandHandler GET /api/codemap/expand?path=...
// Return node + semua direct neighbors beserta edges di antara mereka.
func CodemapExpandHandler(ws string) http.HandlerFunc {
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

		paths := []string{path}
		edgeRows, _ := brainDB.Query(
			`SELECT from_path, to_path FROM codemap_edges WHERE from_path = ? OR to_path = ?`, path, path)
		type edge struct{ From, To string }
		var edges []edge
		if edgeRows != nil {
			for edgeRows.Next() {
				var e edge
				if err := edgeRows.Scan(&e.From, &e.To); err != nil {
					continue
				}
				edges = append(edges, e)
				if e.From != path {
					paths = append(paths, e.From)
				}
				if e.To != path {
					paths = append(paths, e.To)
				}
			}
			_ = edgeRows.Err()
			edgeRows.Close()
		}

		seen := map[string]bool{}
		unique := paths[:0]
		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				unique = append(unique, p)
			}
		}

		var nodes []codemapNode
		for _, p := range unique {
			var n codemapNode
			var symJSON, issJSON string
			var hasTests, hasDocs int
			err := brainDB.QueryRow(`
				SELECT n.path, n.name, n.pkg, n.file_type, n.line_count, n.size_bytes,
				       n.exported_symbols, n.doc_comment, n.health_score, n.has_tests,
				       n.has_docs, n.issues, n.last_indexed,
				       (SELECT COUNT(*) FROM codemap_edges WHERE from_path = n.path),
				       (SELECT COUNT(*) FROM codemap_edges WHERE to_path   = n.path)
				FROM codemap_nodes n WHERE n.path = ?`, p).Scan(
				&n.Path, &n.Name, &n.Pkg, &n.FileType, &n.LineCount, &n.SizeBytes,
				&symJSON, &n.DocComment, &n.HealthScore, &hasTests, &hasDocs,
				&issJSON, &n.LastIndexed, &n.DependencyCount, &n.DependentCount,
			)
			if err != nil {
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
			nodes = append(nodes, n)
		}

		type outEdge struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		outEdges := make([]outEdge, len(edges))
		for i, e := range edges {
			outEdges[i] = outEdge{From: e.From, To: e.To}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"center": path,
			"nodes":  nodes,
			"edges":  outEdges,
		})
	}
}

// CodemapZombiesHandler GET /api/codemap/zombies
// Return file "zombie" — tidak di-import siapapun DAN tidak mengimport siapapun.
func CodemapZombiesHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		rows, err := brainDB.Query(`
			SELECT n.path, n.name, n.file_type, n.line_count, n.health_score, n.last_indexed,
			       n.pkg, n.exported_symbols
			FROM codemap_nodes n
			WHERE (SELECT COUNT(*) FROM codemap_edges WHERE from_path = n.path) = 0
			  AND (SELECT COUNT(*) FROM codemap_edges WHERE to_path   = n.path) = 0
			ORDER BY n.line_count DESC`)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		defer rows.Close()

		type zombie struct {
			Path            string   `json:"path"`
			Name            string   `json:"name"`
			FileType        string   `json:"file_type"`
			LineCount       int      `json:"line_count"`
			HealthScore     float64  `json:"health_score"`
			LastIndexed     string   `json:"last_indexed"`
			Pkg             string   `json:"pkg"`
			ExportedSymbols int      `json:"exported_symbols"`
			SiblingCount    int      `json:"sibling_count"`
			Confidence      string   `json:"confidence"`
			Notes           []string `json:"notes"`
		}

		var zombies []zombie
		for rows.Next() {
			var z zombie
			var exportedJSON string
			if err := rows.Scan(&z.Path, &z.Name, &z.FileType, &z.LineCount, &z.HealthScore, &z.LastIndexed,
				&z.Pkg, &exportedJSON); err != nil {
				continue
			}

			var exported []string
			if err := json.Unmarshal([]byte(exportedJSON), &exported); err != nil {
				log.Printf("codemap: unmarshal exported_symbols %q: %v", z.Path, err)
			}
			z.ExportedSymbols = len(exported)

			dirPrefix := z.Path
			if idx := strings.LastIndexAny(z.Path, "/\\"); idx >= 0 {
				dirPrefix = z.Path[:idx]
			} else {
				dirPrefix = ""
			}
			var sibCount int
			if dirPrefix != "" {
				brainDB.QueryRow(`
					SELECT COUNT(*) FROM codemap_nodes
					WHERE path LIKE ? || '/%'
					  AND path NOT LIKE ? || '/%/%'
					  AND file_type = ?
					  AND path != ?`,
					dirPrefix, dirPrefix, z.FileType, z.Path,
				).Scan(&sibCount)
			}
			z.SiblingCount = sibCount

			var notes []string
			if z.SiblingCount > 0 {
				notes = append(notes, fmt.Sprintf("Ada %d file lain di paket yang sama — kemungkinan dipakai intra-package", z.SiblingCount))
			}
			if z.ExportedSymbols > 0 {
				notes = append(notes, fmt.Sprintf("Punya %d simbol yang di-export — kemungkinan dipakai package lain tapi resolver gagal", z.ExportedSymbols))
			}
			if z.Name == "main.go" {
				notes = append(notes, "File main.go — entry point, wajar tidak di-import")
			}
			if z.LineCount > 200 {
				notes = append(notes, fmt.Sprintf("File besar (%d baris) — jarang jadi dead code sejati", z.LineCount))
			}
			if z.FileType == "js" {
				notes = append(notes, "File JS — parser hanya track relative imports, npm imports tidak terhitung")
			}

			isSpecialName := z.Name == "main.go" || z.Name == "init.go" || z.Name == "doc.go" ||
				z.Name == "errors.go" || z.Name == "types.go" || z.Name == "constants.go"

			if z.SiblingCount > 0 || z.ExportedSymbols > 5 || isSpecialName {
				z.Confidence = "LOW"
			} else if z.ExportedSymbols == 0 && z.SiblingCount == 0 && z.LineCount < 50 {
				z.Confidence = "HIGH"
			} else {
				z.Confidence = "MEDIUM"
			}

			zombies = append(zombies, z)
		}
		if err := rows.Err(); err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		highCount := 0
		for _, z := range zombies {
			if z.Confidence == "HIGH" {
				highCount++
			}
		}
		if zombies == nil {
			zombies = []zombie{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"zombies":         zombies,
			"count":           len(zombies),
			"high_confidence": highCount,
			"warning":         "Zombie dengan confidence LOW/MEDIUM kemungkinan false positive. Verifikasi manual sebelum hapus.",
		})
	}
}

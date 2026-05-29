// flowork-codemap-cli — CLI untuk reindex + audit codebase map.
//
// Plug-and-play tool buat warga AI / Ayah panggil dari shell tanpa lewat
// GUI HTTP auth. Operasi:
//   reindex  — full reindex (DELETE + walk + parse + edges)
//   stats    — dump node/edge count breakdown
//   orphans  — list file orphan (no in/out edges) per file_type
//
// Usage:
//   flowork-codemap-cli reindex
//   flowork-codemap-cli stats
//   flowork-codemap-cli orphans [go|js|ts]
//
// Workspace dari env FLOWORK_WORKSPACE atau --ws flag.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
)

func main() {
	wsFlag := flag.String("ws", "", "workspace (default: $FLOWORK_WORKSPACE or cwd)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	ws := resolveWorkspace(*wsFlag)
	cmd := args[0]

	switch cmd {
	case "reindex":
		runReindex(ws)
	case "stats":
		runStats(ws)
	case "orphans":
		ft := ""
		if len(args) > 1 {
			ft = args[1]
		}
		runOrphans(ws, ft)
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `flowork-codemap-cli — codebase map operations

Usage:
  flowork-codemap-cli [--ws PATH] <command> [args]

Commands:
  reindex            Full reindex: walk + parse + edges (overwrites existing)
  stats              Show node/edge counts (total + by file_type)
  orphans [type]     List orphan nodes (no edges). Optional filter: go|js|ts`)
}

func resolveWorkspace(flagWS string) string {
	if flagWS != "" {
		abs, _ := filepath.Abs(flagWS)
		return abs
	}
	if envWS := os.Getenv("FLOWORK_WORKSPACE"); envWS != "" {
		abs, _ := filepath.Abs(envWS)
		return abs
	}
	cwd, _ := os.Getwd()
	return cwd
}

func openDB(ws string) *sql.DB {
	d, err := db.Shared(ws)
	if err != nil {
		fmt.Fprintln(os.Stderr, "DB open failed:", err)
		os.Exit(1)
	}
	return d
}

func runReindex(ws string) {
	fmt.Printf("[codemap] Workspace: %s\n", ws)
	d := openDB(ws)
	ix := codeindex.NewIndexer(d, ws)

	// Auto-add sibling repos di project root (parity dengan getOrCreateIndexer
	// di guiapi/codemap_indexer.go). Print hanya untuk root yang actually
	// di-add (ada go.mod) — silent skip untuk dir tanpa go.mod.
	if filepath.Base(ws) == "floworkos-go" {
		projectRoot := filepath.Dir(ws)
		entries, _ := os.ReadDir(projectRoot)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			cand := filepath.Join(projectRoot, e.Name())
			if cand == ws {
				continue
			}
			if ix.AddRoot(cand) {
				fmt.Printf("[codemap] Extra root: %s\n", cand)
			}
		}
	}

	_ = codeindex.EnsureLayerColumn(d)
	fmt.Println("[codemap] Reindexing... (full walk + parse)")
	stats, err := ix.IndexAll()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reindex failed:", err)
		os.Exit(1)
	}
	fmt.Printf("[codemap] Done. files_indexed=%d skipped=%d edges=%d errors=%d duration=%s\n",
		stats.FilesIndexed, stats.FilesSkipped, stats.EdgesCreated, stats.Errors, stats.Duration)
	fmt.Println()
	dumpStats(d)
}

func runStats(ws string) {
	d := openDB(ws)
	dumpStats(d)
}

func dumpStats(d *sql.DB) {
	var nodeCount, edgeCount int
	d.QueryRow(`SELECT COUNT(*) FROM codemap_nodes`).Scan(&nodeCount)
	d.QueryRow(`SELECT COUNT(*) FROM codemap_edges`).Scan(&edgeCount)
	fmt.Printf("Total: %d nodes · %d edges (ratio: %.2f edges/node)\n", nodeCount, edgeCount, ratio(edgeCount, nodeCount))

	fmt.Println("\nNodes by file_type:")
	rows, _ := d.Query(`SELECT file_type, COUNT(*) FROM codemap_nodes GROUP BY file_type ORDER BY 2 DESC`)
	for rows.Next() {
		var ft string
		var n int
		rows.Scan(&ft, &n)
		fmt.Printf("  %-8s %d\n", ft, n)
	}
	rows.Close()

	fmt.Println("\nEdges by source file_type → target file_type:")
	rows, _ = d.Query(`
		SELECT n1.file_type AS ft_from, n2.file_type AS ft_to, COUNT(*) AS c
		FROM codemap_edges e
		JOIN codemap_nodes n1 ON e.from_path = n1.path
		JOIN codemap_nodes n2 ON e.to_path = n2.path
		GROUP BY ft_from, ft_to ORDER BY c DESC`)
	for rows.Next() {
		var ftF, ftT string
		var n int
		rows.Scan(&ftF, &ftT, &n)
		fmt.Printf("  %-8s -> %-8s %d\n", ftF, ftT, n)
	}
	rows.Close()

	fmt.Println("\nOrphan nodes (no edges) by file_type:")
	rows, _ = d.Query(`
		SELECT n.file_type, COUNT(*)
		FROM codemap_nodes n
		WHERE NOT EXISTS (SELECT 1 FROM codemap_edges e WHERE e.from_path = n.path)
		  AND NOT EXISTS (SELECT 1 FROM codemap_edges e WHERE e.to_path   = n.path)
		GROUP BY n.file_type ORDER BY 2 DESC`)
	for rows.Next() {
		var ft string
		var n int
		rows.Scan(&ft, &n)
		fmt.Printf("  %-8s %d\n", ft, n)
	}
	rows.Close()
}

func runOrphans(ws, ft string) {
	d := openDB(ws)
	q := `SELECT path, line_count FROM codemap_nodes n
	      WHERE NOT EXISTS (SELECT 1 FROM codemap_edges e WHERE e.from_path = n.path)
	        AND NOT EXISTS (SELECT 1 FROM codemap_edges e WHERE e.to_path   = n.path)`
	args := []any{}
	if ft != "" {
		q += " AND file_type = ?"
		args = append(args, ft)
	}
	q += " ORDER BY line_count DESC LIMIT 100"
	rows, err := d.Query(q, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query failed:", err)
		os.Exit(1)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var p string
		var lc int
		rows.Scan(&p, &lc)
		fmt.Printf("[%5d lines] %s\n", lc, p)
		count++
	}
	fmt.Printf("\n%d orphan(s) listed (top 100, sorted by lines desc)\n", count)
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

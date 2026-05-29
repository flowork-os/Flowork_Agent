// Package brain — codemap_columns.go: canonical const registry untuk schema
// codemap_* tables di flowork-brain.sqlite (codeindex + GUI handler + tool).
//
// Why: per effekdomino.md #43 (MEDIUM partial — opus-3 round 5), 11+ callsite
// reference SQL string literal `codemap_nodes` / `codemap_edges` / column
// `path`/`health_score`/`doc_comment`/`exported_symbols` tersebar di:
//   - internal/codeindex/ (indexer + parser + diffhighlight + docgen + tour)
//   - internal/guiapi/codemap_*.go (3 handler)
//   - internal/tools/codemap_tool*.go (4 tool)
//   - frontend tabs/codemap.js (display)
//
// Solution: const block kanonikal untuk tabel + key column. Caller wajib pakai
// const, BUKAN string literal. SQL string composition tetap pakai const
// concatenation (Go ngga punya prepared-statement-with-table-name).
//
// Migration sample: per fix branch refactor/fix-43-codemap-columns. Sisa 11
// caller progressive (touch-by-touch per Standar 1.6).
//
// Pattern:
//
//	// Sebelum:
//	rows, _ := db.Query("SELECT path, health_score FROM codemap_nodes ORDER BY path")
//
//	// Sesudah:
//	rows, _ := db.Query(fmt.Sprintf(
//	    "SELECT %s, %s FROM %s ORDER BY %s",
//	    brain.ColCodemapPath, brain.ColCodemapHealthScore,
//	    brain.TableCodemapNodes, brain.ColCodemapPath))

package brain

// Codemap table names (canonical).
const (
	// TableCodemapNodes — node per file/symbol di codebase. Schema:
	//   path TEXT PRIMARY KEY, kind, package, doc_comment, exported_symbols,
	//   line_count, content_hash, layer, health_score (0-100).
	TableCodemapNodes = "codemap_nodes"

	// TableCodemapEdges — directed edge import graph. Schema:
	//   from_path, to_path, edge_type ('import' | 'call'), weight.
	TableCodemapEdges = "codemap_edges"
)

// Codemap column names (canonical, sering di-query).
const (
	// Common columns shared codemap_nodes
	ColCodemapPath            = "path"
	ColCodemapKind            = "kind"
	ColCodemapPackage         = "package"
	ColCodemapDocComment      = "doc_comment"
	ColCodemapExportedSymbols = "exported_symbols"
	ColCodemapLineCount       = "line_count"
	ColCodemapContentHash     = "content_hash"
	ColCodemapLayer           = "layer"
	ColCodemapHealthScore     = "health_score"

	// codemap_edges columns
	ColCodemapEdgeFromPath  = "from_path"
	ColCodemapEdgeToPath    = "to_path"
	ColCodemapEdgeType      = "edge_type"
	ColCodemapEdgeWeight    = "weight"
)

// AllCodemapTables returns canonical list — used by drift scanner +
// migration framework Phase 4.
func AllCodemapTables() []string {
	return []string{TableCodemapNodes, TableCodemapEdges}
}

// AllCodemapNodeColumns returns canonical column list untuk codemap_nodes.
func AllCodemapNodeColumns() []string {
	return []string{
		ColCodemapPath, ColCodemapKind, ColCodemapPackage,
		ColCodemapDocComment, ColCodemapExportedSymbols,
		ColCodemapLineCount, ColCodemapContentHash,
		ColCodemapLayer, ColCodemapHealthScore,
	}
}

// AllCodemapEdgeColumns returns canonical column list untuk codemap_edges.
func AllCodemapEdgeColumns() []string {
	return []string{
		ColCodemapEdgeFromPath, ColCodemapEdgeToPath,
		ColCodemapEdgeType, ColCodemapEdgeWeight,
	}
}

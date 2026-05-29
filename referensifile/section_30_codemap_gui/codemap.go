// codemap.go — Response types untuk codemap API.
// Handlers split ke: codemap_indexer.go, codemap_graph.go,
// codemap_health.go, codemap_reindex.go, codemap_gitnexus.go
package guiapi

// codemapNode adalah response type untuk satu file node di dependency graph.
type codemapNode struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	Pkg             string   `json:"pkg"`
	FileType        string   `json:"file_type"`
	LineCount       int      `json:"line_count"`
	SizeBytes       int      `json:"size_bytes"`
	ExportedSymbols []string `json:"exported_symbols"`
	DocComment      string   `json:"doc_comment"`
	HealthScore     float64  `json:"health_score"`
	HasTests        bool     `json:"has_tests"`
	HasDocs         bool     `json:"has_docs"`
	Issues          []string `json:"issues"`
	DependencyCount int      `json:"dependency_count"` // outgoing
	DependentCount  int      `json:"dependent_count"`  // incoming
	LastIndexed     string   `json:"last_indexed"`
	// 2026-05-06 (adopt Understand-Anything fitur 1): layer auto-classifier
	// (UI / API / Data / Service / Util / Test / Core). Heuristic
	// path-based via codeindex.ClassifyLayer. Nil-safe — kosong default Core.
	Layer string `json:"layer"`
	// 2026-05-06 (fitur 3 diff highlight): true kalau file di-touch oleh
	// commit terakhir N (default 5). Frontend pakai untuk overlay nyala.
	RecentlyTouched bool `json:"recently_touched"`
}

// codemapEdge adalah response type untuk satu dependency edge.
type codemapEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	EdgeType string `json:"edge_type"`
}

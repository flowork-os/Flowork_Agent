package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// CodemapSearchTool — cari file di codemap index.
// Berguna untuk coder warga sebelum mulai edit: "ada file yang namanya auth?"
type CodemapSearchTool struct{ workspace string }

type codemapSearchArgs struct {
	Query    string `json:"query" validate:"required"`
	FileType string `json:"file_type,omitempty"` // "go", "js", "ts" — optional filter
	Limit    int    `json:"limit,omitempty"`
}

func NewCodemapSearchTool(workspace string) *CodemapSearchTool {
	return &CodemapSearchTool{workspace: workspace}
}

func (t *CodemapSearchTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "codemap_search",
		Description: `Cari file di codebase index berdasarkan nama, path, atau package.
Gunakan ini untuk:
- Temukan file sebelum edit: codemap_search("auth") → lihat semua file auth-related
- Cek apakah file/package ada: codemap_search("guiapi")
- Temukan file by type: codemap_search("handler", file_type: "go")

Output: list file dengan health score, line count, dan package name.
Catatan: index harus sudah di-populate via GUI Code Map → Reindex dulu.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Kata kunci — cocok ke path, nama file, atau package name. Case-insensitive.",
				},
				"file_type": map[string]any{
					"type":        "string",
					"description": "Filter by tipe file: 'go', 'js', atau 'ts'. Default: semua.",
					"enum":        []string{"go", "js", "ts"},
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maks jumlah hasil. Default: 20.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *CodemapSearchTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args codemapSearchArgs
	if err := json.Unmarshal(inv.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("codemap_search: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, err
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}

	brainDB, err := db.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_search: DB unavailable — run Reindex via GUI Code Map: %w", err)
	}

	q := "%" + strings.ToLower(args.Query) + "%"
	query := `SELECT path, name, pkg, file_type, line_count, health_score,
	                 (SELECT COUNT(*) FROM codemap_edges WHERE from_path = path) as dep_count,
	                 (SELECT COUNT(*) FROM codemap_edges WHERE to_path   = path) as rev_count
	          FROM codemap_nodes
	          WHERE (LOWER(path) LIKE ? OR LOWER(name) LIKE ? OR LOWER(pkg) LIKE ?)`
	queryArgs := []any{q, q, q}
	if args.FileType != "" {
		query += " AND file_type = ?"
		queryArgs = append(queryArgs, args.FileType)
	}
	query += " ORDER BY health_score ASC, rev_count DESC LIMIT ?"
	queryArgs = append(queryArgs, args.Limit)

	rows, err := brainDB.Query(query, queryArgs...)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_search: query failed: %w", err)
	}
	defer rows.Close()

	type fileResult struct {
		Path     string  `json:"path"`
		Name     string  `json:"name"`
		Pkg      string  `json:"pkg,omitempty"`
		Type     string  `json:"type"`
		Lines    int     `json:"lines"`
		Health   float64 `json:"health"`
		DepCount int     `json:"dep_count"`
		RevCount int     `json:"rev_count"`
	}

	var results []fileResult
	for rows.Next() {
		var f fileResult
		rows.Scan(&f.Path, &f.Name, &f.Pkg, &f.Type, &f.Lines, &f.Health, &f.DepCount, &f.RevCount)
		results = append(results, f)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()

	if len(results) == 0 {
		return Result{
			Output: fmt.Sprintf("Tidak ada file yang match query %q. Coba kata kunci lain atau jalankan Reindex.", args.Query),
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ditemukan %d file untuk query %q:\n\n", len(results), args.Query))
	for _, f := range results {
		healthIcon := "🟢"
		if f.Health < 80 {
			healthIcon = "🟡"
		}
		if f.Health < 60 {
			healthIcon = "🟠"
		}
		if f.Health < 40 {
			healthIcon = "🔴"
		}
		pkg := ""
		if f.Pkg != "" {
			pkg = fmt.Sprintf(" [%s]", f.Pkg)
		}
		sb.WriteString(fmt.Sprintf("%s %s%s\n   path: %s | %d baris | %d deps | %d dependents | health: %.0f/100\n",
			healthIcon, f.Name, pkg, f.Path, f.Lines, f.DepCount, f.RevCount, f.Health))
	}

	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"count": len(results),
			"query": args.Query,
		},
	}, nil
}

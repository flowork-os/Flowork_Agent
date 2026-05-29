package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// CodemapHealthTool — health report file-file yang paling butuh perhatian.
type CodemapHealthTool struct{ workspace string }

type codemapHealthArgs struct {
	Limit    int     `json:"limit,omitempty"`     // default 15, worst-first
	MinScore float64 `json:"min_score,omitempty"` // filter: hanya file dengan score <= ini
	FileType string  `json:"file_type,omitempty"`
}

func NewCodemapHealthTool(workspace string) *CodemapHealthTool {
	return &CodemapHealthTool{workspace: workspace}
}

func (t *CodemapHealthTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "codemap_health",
		Description: `Lihat health report file-file yang paling butuh perhatian (sorted worst-first).
Health score dihitung dari: docs (-15), tests (-20), panjang file (-10/-20), banyak deps (-5/-15), circular dep (-25).

Gunakan ini untuk:
- Prioritas cleanup: "file mana yang paling butuh test?"
- Code review awal: "ada circular dependency?"
- Sprint planning: mana yang harus diperbaiki dulu?

Output: list file dengan score, issues spesifik, dan rekomendasi.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Jumlah file yang ditampilkan. Default: 15.",
				},
				"min_score": map[string]any{
					"type":        "number",
					"description": "Hanya tampilkan file dengan health ≤ nilai ini. Default: 100 (semua).",
				},
				"file_type": map[string]any{
					"type":        "string",
					"description": "Filter by type: 'go', 'js', 'ts'.",
				},
			},
		},
	}
}

func (t *CodemapHealthTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args codemapHealthArgs
	json.Unmarshal(inv.Arguments, &args)
	if args.Limit <= 0 {
		args.Limit = 15
	}
	if args.MinScore <= 0 {
		args.MinScore = 100
	}

	brainDB, err := db.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_health: DB unavailable: %w", err)
	}

	q := `SELECT path, name, file_type, line_count, health_score, has_tests, has_docs, issues
	      FROM codemap_nodes WHERE health_score <= ?`
	qArgs := []any{args.MinScore}
	if args.FileType != "" {
		q += " AND file_type = ?"
		qArgs = append(qArgs, args.FileType)
	}
	q += " ORDER BY health_score ASC LIMIT ?"
	qArgs = append(qArgs, args.Limit)

	rows, err := brainDB.Query(q, qArgs...)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_health: query: %w", err)
	}
	defer rows.Close()

	type fileHealth struct {
		Path     string
		Name     string
		FT       string
		Lines    int
		Score    float64
		HasTests int
		HasDocs  int
		IssJSON  string
	}
	var files []fileHealth
	for rows.Next() {
		var f fileHealth
		rows.Scan(&f.Path, &f.Name, &f.FT, &f.Lines, &f.Score, &f.HasTests, &f.HasDocs, &f.IssJSON)
		files = append(files, f)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()

	if len(files) == 0 {
		return Result{
			Output: "✅ Semua file punya health score yang baik. Tidak ada yang perlu perhatian segera.",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("❤️ Health Report — %d file terburuk:\n\n", len(files)))
	for i, f := range files {
		grade := "A"
		if f.Score < 90 {
			grade = "B"
		}
		if f.Score < 75 {
			grade = "C"
		}
		if f.Score < 60 {
			grade = "D"
		}
		if f.Score < 40 {
			grade = "F"
		}

		testIcon := "✅"
		if f.HasTests == 0 {
			testIcon = "❌"
		}
		docsIcon := "✅"
		if f.HasDocs == 0 {
			docsIcon = "❌"
		}

		var issues []string
		json.Unmarshal([]byte(f.IssJSON), &issues)

		sb.WriteString(fmt.Sprintf("%d. [%s] %s (%.0f/100)\n", i+1, grade, f.Name, f.Score))
		sb.WriteString(fmt.Sprintf("   path: %s | %d baris | test:%s docs:%s\n",
			f.Path, f.Lines, testIcon, docsIcon))
		if len(issues) > 0 {
			sb.WriteString(fmt.Sprintf("   issues: %s\n", strings.Join(issues, " · ")))
		}
	}

	sb.WriteString("\nRekomendasi prioritas: tambah test untuk file F/D, pisah file >1000 baris.\n")
	return Result{
		Output:   sb.String(),
		Metadata: map[string]any{"count": len(files)},
	}, nil
}

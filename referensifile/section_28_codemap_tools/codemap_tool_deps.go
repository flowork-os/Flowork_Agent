package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// CodemapDepsTool — lihat semua dependency satu file.
type CodemapDepsTool struct{ workspace string }

type codemapDepsArgs struct {
	Path string `json:"path" validate:"required"`
}

func NewCodemapDepsTool(workspace string) *CodemapDepsTool {
	return &CodemapDepsTool{workspace: workspace}
}

func (t *CodemapDepsTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "codemap_deps",
		Description: `Lihat dependency graph satu file: file apa saja yang dia import, dan file mana yang mengimport dia.
Gunakan ini untuk:
- Sebelum refactor: "siapa yang bergantung ke file ini?"
- Debug circular dependency
- Pahami scope perubahan sebelum edit

Output: daftar imports (outgoing) + dependents (incoming) dengan path lengkap.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path relatif file dari workspace root, contoh: 'internal/guiapi/codemap.go'",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *CodemapDepsTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args codemapDepsArgs
	if err := json.Unmarshal(inv.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("codemap_deps: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, err
	}

	brainDB, err := db.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_deps: DB unavailable: %w", err)
	}

	// Cek node exists
	var name, pkg, ft string
	var lines int
	var health float64
	err = brainDB.QueryRow(
		`SELECT name, pkg, file_type, line_count, health_score FROM codemap_nodes WHERE path = ?`,
		args.Path,
	).Scan(&name, &pkg, &ft, &lines, &health)
	if err != nil {
		return Result{
			Output: fmt.Sprintf("File %q tidak ada di index. Jalankan Reindex atau cek path.", args.Path),
		}, nil
	}

	// Imports (outgoing)
	depRows, _ := brainDB.Query(`SELECT to_path FROM codemap_edges WHERE from_path = ? ORDER BY to_path`, args.Path)
	var deps []string
	if depRows != nil {
		for depRows.Next() {
			var p string
			depRows.Scan(&p)
			deps = append(deps, p)
		}
		// Sprint 3.5d (BUG-C15 fix): rows.Err() check
		_ = depRows.Err()
		depRows.Close()
	}

	// Dependents (incoming)
	revRows, _ := brainDB.Query(`SELECT from_path FROM codemap_edges WHERE to_path = ? ORDER BY from_path`, args.Path)
	var revDeps []string
	if revRows != nil {
		for revRows.Next() {
			var p string
			if err := revRows.Scan(&p); err != nil {
				continue
			}
			revDeps = append(revDeps, p)
		}
		// Sprint 3.5d (BUG-C15 fix): rows.Err() check
		_ = revRows.Err()
		revRows.Close()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📄 %s (%s)\n   path: %s | pkg: %s | %d baris | health: %.0f/100\n\n",
		name, strings.ToUpper(ft), args.Path, pkg, lines, health))

	sb.WriteString(fmt.Sprintf("📥 IMPORTS (%d) — file yang di-import oleh %s:\n", len(deps), name))
	if len(deps) == 0 {
		sb.WriteString("   (tidak ada — file ini tidak mengimport file internal manapun)\n")
	} else {
		for _, d := range deps {
			sb.WriteString(fmt.Sprintf("   → %s\n", d))
		}
	}

	sb.WriteString(fmt.Sprintf("\n📤 DEPENDENTS (%d) — file yang mengimport %s:\n", len(revDeps), name))
	if len(revDeps) == 0 {
		sb.WriteString("   (tidak ada — file ini tidak di-import siapapun)\n")
		if len(deps) == 0 {
			sb.WriteString("   ⚠️  File ini ZOMBIE: tidak ada koneksi sama sekali!\n")
		}
	} else {
		for _, d := range revDeps {
			sb.WriteString(fmt.Sprintf("   ← %s\n", d))
		}
	}

	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"path":            args.Path,
			"dep_count":       len(deps),
			"dependent_count": len(revDeps),
			"is_zombie":       len(deps) == 0 && len(revDeps) == 0,
		},
	}, nil
}

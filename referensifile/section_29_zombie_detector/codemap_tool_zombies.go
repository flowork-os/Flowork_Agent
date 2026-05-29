package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// CodemapZombiesTool — temukan file zombie (orphan, dead code candidate).
type CodemapZombiesTool struct{ workspace string }

type codemapZombiesArgs struct {
	FileType string `json:"file_type,omitempty"`
}

func NewCodemapZombiesTool(workspace string) *CodemapZombiesTool {
	return &CodemapZombiesTool{workspace: workspace}
}

func (t *CodemapZombiesTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "codemap_zombies",
		Description: `Temukan file zombie — file yang tidak di-import siapapun DAN tidak mengimport siapapun.
File zombie = dead code candidate: aman untuk dihapus atau diisolasi.

Gunakan ini untuk:
- Code cleanup: "ada berapa file yang ga kepake?"
- Cek setelah refactor: apakah ada file yang jadi yatim piatu?
- Audit codebase kesehatan

Output: list file zombie dengan nama, path, size, dan health score.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_type": map[string]any{
					"type":        "string",
					"description": "Filter by type: 'go', 'js', 'ts'. Default: semua.",
				},
			},
		},
	}
}

func (t *CodemapZombiesTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args codemapZombiesArgs
	json.Unmarshal(inv.Arguments, &args)

	brainDB, err := db.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_zombies: DB unavailable: %w", err)
	}

	q := `SELECT path, name, file_type, line_count, size_bytes, health_score, pkg, exported_symbols
	      FROM codemap_nodes
	      WHERE (SELECT COUNT(*) FROM codemap_edges WHERE from_path = path) = 0
	        AND (SELECT COUNT(*) FROM codemap_edges WHERE to_path   = path) = 0`
	var qArgs []any
	if args.FileType != "" {
		q += " AND file_type = ?"
		qArgs = append(qArgs, args.FileType)
	}
	q += " ORDER BY line_count DESC"

	rows, err := brainDB.Query(q, qArgs...)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_zombies: query: %w", err)
	}
	defer rows.Close()

	type zombie struct {
		Path            string
		Name            string
		FT              string
		Lines           int
		Size            int64
		Health          float64
		Pkg             string
		ExportedSymbols int
		SiblingCount    int
		Confidence      string
		Notes           []string
	}
	var zombies []zombie
	for rows.Next() {
		var z zombie
		var exportedJSON string
		rows.Scan(&z.Path, &z.Name, &z.FT, &z.Lines, &z.Size, &z.Health, &z.Pkg, &exportedJSON)

		// Hitung exported_symbols dari JSON array
		var exported []string
		json.Unmarshal([]byte(exportedJSON), &exported)
		z.ExportedSymbols = len(exported)

		// Hitung SiblingCount
		dirPrefix := z.Path
		if idx := strings.LastIndexAny(z.Path, "/\\"); idx >= 0 {
			dirPrefix = z.Path[:idx]
		} else {
			dirPrefix = ""
		}
		if dirPrefix != "" {
			brainDB.QueryRow(`
				SELECT COUNT(*) FROM codemap_nodes
				WHERE path LIKE ? || '/%'
				  AND path NOT LIKE ? || '/%/%'
				  AND file_type = ?
				  AND path != ?`,
				dirPrefix, dirPrefix, z.FT, z.Path,
			).Scan(&z.SiblingCount)
		}

		// Bangun notes
		if z.SiblingCount > 0 {
			z.Notes = append(z.Notes, fmt.Sprintf("Ada %d file lain di paket yang sama — kemungkinan dipakai intra-package (parser tidak track ini)", z.SiblingCount))
		}
		if z.ExportedSymbols > 0 {
			z.Notes = append(z.Notes, fmt.Sprintf("Punya %d simbol yang di-export — kemungkinan dipakai package lain tapi resolver gagal", z.ExportedSymbols))
		}
		if z.Name == "main.go" {
			z.Notes = append(z.Notes, "File main.go — entry point, wajar tidak di-import")
		}
		if z.Lines > 200 {
			z.Notes = append(z.Notes, fmt.Sprintf("File besar (%d baris) — jarang jadi dead code sejati", z.Lines))
		}
		if z.FT == "js" {
			z.Notes = append(z.Notes, "File JS — parser hanya track relative imports, npm imports tidak terhitung")
		}

		// Logic confidence
		isSpecialName := z.Name == "main.go" || z.Name == "init.go" || z.Name == "doc.go" ||
			z.Name == "errors.go" || z.Name == "types.go" || z.Name == "constants.go"

		if z.SiblingCount > 0 || z.ExportedSymbols > 5 || isSpecialName {
			z.Confidence = "LOW"
		} else if z.ExportedSymbols == 0 && z.SiblingCount == 0 && z.Lines < 50 {
			z.Confidence = "HIGH"
		} else {
			z.Confidence = "MEDIUM"
		}

		zombies = append(zombies, z)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()

	if len(zombies) == 0 {
		return Result{
			Output:   "Tidak ada zombie file! Semua file punya minimal satu koneksi dependency.",
			Metadata: map[string]any{"count": 0},
		}, nil
	}

	var sb strings.Builder
	highCount := 0
	for _, z := range zombies {
		if z.Confidence == "HIGH" {
			highCount++
		}
	}
	sb.WriteString(fmt.Sprintf("%d ZOMBIE FILE ditemukan — tidak ada yang import, tidak import siapapun:\n", len(zombies)))
	sb.WriteString(fmt.Sprintf("HIGH confidence (kemungkinan dead code sejati): %d | MEDIUM: lihat notes | LOW: jangan hapus sembarangan\n\n", highCount))

	totalLines := 0
	for i, z := range zombies {
		sz := humanSizeZ(z.Size)
		sb.WriteString(fmt.Sprintf("%d. [%s] %s (%s)\n   path: %s | pkg: %s | %d baris | %s | health: %.0f/100 | exports: %d | siblings: %d\n",
			i+1, z.Confidence, z.Name, strings.ToUpper(z.FT), z.Path, z.Pkg, z.Lines, sz, z.Health, z.ExportedSymbols, z.SiblingCount))
		if len(z.Notes) > 0 {
			sb.WriteString(fmt.Sprintf("   notes: %s\n", strings.Join(z.Notes, " | ")))
		}
		totalLines += z.Lines
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d file zombie, %d baris kode.\n", len(zombies), totalLines))
	sb.WriteString("PERINGATAN: Zombie dengan confidence LOW/MEDIUM kemungkinan false positive. Verifikasi manual dengan codemap_deps sebelum hapus.\n")

	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"count":           len(zombies),
			"total_lines":     totalLines,
			"high_confidence": highCount,
		},
	}, nil
}

// humanSizeZ formats a byte count in human-readable units (B / KB / MB).
// Used by codemap_zombies output.
func humanSizeZ(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	if b < 1048576 {
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(b)/1048576)
}

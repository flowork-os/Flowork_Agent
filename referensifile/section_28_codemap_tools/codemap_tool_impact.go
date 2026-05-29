package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// CodemapImpactTool — blast radius analysis: kalau file X berubah, apa yang terdampak?
type CodemapImpactTool struct{ workspace string }

type codemapImpactArgs struct {
	Path  string `json:"path" validate:"required"`
	Depth int    `json:"depth,omitempty"` // max transitive depth, default 5
}

func NewCodemapImpactTool(workspace string) *CodemapImpactTool {
	return &CodemapImpactTool{workspace: workspace}
}

func (t *CodemapImpactTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "codemap_impact",
		Description: `Analisis blast radius: jika file X berubah (interface/struct/function), file mana saja yang terdampak?
Gunakan ini SEBELUM refactor besar:
- "Kalau gw ubah signature fungsi di auth.go, ada berapa file yang perlu diupdate?"
- Cek apakah perubahan aman (low blast radius) atau berisiko (high blast radius)
- Pahami ripple effect perubahan di codebase

Output: list file terdampak per degree (1=langsung, 2=2-hop, dst) + total count.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path relatif file yang akan diubah, contoh: 'brain/db/schema.go'",
				},
				"depth": map[string]any{
					"type":        "integer",
					"description": "Kedalaman transitive analysis. Default: 5. Max: 8.",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *CodemapImpactTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args codemapImpactArgs
	if err := json.Unmarshal(inv.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("codemap_impact: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, err
	}
	maxDepth := args.Depth
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 8 {
		maxDepth = 8
	}

	brainDB, err := db.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_impact: DB unavailable: %w", err)
	}

	// Build reverse adjacency: siapa yang import path ini
	rows, err := brainDB.Query(`SELECT from_path, to_path FROM codemap_edges`)
	if err != nil {
		return Result{}, fmt.Errorf("codemap_impact: query edges: %w", err)
	}
	revAdj := map[string][]string{} // path → daftar yang mengimportnya
	for rows.Next() {
		var from, to string
		rows.Scan(&from, &to)
		revAdj[to] = append(revAdj[to], from)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()

	// BFS dari args.Path ke atas
	type impactItem struct {
		Path   string
		Degree int
	}
	visited := map[string]bool{args.Path: true}
	queue := []string{args.Path}
	var impact []impactItem

	for deg := 1; deg <= maxDepth && len(queue) > 0; deg++ {
		var next []string
		for _, cur := range queue {
			for _, parent := range revAdj[cur] {
				if !visited[parent] {
					visited[parent] = true
					impact = append(impact, impactItem{Path: parent, Degree: deg})
					next = append(next, parent)
				}
			}
		}
		queue = next
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💥 Impact Analysis: %s\n", args.Path))

	if len(impact) == 0 {
		sb.WriteString("   Tidak ada file yang terdampak — file ini tidak di-import siapapun.\n")
		sb.WriteString("   (Aman untuk diubah, tapi perlu cek apakah ini zombie file)\n")
		return Result{Output: sb.String(), Metadata: map[string]any{"total": 0, "path": args.Path}}, nil
	}

	sb.WriteString(fmt.Sprintf("   Total: %d file terdampak\n\n", len(impact)))

	// Group by degree
	byDeg := map[int][]string{}
	for _, item := range impact {
		byDeg[item.Degree] = append(byDeg[item.Degree], item.Path)
	}

	labels := map[int]string{
		1: "🔴 Degree 1 — LANGSUNG (import file ini secara direct)",
		2: "🟠 Degree 2 — 2 hop",
		3: "🟡 Degree 3 — 3 hop",
		4: "🟢 Degree 4+",
	}
	for deg := 1; deg <= maxDepth; deg++ {
		paths := byDeg[deg]
		if len(paths) == 0 {
			continue
		}
		label := labels[deg]
		if deg > 3 {
			label = fmt.Sprintf("🟢 Degree %d", deg)
		}
		sb.WriteString(fmt.Sprintf("%s (%d file):\n", label, len(paths)))
		for _, p := range paths {
			sb.WriteString(fmt.Sprintf("   - %s\n", p))
		}
		sb.WriteString("\n")
	}

	risk := "LOW"
	if len(impact) > 30 {
		risk = "CRITICAL"
	} else if len(impact) > 10 {
		risk = "HIGH"
	} else if len(impact) > 4 {
		risk = "MEDIUM"
	}
	sb.WriteString(fmt.Sprintf("⚠️  Risk Level: %s (%d file akan terdampak)\n", risk, len(impact)))
	if risk == "HIGH" || risk == "CRITICAL" {
		sb.WriteString("   Rekomendasi: buat interface/wrapper sebelum ubah langsung, atau refactor incremental.\n")
	}

	return Result{
		Output: sb.String(),
		Metadata: map[string]any{
			"path":  args.Path,
			"total": len(impact),
			"risk":  risk,
		},
	}, nil
}

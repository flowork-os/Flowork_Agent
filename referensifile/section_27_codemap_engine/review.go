// Package codeindex — review.go
//
// CRG-inspired review context builder.
// Saat file berubah, generate "minimal context" yang harus dibaca AI.
// Ini inti dari token saving: AI hanya baca file yang relevan,
// bukan scan seluruh codebase.
//
// Prinsip dari code-review-graph, di-port ke native Go.
package codeindex

import (
	"database/sql"
	"fmt"
	"path"
	"strings"
)

// ImpactedFile — file yang terdampak perubahan.
type ImpactedFile struct {
	Path        string  `json:"path"`
	Degree      int     `json:"degree"` // 1=direct, 2=2-hop, dst
	HealthScore float64 `json:"health_score"`
	HasTests    bool    `json:"has_tests"`
	LineCount   int     `json:"line_count"`
	Pkg         string  `json:"pkg"`
}

// ReviewContext — output untuk AI: "ini yang perlu lo baca"
// Designed to be injected into LLM prompt for minimal-context review.
type ReviewContext struct {
	ChangedFiles  []string       `json:"changed_files"`
	ImpactedFiles []ImpactedFile `json:"impacted_files"`
	MissingTests  []string       `json:"missing_tests"`  // file terdampak TANPA test
	CircularDeps  []string       `json:"circular_deps"`  // file dengan circular dependency
	RiskLevel     string         `json:"risk_level"`     // LOW/MEDIUM/HIGH/CRITICAL
	TotalImpacted int            `json:"total_impacted"`
	Summary       string         `json:"summary"`        // rendered markdown for LLM injection
	TokenEstimate int            `json:"token_estimate"` // estimasi token yang dihemat
}

// BuildReviewContext — core CRG logic ported ke Go.
// Terima list changed files, trace blast radius, generate minimal context.
//
// Alur:
//  1. Untuk setiap changed file → BFS blast radius
//  2. Deduplicate across all changed files
//  3. Enrich: health, missing tests, circular deps
//  4. Generate summary markdown untuk LLM prompt injection
func BuildReviewContext(db *sql.DB, changedFiles []string, maxDepth int) (*ReviewContext, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > 8 {
		maxDepth = 8
	}

	ctx := &ReviewContext{
		ChangedFiles: changedFiles,
	}

	// 2026-04-30 BUGFIX: edges di codemap_edges store `to_path` = package
	// directory (e.g. "internal/tools"), bukan file path. BFS di file level
	// langsung selalu hasilin 0 impact. Fix: derive package dir dari changed
	// files, BFS di package level, lalu resolve impacted package → list file.
	//
	// Build package-level reverse adjacency: pkg_path → []file_paths_yg_import
	revAdjPkg := map[string][]string{}
	rows, err := db.Query(`SELECT from_path, to_path FROM codemap_edges`)
	if err != nil {
		return nil, fmt.Errorf("review: query edges: %w", err)
	}
	for rows.Next() {
		var from, to string
		rows.Scan(&from, &to)
		revAdjPkg[to] = append(revAdjPkg[to], from)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()

	// Resolve changed files → unique set of package paths
	changedPkgs := map[string]bool{}
	for _, cf := range changedFiles {
		pkgPath := path.Dir(strings.ReplaceAll(cf, "\\", "/"))
		changedPkgs[pkgPath] = true
	}

	// BFS di package-level. Setiap iter: untuk tiap pkg yg lagi diproses,
	// cari file yang import pkg itu (= revAdjPkg[pkg]). Tiap file hasil =
	// impacted file dengan degree = current BFS depth. File-nya juga punya
	// pkg sendiri (path.Dir-nya), jadi loop terus ke degree berikutnya.
	visitedFiles := map[string]bool{}
	for _, cf := range changedFiles {
		visitedFiles[cf] = true
	}
	visitedPkgs := map[string]bool{}
	for p := range changedPkgs {
		visitedPkgs[p] = true
	}

	queuePkgs := make([]string, 0, len(changedPkgs))
	for p := range changedPkgs {
		queuePkgs = append(queuePkgs, p)
	}

	for deg := 1; deg <= maxDepth && len(queuePkgs) > 0; deg++ {
		var nextPkgs []string
		for _, curPkg := range queuePkgs {
			for _, parentFile := range revAdjPkg[curPkg] {
				if visitedFiles[parentFile] {
					continue
				}
				visitedFiles[parentFile] = true

				imp := ImpactedFile{
					Path:   parentFile,
					Degree: deg,
				}
				db.QueryRow(`SELECT health_score, has_tests, line_count, pkg
					FROM codemap_nodes WHERE path = ?`, parentFile).Scan(
					&imp.HealthScore, &imp.HasTests, &imp.LineCount, &imp.Pkg)
				ctx.ImpactedFiles = append(ctx.ImpactedFiles, imp)

				if !imp.HasTests {
					ctx.MissingTests = append(ctx.MissingTests, parentFile)
				}

				// Kalau parent file ada di package yang belum di-visit, queue
				// package itu untuk BFS berikutnya.
				parentPkg := path.Dir(strings.ReplaceAll(parentFile, "\\", "/"))
				if !visitedPkgs[parentPkg] {
					visitedPkgs[parentPkg] = true
					nextPkgs = append(nextPkgs, parentPkg)
				}
			}
		}
		queuePkgs = nextPkgs
	}

	// Detect circular deps among impacted files
	for _, imp := range ctx.ImpactedFiles {
		var circCount int
		db.QueryRow(`
			SELECT COUNT(*) FROM codemap_edges e1
			JOIN codemap_edges e2 ON e1.from_path = e2.to_path AND e1.to_path = e2.from_path
			WHERE e1.from_path = ?`, imp.Path).Scan(&circCount)
		if circCount > 0 {
			ctx.CircularDeps = append(ctx.CircularDeps, imp.Path)
		}
	}

	ctx.TotalImpacted = len(ctx.ImpactedFiles)

	// Risk level
	switch {
	case ctx.TotalImpacted > 30:
		ctx.RiskLevel = "CRITICAL"
	case ctx.TotalImpacted > 10:
		ctx.RiskLevel = "HIGH"
	case ctx.TotalImpacted > 4:
		ctx.RiskLevel = "MEDIUM"
	default:
		ctx.RiskLevel = "LOW"
	}

	// Token estimate: ~100 tokens per file read by AI
	// Without CRG: AI reads ~all files in package → estimate 50 files × 100 = 5000 tokens
	// With CRG: AI reads only impacted files → len(impacted) × 100
	ctx.TokenEstimate = max(0, 5000-(ctx.TotalImpacted+len(changedFiles))*100)

	// Build summary markdown for LLM prompt injection
	ctx.Summary = buildReviewSummary(ctx)

	return ctx, nil
}

// buildReviewSummary generates concise markdown that can be injected into LLM prompt.
func buildReviewSummary(ctx *ReviewContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Review Context (Risk: %s)\n\n", ctx.RiskLevel))

	// Changed files
	sb.WriteString(fmt.Sprintf("### Changed Files (%d)\n", len(ctx.ChangedFiles)))
	for _, f := range ctx.ChangedFiles {
		sb.WriteString(fmt.Sprintf("- `%s`\n", f))
	}

	// Impacted files by degree
	if ctx.TotalImpacted > 0 {
		sb.WriteString(fmt.Sprintf("\n### Impacted Files (%d)\n", ctx.TotalImpacted))
		byDeg := map[int][]ImpactedFile{}
		for _, imp := range ctx.ImpactedFiles {
			byDeg[imp.Degree] = append(byDeg[imp.Degree], imp)
		}
		degLabels := map[int]string{1: "Direct", 2: "2-hop", 3: "3-hop"}
		for deg := 1; deg <= 8; deg++ {
			files := byDeg[deg]
			if len(files) == 0 {
				continue
			}
			label := degLabels[deg]
			if label == "" {
				label = fmt.Sprintf("%d-hop", deg)
			}
			sb.WriteString(fmt.Sprintf("\n**%s (%d):**\n", label, len(files)))
			for _, f := range files {
				testIcon := "✅"
				if !f.HasTests {
					testIcon = "❌"
				}
				sb.WriteString(fmt.Sprintf("- `%s` (health:%.0f, test:%s, %d lines)\n",
					f.Path, f.HealthScore, testIcon, f.LineCount))
			}
		}
	}

	// Warnings
	if len(ctx.MissingTests) > 0 {
		sb.WriteString(fmt.Sprintf("\n### ⚠️ Missing Tests (%d files)\n", len(ctx.MissingTests)))
		for _, f := range ctx.MissingTests {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	}
	if len(ctx.CircularDeps) > 0 {
		sb.WriteString(fmt.Sprintf("\n### 🔄 Circular Dependencies (%d)\n", len(ctx.CircularDeps)))
		for _, f := range ctx.CircularDeps {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	}

	return sb.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

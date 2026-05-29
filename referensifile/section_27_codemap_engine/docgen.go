package codeindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateDocs generate file markdown dokumentasi ke docs/auto/<path>.md
// untuk setiap node di codemap_nodes yang punya doc_comment atau exported_symbols.
// Dipanggil async setelah IndexAll selesai.
//
// FIX #61 effekdomino.md: tambah orphan cleanup pre-pass — scan filesystem
// docs/auto/*.md, set diff vs codemap_nodes paths, DELETE orphan .md yang
// source-nya udah hilang. Per [AR.1] precedent 12 orphan ditemukan post-cleanup.
func GenerateDocs(db *sql.DB, workspaceRoot string) {
	start := time.Now()
	outDir := filepath.Join(workspaceRoot, "docs", "auto")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("[docgen] mkdir failed: %v", err)
		return
	}

	// FIX #61: build set of expected .md filenames dari codemap_nodes paths.
	// SAFETY: kalau query fail ATAU return 0 row, SKIP orphan cleanup —
	// jangan asumsi 'expected = empty' lalu DELETE semua .md. Sebelumnya
	// bug catastrophic: query error (DB lock / schema not init) = nuke 1480 file.
	expected := make(map[string]bool)
	cleanupSafe := false
	pathRows, qerr := db.Query(`SELECT path FROM codemap_nodes`)
	if qerr == nil {
		for pathRows.Next() {
			var p string
			if pathRows.Scan(&p) == nil {
				expected[strings.ReplaceAll(p, "/", "__")+".md"] = true
			}
		}
		pathRows.Close()
		// Hanya safe cleanup kalau ada minimum row yang reasonable (>10).
		// Edge case: codemap_nodes baru di-init kosong = false-positive nuke.
		if len(expected) >= 10 {
			cleanupSafe = true
		} else {
			log.Printf("[docgen] orphan cleanup SKIP — codemap_nodes only %d row (suspect uninitialized)", len(expected))
		}
	} else {
		log.Printf("[docgen] orphan cleanup SKIP — query codemap_nodes failed: %v", qerr)
	}

	// Scan filesystem docs/auto/*.md, hapus yang ngga di expected set.
	orphanCount := 0
	if cleanupSafe {
		if entries, err := os.ReadDir(outDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				// INDEX.md di-preserve (bukan orphan, generated oleh writeDocIndex).
				if e.Name() == "INDEX.md" || e.Name() == "index.md" {
					continue
				}
				if !expected[e.Name()] {
					orphanPath := filepath.Join(outDir, e.Name())
					if err := os.Remove(orphanPath); err != nil {
						log.Printf("[docgen] orphan rm failed %s: %v", e.Name(), err)
					} else {
						orphanCount++
					}
				}
			}
		}
		if orphanCount > 0 {
			log.Printf("[docgen] orphan cleanup: %d stale .md removed", orphanCount)
		}
	}

	rows, err := db.Query(`
		SELECT path, name, pkg, file_type, line_count, size_bytes,
		       exported_symbols, doc_comment, health_score, has_tests, has_docs, issues
		FROM codemap_nodes ORDER BY path`)
	if err != nil {
		log.Printf("[docgen] query failed: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			path, name, pkg, ft string
			lines, sizeB        int
			symJSON, docCmt     string
			health              float64
			hasTests, hasDocs   int
			issJSON             string
		)
		if err := rows.Scan(&path, &name, &pkg, &ft, &lines, &sizeB,
			&symJSON, &docCmt, &health, &hasTests, &hasDocs, &issJSON); err != nil {
			continue
		}

		var symbols []string
		json.Unmarshal([]byte(symJSON), &symbols)
		var issues []string
		json.Unmarshal([]byte(issJSON), &issues)

		// Skip file tanpa info berarti
		if docCmt == "" && len(symbols) == 0 {
			continue
		}

		md := buildMarkdown(path, name, pkg, ft, lines, sizeB, docCmt, symbols, health, hasTests, hasDocs, issues)

		// Tulis ke docs/auto/<path>.md
		outPath := filepath.Join(outDir, strings.ReplaceAll(path, "/", "__")+".md")
		if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
			log.Printf("[docgen] write %s: %v", outPath, err)
			continue
		}
		count++
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	if err := rows.Err(); err != nil {
		log.Printf("[docgen] rows.Err: %v", err)
	}

	// Tulis index.md — daftar semua file
	writeDocIndex(db, outDir)

	log.Printf("[docgen] generated %d docs in %s", count, time.Since(start).Round(time.Millisecond))
}

func buildMarkdown(path, name, pkg, ft string, lines, sizeB int, docCmt string, symbols []string, health float64, hasTests, hasDocs int, issues []string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# `%s`\n\n", name))
	b.WriteString(fmt.Sprintf("**Path:** `%s`  \n", path))
	b.WriteString(fmt.Sprintf("**Type:** `%s`", strings.ToUpper(ft)))
	if pkg != "" {
		b.WriteString(fmt.Sprintf("  **Package:** `%s`", pkg))
	}
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("**Lines:** %d  **Size:** %s  **Health:** %.0f/100\n\n",
		lines, humanSize(int64(sizeB)), health))

	// Badges
	if hasTests == 1 {
		b.WriteString("![test](https://img.shields.io/badge/test-✅-green) ")
	} else {
		b.WriteString("![test](https://img.shields.io/badge/test-❌-red) ")
	}
	if hasDocs == 1 {
		b.WriteString("![docs](https://img.shields.io/badge/docs-✅-green)\n\n")
	} else {
		b.WriteString("![docs](https://img.shields.io/badge/docs-❌-red)\n\n")
	}

	// Deskripsi
	if docCmt != "" {
		b.WriteString("## Deskripsi\n\n")
		b.WriteString(docCmt)
		b.WriteString("\n\n")
	}

	// Exported symbols
	if len(symbols) > 0 {
		b.WriteString("## Exported Symbols\n\n")
		for _, s := range symbols {
			b.WriteString(fmt.Sprintf("- `%s`\n", s))
		}
		b.WriteString("\n")
	}

	// Issues
	if len(issues) > 0 {
		b.WriteString("## ⚠️ Issues\n\n")
		for _, iss := range issues {
			b.WriteString(fmt.Sprintf("- %s\n", iss))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("*Auto-generated by FloworkOS codeindex — jangan edit manual.*\n"))
	return b.String()
}

func writeDocIndex(db *sql.DB, outDir string) {
	rows, err := db.Query(`
		SELECT path, name, file_type, health_score, line_count
		FROM codemap_nodes ORDER BY health_score ASC`)
	if err != nil {
		return
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("# Auto-Documentation Index\n\n")
	b.WriteString("*Sorted by health score (terendah = butuh perhatian paling dulu)*\n\n")
	b.WriteString("| File | Type | Lines | Health |\n")
	b.WriteString("|------|------|-------|--------|\n")

	for rows.Next() {
		var path, name, ft string
		var health float64
		var lines int
		rows.Scan(&path, &name, &ft, &health, &lines)
		bar := healthBar(health)
		b.WriteString(fmt.Sprintf("| `%s` | %s | %d | %s %.0f |\n", path, ft, lines, bar, health))
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()

	os.WriteFile(filepath.Join(outDir, "INDEX.md"), []byte(b.String()), 0644)
}

func healthBar(score float64) string {
	filled := int(score / 10)
	empty := 10 - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func humanSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
}

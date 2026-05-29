//go:build ignore

// panic_in_goroutine_scanner — deteksi goroutine tanpa recover wrapper.
//
// Pattern detect: `go func() { ... }()` tanpa `defer recover()` di body.
// Goroutine yang panic tanpa recover akan CRASH SELURUH PROCESS —
// bukan cuma goroutine itu, tapi semua. Ini beda sama Java/Python thread.
// False positive guard: skip jika body punya defer+recover, atau fungsi pendek.
// Severity: CRITICAL — single unrecovered panic = seluruh Flowork mati.
// Policy: per GOL_FLOWORK §FASE 3 (Night-Watch — process survival guarantee).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Issue struct {
	Level   string
	File    string
	Line    int
	Message string
}

func main() {
	start := time.Now()
	fmt.Println("💥 [\033[1;31mPANIC IN GOROUTINE SCANNER\033[0m] Cari goroutine tanpa recover protection...")

	fset := token.NewFileSet()
	var issues []Issue

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			b := filepath.Base(path)
			if b == ".git" || b == "vendor" || b == "scanner" || b == "_sgvp" || b == "node_modules" || b == "tools_temp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				goStmt, ok := n.(*ast.GoStmt)
				if !ok {
					return true
				}

				funcLit, ok := goStmt.Call.Fun.(*ast.FuncLit)
				if !ok {
					return true // named function call — harder to check, skip
				}

				if funcLit.Body == nil || len(funcLit.Body.List) == 0 {
					return true
				}

				// Check if body has defer+recover
				hasRecover := false
				ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
					deferStmt, ok := inner.(*ast.DeferStmt)
					if !ok {
						return true
					}
					// Check if defer calls recover() directly or wraps it
					ast.Inspect(deferStmt.Call, func(dNode ast.Node) bool {
						if call, ok := dNode.(*ast.CallExpr); ok {
							if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "recover" {
								hasRecover = true
								return false
							}
						}
						return true
					})
					return !hasRecover
				})

				if !hasRecover {
					// Skip very short goroutines (1-2 statements) — usually simple channel sends
					if len(funcLit.Body.List) <= 2 {
						return true
					}

					pos := fset.Position(goStmt.Pos())
					issues = append(issues, Issue{
						Level: "HIGH",
						File:  pos.Filename,
						Line:  pos.Line,
						Message: fmt.Sprintf(
							"Goroutine closure di `%s()` tanpa `defer func() { recover() }()` — jika goroutine ini panic, SELURUH PROCESS CRASH (bukan cuma goroutine ini). Tambah recover wrapper.",
							fn.Name.Name),
					})
				}
				return true
			})
		}
		return nil
	})

	for _, i := range issues {
		fmt.Printf("[%s] %s:%d — %s\n", i.Level, i.File, i.Line, i.Message)
	}
	fmt.Printf("\n✅ PANIC IN GOROUTINE scanner done in %s. %d findings.\n",
		time.Since(start).Truncate(time.Millisecond), len(issues))

	outFile := filepath.Join(".", "docs", "bug", "ext_panic_goroutine_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeReport(outFile, issues, "💥 Panic in Goroutine", "flowork_panic_goroutine_auditor.go", "go func(){} tanpa recover — panic = process crash")
	fmt.Println("📜 Report:", outFile)

	if len(issues) > 0 {
		os.Exit(1)
	}
}

func writeReport(outFile string, issues []Issue, title, scanner, target string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()
	out.WriteString(fmt.Sprintf("# %s Scanner Report\n\n", title))
	out.WriteString(fmt.Sprintf("> **Scanner:** %s\n", scanner))
	out.WriteString(fmt.Sprintf("> **Target:** %s\n\n", target))
	if len(issues) == 0 {
		out.WriteString("✅ *Tidak ditemukan issue di codebase.*\n")
		return
	}
	crit, high, med, low := 0, 0, 0, 0
	for _, f := range issues {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		case "LOW":
			low++
		}
	}
	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 %d | 🟠 %d | 🟡 %d | 🔵 %d)\n\n", len(issues), crit, high, med, low))
	for i, f := range issues {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s]\n", i+1, f.Level))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

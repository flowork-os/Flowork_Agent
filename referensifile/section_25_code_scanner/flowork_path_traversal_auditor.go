//go:build ignore

// path_traversal_scanner — deteksi user input yang masuk ke filepath tanpa sanitize.
//
// Pattern detect: filepath.Join/os.Open/os.ReadFile/os.Create di mana argument
// berasal dari parameter fungsi HTTP handler, tanpa filepath.Clean + containment check.
// Attacker kirim "../../etc/passwd" → baca file arbitrary dari server.
// False positive guard: skip jika ada filepath.Clean + strings.HasPrefix check.
// Severity: CRITICAL — path traversal = arbitrary file read/write.
// Policy: per GOL_FLOWORK §FASE 7 (Sistem Imun — injection defense).
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

// Functions that operate on file paths (dangerous with user input)
var fileOps = map[string]bool{
	"Open": true, "Create": true, "OpenFile": true,
	"ReadFile": true, "WriteFile": true, "Remove": true,
	"Stat": true, "Lstat": true, "MkdirAll": true, "Mkdir": true,
}

func main() {
	start := time.Now()
	fmt.Println("🛤️ [\033[1;31mPATH TRAVERSAL SCANNER\033[0m] Cari path traversal vulnerability...")

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

			// Collect func params
			paramNames := map[string]bool{}
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					for _, name := range field.Names {
						paramNames[name.Name] = true
					}
				}
			}
			if len(paramNames) == 0 {
				continue
			}

			// Check if function has filepath.Clean or containment check
			hasClean := false
			hasContain := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if pkg, ok := sel.X.(*ast.Ident); ok {
							if pkg.Name == "filepath" && sel.Sel.Name == "Clean" {
								hasClean = true
							}
							if pkg.Name == "strings" && sel.Sel.Name == "HasPrefix" {
								hasContain = true
							}
							if pkg.Name == "strings" && sel.Sel.Name == "Contains" {
								// Check for ".." containment check
								if len(call.Args) >= 2 {
									if lit, ok := call.Args[1].(*ast.BasicLit); ok {
										if strings.Contains(lit.Value, "..") {
											hasContain = true
										}
									}
								}
							}
						}
					}
				}
				return true
			})

			if hasClean && hasContain {
				continue // properly sanitized
			}

			// Look for filepath.Join where a param is used
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}

				// filepath.Join with user param
				if pkg.Name == "filepath" && sel.Sel.Name == "Join" {
					for _, arg := range call.Args {
						if ident, ok := arg.(*ast.Ident); ok && paramNames[ident.Name] {
							lower := strings.ToLower(ident.Name)
							if strings.Contains(lower, "path") || strings.Contains(lower, "file") ||
								strings.Contains(lower, "name") || strings.Contains(lower, "dir") ||
								strings.Contains(lower, "key") || strings.Contains(lower, "id") {
								pos := fset.Position(call.Pos())
								issues = append(issues, Issue{
									Level: "HIGH",
									File:  pos.Filename,
									Line:  pos.Line,
									Message: fmt.Sprintf(
										"Path traversal risk: `filepath.Join(..., %s)` — parameter `%s` dari caller bisa berisi `../../`. Tambah `filepath.Clean()` + containment check.",
										ident.Name, ident.Name),
								})
							}
						}
					}
				}

				// os.Open/ReadFile etc with user param directly
				if pkg.Name == "os" && fileOps[sel.Sel.Name] {
					if len(call.Args) >= 1 {
						if ident, ok := call.Args[0].(*ast.Ident); ok && paramNames[ident.Name] {
							lower := strings.ToLower(ident.Name)
							if strings.Contains(lower, "path") || strings.Contains(lower, "file") ||
								strings.Contains(lower, "name") {
								pos := fset.Position(call.Pos())
								issues = append(issues, Issue{
									Level: "CRITICAL",
									File:  pos.Filename,
									Line:  pos.Line,
									Message: fmt.Sprintf(
										"Path traversal: `os.%s(%s)` — parameter langsung masuk filesystem op tanpa sanitasi. Attacker kirim `../../etc/passwd`.",
										sel.Sel.Name, ident.Name),
								})
							}
						}
					}
				}

				return true
			})
		}
		return nil
	})

	for _, i := range issues {
		fmt.Printf("[%s] %s:%d — %s\n", i.Level, i.File, i.Line, i.Message)
	}
	fmt.Printf("\n✅ PATH TRAVERSAL scanner done in %s. %d findings.\n",
		time.Since(start).Truncate(time.Millisecond), len(issues))

	outFile := filepath.Join(".", "docs", "bug", "ext_path_traversal_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeReport(outFile, issues, "🛤️ Path Traversal", "flowork_path_traversal_auditor.go", "User input di filepath tanpa sanitasi → arbitrary file access")
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

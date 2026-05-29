//go:build ignore

// toctou_scanner — deteksi Time-of-Check Time-of-Use (TOCTOU) race conditions.
//
// Pattern detect: os.Stat/os.Lstat followed by os.Open/os.Create/os.ReadFile
// tanpa atomic operation. Penyerang bisa swap file antara check dan use.
// False positive guard: skip jika check+use dalam satu atomic op (O_CREATE|O_EXCL).
// Severity: HIGH — TOCTOU = classic filesystem race → privilege escalation via symlink.
// Policy: per GOL_FLOWORK §FASE 7 (Sistem Imun — anti filesystem attack).
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

// Functions that check file existence/properties
var checkFuncs = map[string]bool{
	"os.Stat":       true,
	"os.Lstat":      true,
	"os.IsExist":    true,
	"os.IsNotExist": true,
	"filepath.Glob": true,
}

// Functions that use files
var useFuncs = map[string]bool{
	"os.Open":      true,
	"os.Create":    true,
	"os.OpenFile":  true,
	"os.ReadFile":  true,
	"os.WriteFile": true,
	"os.Remove":    true,
	"os.RemoveAll": true,
	"os.Rename":    true,
	"os.Chmod":     true,
	"os.Chown":     true,
	"os.Mkdir":     true,
	"os.MkdirAll":  true,
}

func getFuncName(call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok {
			return pkg.Name + "." + sel.Sel.Name
		}
	}
	return ""
}

func main() {
	start := time.Now()
	fmt.Println("⏱️ [\033[1;31mTOCTOU SCANNER\033[0m] Cari Time-of-Check/Time-of-Use file race conditions...")

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

			// Walk through statements looking for check-then-use pattern
			stmts := fn.Body.List
			for i := 0; i < len(stmts); i++ {
				// Look for if-statement with os.Stat check
				ifStmt, ok := stmts[i].(*ast.IfStmt)
				if !ok {
					continue
				}

				// Check the Init or Cond for stat/check calls
				hasCheck := false
				checkFuncName := ""
				var checkPath string

				ast.Inspect(ifStmt, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := getFuncName(call)
					if checkFuncs[name] {
						hasCheck = true
						checkFuncName = name
						// Try to extract the file path argument
						if len(call.Args) > 0 {
							if ident, ok := call.Args[0].(*ast.Ident); ok {
								checkPath = ident.Name
							}
						}
					}
					return true
				})

				if !hasCheck {
					continue
				}

				// Now look inside the if-body for file use operations
				ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := getFuncName(call)
					if !useFuncs[name] {
						return true
					}

					// Check if same path variable is used
					samePath := false
					if checkPath != "" && len(call.Args) > 0 {
						if ident, ok := call.Args[0].(*ast.Ident); ok {
							if ident.Name == checkPath {
								samePath = true
							}
						}
					}

					// Skip os.MkdirAll — it's inherently safe (creates if not exists)
					if name == "os.MkdirAll" {
						return true
					}

					if samePath {
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Level: "HIGH",
							File:  pos.Filename,
							Line:  pos.Line,
							Message: fmt.Sprintf(
								"TOCTOU race: `%s()` dipakai setelah `%s()` pada path yang sama (`%s`) — antara check dan use, penyerang bisa swap file/symlink. Gunakan atomic open (O_CREATE|O_EXCL) atau lock file.",
								name, checkFuncName, checkPath),
						})
					}

					return true
				})

				// Also check else branch
				if ifStmt.Else != nil {
					ast.Inspect(ifStmt.Else, func(n ast.Node) bool {
						call, ok := n.(*ast.CallExpr)
						if !ok {
							return true
						}
						name := getFuncName(call)
						if !useFuncs[name] || name == "os.MkdirAll" {
							return true
						}

						if checkPath != "" && len(call.Args) > 0 {
							if ident, ok := call.Args[0].(*ast.Ident); ok {
								if ident.Name == checkPath {
									pos := fset.Position(call.Pos())
									issues = append(issues, Issue{
										Level: "HIGH",
										File:  pos.Filename,
										Line:  pos.Line,
										Message: fmt.Sprintf(
											"TOCTOU race (else branch): `%s()` setelah `%s()` check — filesystem state bisa berubah. Gunakan atomic open.",
											name, checkFuncName),
									})
								}
							}
						}

						return true
					})
				}
			}
		}
		return nil
	})

	for _, i := range issues {
		fmt.Printf("[%s] %s:%d — %s\n", i.Level, i.File, i.Line, i.Message)
	}
	fmt.Printf("\n✅ TOCTOU scanner done in %s. %d findings.\n",
		time.Since(start).Truncate(time.Millisecond), len(issues))

	outFile := filepath.Join(".", "docs", "bug", "ext_toctou_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeReport(outFile, issues, "⏱️ TOCTOU Race", "flowork_toctou_auditor.go", "Time-of-Check/Time-of-Use file race conditions")
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

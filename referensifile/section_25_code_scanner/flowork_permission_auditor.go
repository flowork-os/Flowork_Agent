//go:build ignore

// ext_permission_scanner — mendeteksi file permission terlalu permissive.
//
// Checks: 0777, 0766, 0666 pada MkdirAll/WriteFile/OpenFile
//
//	File sensitif tanpa 0600/0700
//
// Prinsip: GOL B (Protected Core), FQP-13 (No-Broadcasting)
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PermFinding struct {
	Level, Type, File, Func, Message string
	Line                             int
}

var findings []PermFinding

func main() {
	fmt.Println("🔓 [EXT_PERMISSION v1] Scanning for overly permissive file permissions...")
	fmt.Println("   Prinsip: GOL B (Protected Core), FQP-13 (No-Broadcasting)")
	fmt.Println()
	root := "."
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			b := filepath.Base(path)
			if b == ".git" || b == "vendor" || b == "scanner" || b == "_sgvp" || b == "docs" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scan(path)
		}
		return nil
	})
	fmt.Printf("\n[🔓] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}
	// rc140 fix: writeReport lives in flowork_auditor.go (shared) but each
	// scanner `//go:build ignore` = standalone package main, so cross-file
	// references don't compile. Inlined minimal markdown emitter here so
	// the scanner runs standalone (required by _sgvp/run_sgvp.sh + audit_all.sh).
	out := filepath.Join(root, "state", "scanner-reports", "ext_permission_report.md")
	os.MkdirAll(filepath.Dir(out), 0755)
	writeMarkdownReport(out, "PERMISSION SCANNER", findings)
	fmt.Println("\n📜 Report:", out)
}

// writeMarkdownReport emits a minimal per-scanner .md so CI/Dashboard can
// pick up findings without needing the (non-standalone) writeReport helper
// from flowork_auditor.go.
func writeMarkdownReport(path, title string, items []PermFinding) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "# %s — Report\n\n", title)
	fmt.Fprintf(f, "Findings: %d\n\n", len(items))
	for _, it := range items {
		fmt.Fprintf(f, "- **[%s] %s** — `%s:%d` (func %s) — %s\n",
			it.Level, it.Type, it.File, it.Line, it.Func, it.Message)
	}
}

func scan(filePath string) {
	fset := token.NewFileSet()
	node, _ := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if node == nil {
		return
	}
	for _, d := range node.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check os.MkdirAll, os.WriteFile, os.OpenFile for permission arg
			permIdx := -1
			if isId(sel.X, "os") {
				switch sel.Sel.Name {
				case "MkdirAll":
					permIdx = 1
				case "WriteFile":
					permIdx = 2
				case "OpenFile":
					permIdx = 2
				}
			}

			if permIdx >= 0 && permIdx < len(call.Args) {
				lit, ok := call.Args[permIdx].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					return true
				}
				perm, err := strconv.ParseInt(lit.Value, 0, 64)
				if err != nil {
					return true
				}

				// Flag world-writable (xx7, xx6)
				worldBits := perm & 0o007
				if worldBits >= 6 {
					findings = append(findings, PermFinding{
						Level: "HIGH", Type: "World-Writable Permission",
						File: filePath, Line: fset.Position(lit.Pos()).Line, Func: name,
						Message: fmt.Sprintf(
							"os.%s() dengan permission %s — world-writable. "+
								"Siapa saja di sistem bisa modify file ini. "+
								"Gunakan 0600 untuk file sensitif, 0755 untuk directory.",
							sel.Sel.Name, lit.Value),
					})
				} else if worldBits >= 4 {
					// World-readable but not writable — medium for sensitive files
					funcLow := strings.ToLower(name)
					if strings.Contains(funcLow, "token") || strings.Contains(funcLow, "key") ||
						strings.Contains(funcLow, "secret") || strings.Contains(funcLow, "auth") ||
						strings.Contains(funcLow, "password") || strings.Contains(funcLow, "credential") {
						findings = append(findings, PermFinding{
							Level: "MEDIUM", Type: "Sensitive File World-Readable",
							File: filePath, Line: fset.Position(lit.Pos()).Line, Func: name,
							Message: fmt.Sprintf(
								"os.%s() dengan permission %s di fungsi terkait credential (%s). "+
									"File sensitif harus 0600 (owner-only). Permission saat ini "+
									"memungkinkan user lain membaca file.",
								sel.Sel.Name, lit.Value, name),
						})
					}
				}
			}
			return true
		})
	}
}

func isId(e ast.Expr, n string) bool { id, ok := e.(*ast.Ident); return ok && id.Name == n }

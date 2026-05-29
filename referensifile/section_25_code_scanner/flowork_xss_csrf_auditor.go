//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SGVP XSS CSRF Auditor
// Mendeteksi kerawanan import `text/template` untuk endpoint html atau Write terhadap w HTTP dari URL path tanpa diekstrak ke escaped template.
func main() {
	cwd, _ := os.Getwd()
	repoRoot := getRepoRoot(cwd)
	fset := token.NewFileSet()

	findings := 0
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if strings.Contains(path, "vendor") || strings.Contains(path, "scanner") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		
		for _, imp := range node.Imports {
			if imp.Path.Value == `"text/template"` {
				rel, _ := filepath.Rel(repoRoot, path)
				fmt.Printf("[HIGH] XSS Hazard: Dilarang mengimpor `+"`text/template`"+` karena tidak dilengkapi Auto-Escaping (gunakan `+"`html/template`"+`) di %s\n", rel)
				findings++
			}
		}

		ast.Inspect(node, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					// Detect raw HTML Write directly injected with query string wrapper.
					if sel.Sel.Name == "Write" || sel.Sel.Name == "WriteString" {
						if len(call.Args) > 0 {
							arg0 := call.Args[0]
							switch callInner := arg0.(type) {
							case *ast.CallExpr:
								if selIn, ok := callInner.Fun.(*ast.SelectorExpr); ok && selIn.Sel.Name == "Sprintf" {
									// Potentially a fmt.Sprintf writing directly to http.ResponseWriter
									// Simplified heuristic check here
									rel, _ := filepath.Rel(repoRoot, path)
									fmt.Printf("[MEDIUM] XSS / Injection Hazard: Menembak penggabungan string secara mentah ke aliran data io.Writer HTTP (Potensi XSS) di %s pada %s\n", rel, fset.Position(n.Pos()).String())
									findings++
								}
							}
						}
					}
				}
			}
			return true
		})
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking repo: %v\n", err)
		os.Exit(1)
	}

	if findings > 0 {
		fmt.Printf("🚨 Ditemukan %d XSS hazard(s).\n", findings)
		// os.Exit(1)
	}
}

func getRepoRoot(cwd string) string {
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return cwd
}

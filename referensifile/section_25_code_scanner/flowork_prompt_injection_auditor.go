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

// SGVP Prompt Injection Auditor
// Mendeteksi penggabungan string yang tidak aman ke dalam payload Prompt yang dapat dilanggar oleh XML Injection.
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

		ast.Inspect(node, func(n ast.Node) bool {
			// Find aiforge.Request definitions or Request structs that set Prompt
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if ident, ok := kv.Key.(*ast.Ident); ok {
					if ident.Name == "SystemPrompt" || ident.Name == "UserPrompt" {
						if isUnsafePromptString(kv.Value) {
							fmt.Printf("[CRITICAL] Prompt Injection Hazard: Tidak terproteksi dari input tak aman pada %s di %s\n", ident.Name, fset.Position(n.Pos()).String())
							findings++
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
		fmt.Printf("🚨 Ditemukan %d Prompt Injection hazard(s).\n", findings)
		os.Exit(1)
	}
}

func isUnsafePromptString(node ast.Expr) bool {
	// Jika gabungan string atau fmt.Sprintf tanpa sanitasi
	switch expr := node.(type) {
	case *ast.BinaryExpr:
		if expr.Op == token.ADD {
			return true
		}
	case *ast.CallExpr:
		if sel, ok := expr.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" {
			return true
		}
	}
	return false
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

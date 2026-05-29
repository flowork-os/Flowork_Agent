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

// SGVP Token Leak Auditor
// Mendeteksi kemungkinan memori LLM kepenuhan karena membaca output file ke prompt tanpa truncateizer.
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
			// Find os.ReadFile being used directly in a Prompt struct definition argument natively without wrapping in limits
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ReadFile" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "os" {
						rel, _ := filepath.Rel(repoRoot, path)
						// This is a heuristic finding.
						fmt.Printf("[LOW] Token Leak Hazard: os.ReadFile harus memperhatikan panjang prompt LLM ketika dilempar secara brutal di %s pada %s\n", rel, fset.Position(n.Pos()).String())
						findings++
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
		fmt.Printf("🚨 Ditemukan %d Token Leak hazard(s).\n", findings)
		// os.Exit(1) // Not fatal threshold
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

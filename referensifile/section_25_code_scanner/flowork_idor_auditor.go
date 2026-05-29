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

// SGVP IDOR Auditor
// Mendeteksi pengambilan parameter ID dari URL Query yang tidak diiringi dengan validasi otorisasi.
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
			// Find function declarations or literals handling http requests
			if fn, ok := n.(*ast.FuncDecl); ok {
				if hasIDORHazard(fn.Body) {
					fmt.Printf("[HIGH] IDOR Hazard: Pengambilan parameter 'id' tanpa validasi yang mencukupi pada %s di %s\n", fn.Name.Name, fset.Position(n.Pos()).String())
					findings++
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
		fmt.Printf("🚨 Ditemukan %d IDOR hazard(s).\n", findings)
		os.Exit(1)
	}
}

func hasIDORHazard(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	getsID := false
	validates := false

	ast.Inspect(block, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Get" {
					if len(call.Args) == 1 {
						if lit, ok := call.Args[0].(*ast.BasicLit); ok {
							raw := strings.Trim(lit.Value, "\"")
							if raw == "id" || raw == "user_id" {
								getsID = true
							}
						}
					}
				}
				// Look for some typical permission checks like ownerauth.Check or strings.Contains(..., "auth")
				name := sel.Sel.Name
				if strings.Contains(strings.ToLower(name), "auth") || strings.Contains(strings.ToLower(name), "owner") || strings.Contains(strings.ToLower(name), "valid") {
					validates = true
				}
			}
		}
		return true
	})

	return getsID && !validates
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

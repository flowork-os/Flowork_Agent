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

// SGVP API Rate Limit Auditor
// Mendeteksi http call di dalam For/Range tanpa timer rate-limit (time.Sleep).
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
			// Find for/range loop
			var body *ast.BlockStmt
			switch loop := n.(type) {
			case *ast.ForStmt:
				body = loop.Body
			case *ast.RangeStmt:
				body = loop.Body
			}

			if body != nil {
				hasSleep := false
				hasHTTP := false
				var httpPos token.Pos

				ast.Inspect(body, func(nn ast.Node) bool {
					if call, ok := nn.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if ident, ok := sel.X.(*ast.Ident); ok {
								if ident.Name == "http" || ident.Name == "client" {
									if sel.Sel.Name == "Get" || sel.Sel.Name == "Post" || sel.Sel.Name == "Do" {
										hasHTTP = true
										httpPos = call.Pos()
									}
								}
								if ident.Name == "time" && sel.Sel.Name == "Sleep" {
									hasSleep = true
								}
							}
						}
					}
					return true
				})

				if hasHTTP && !hasSleep {
					rel, _ := filepath.Rel(repoRoot, path)
					fmt.Printf("[HIGH] API Rate Limit Hazard: HTTP request dieksekusi di dalam perulangan tanpa jeda sleep/rate-limit di %s pada %s\n", rel, fset.Position(httpPos).String())
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
		fmt.Printf("🚨 Ditemukan %d API Rate Limit hazard(s).\n", findings)
		os.Exit(1)
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

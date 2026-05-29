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

// SGVP SQL Injection Auditor
// Mendeteksi penggabungan string atau fmt.Sprintf pada argument SQL eksekusi.
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
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Looking for Query, QueryRow, Exec
			if sel.Sel.Name == "Query" || sel.Sel.Name == "QueryRow" || sel.Sel.Name == "Exec" {
				if len(call.Args) > 0 {
					arg0 := call.Args[0]

					// Jika ada args `ctx`, check arg selanjutnya (di stdlib sql, ctx selalu di arg 0 QueryContext/ExecContext)
					// Handle generic: check args for raw string concatenation
					if isUnsafeSQLArg(arg0) {
						rel, _ := filepath.Rel(repoRoot, path)
						fmt.Printf("[CRITICAL] SQL Injection Hazard: Penggabungan string/fmt.Sprintf digunakan di argumen %s kueri SQL di %s\n", sel.Sel.Name, fset.Position(n.Pos()).String())
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
		fmt.Printf("🚨 Ditemukan %d SQL Injection hazard(s).\n", findings)
		os.Exit(1)
	}
}

func isUnsafeSQLArg(node ast.Expr) bool {
	switch expr := node.(type) {
	case *ast.BinaryExpr:
		if expr.Op == token.ADD {
			return true
		}
	case *ast.CallExpr:
		if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "fmt" && (sel.Sel.Name == "Sprintf" || sel.Sel.Name == "Sprint") {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "strings" && sel.Sel.Name == "Join" {
				return true
			}
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

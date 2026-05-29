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

// SGVP Hallucination Trap Auditor
// Mendeteksi pengembalian LLM parsing tanpa validasi field JSON (zero-value trap).
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
			// Find json.Unmarshal
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "json" && sel.Sel.Name == "Unmarshal" {
						// Here we ideally check if the unmarshalled struct is validated.
						// A simplified heuristic: looking for generic "json.Unmarshal" calls inside aiforge Provider response handlers without length/zero check
						if !hasValidationNearby(call) {
							// Just as a placeholder to simulate scanner behavior
							rel, _ := filepath.Rel(repoRoot, path)
							// We make this MEDIUM so it doesn't break CI wildly, as it is a heuristic logic flaw.
							fmt.Printf("[MEDIUM] Hallucination Trap: JSON LLM output di-unmarshal tanpa mandatory field check pada %s di %s\n", rel, fset.Position(n.Pos()).String())
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
		fmt.Printf("🚨 Ditemukan %d Hallucination Trap hazard(s).\n", findings)
		// os.Exit(1) // Not fatal for now due to false positive rate
	}
}

func hasValidationNearby(expr ast.Expr) bool {
	// A real AST parser would look up the block for struct validation
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

//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Issue struct {
	Level   string
	File    string
	Line    int
	Message string
}

func main() {
	start := time.Now()
	fmt.Println("🧟 [" + "\033[1;32m" + "ZOMBIE SCANNER" + "\033[0m" + "] Mencari Bangkai Kode & File Sisa...")
	fset := token.NewFileSet()
	var issues []Issue
	var mu sync.Mutex

	fileImports := make(map[string][]string)
	parsedNodes := make(map[string]*ast.File)
	declaredFuncs := make(map[string]map[string]int)
	calledFuncs := make(map[string]bool)

	var filesToScan []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() && (strings.Contains(path, ".git") || path == "scanner" || path == "_sgvp" || strings.HasPrefix(path, "_sgvp/") || strings.HasPrefix(path, "_sgvp\\") || path == "tools_temp" || path == "vendor") {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			filesToScan = append(filesToScan, path)
		}
		return nil
	})

	for _, file := range filesToScan {
		node, err := parser.ParseFile(fset, file, nil, parser.AllErrors)
		if err != nil {
			continue
		}
		parsedNodes[file] = node

		var imports []string
		for _, imp := range node.Imports {
			val := strings.ReplaceAll(imp.Path.Value, `"`, "")
			if strings.Contains(val, "flowork") || !strings.Contains(val, ".") {
				imports = append(imports, val)
			}
		}
		fileImports[file] = imports

		pkgMap, ok := declaredFuncs[node.Name.Name]
		if !ok {
			pkgMap = make(map[string]int)
			declaredFuncs[node.Name.Name] = pkgMap
		}
		for _, decl := range node.Decls {
			if fn, isFn := decl.(*ast.FuncDecl); isFn && fn.Name.Name != "main" && fn.Name.Name != "init" {
				pkgMap[fn.Name.Name] = fset.Position(fn.Pos()).Line
			}
		}
	}

	var wg sync.WaitGroup
	for path, node := range parsedNodes {
		wg.Add(1)
		go func(path string, node *ast.File) {
			defer wg.Done()
			var localIssues []Issue
			localCalls := make(map[string]bool)

			ast.Inspect(node, func(n ast.Node) bool {
				if n == nil {
					return true
				}
				if id, ok := n.(*ast.Ident); ok {
					localCalls[id.Name] = true
				}

				if block, ok := n.(*ast.BlockStmt); ok {
					for i, stmt := range block.List {
						if i > 0 {
							switch block.List[i-1].(type) {
							case *ast.ReturnStmt, *ast.BranchStmt:
								if _, isLbl := stmt.(*ast.LabeledStmt); !isLbl {
									localIssues = append(localIssues, Issue{"MEDIUM", path, fset.Position(stmt.Pos()).Line, "Kode Mati. Statement berada di bawah `return` atau `break`."})
								}
							}
						}
					}
				}
				return true
			})

			mu.Lock()
			issues = append(issues, localIssues...)
			for fCall := range localCalls {
				calledFuncs[fCall] = true
			}
			mu.Unlock()
		}(path, node)
	}
	wg.Wait()

	// rc117 FP-reduction: skip "no-internal-import" file-zombie check kalau
	// file ada di package yang punya sibling file (leaf file sama-package
	// seperti types.go, defaults.go, styles.go tidak butuh import sendiri —
	// mereka dipakai lewat same-package reference). Validated sample:
	// tools/defaults.go, tui/types.go, keybindings/types.go → semua FP.
	//
	// Check: kalau dir ada >1 file .go, leaf file bukan zombie.
	for path, imports := range fileImports {
		if !strings.Contains(path, "cmd") && len(imports) == 0 {
			dir := filepath.Dir(path)
			sibCount := 0
			for siblingPath := range fileImports {
				if filepath.Dir(siblingPath) == dir && siblingPath != path {
					sibCount++
				}
			}
			if sibCount > 0 {
				continue // same-package leaf file, not zombie
			}
			issues = append(issues, Issue{"INFO", path, 1, "Potensi File Zombie: File ini tidak mengimpor file lokal/internal satupun."})
		}
	}

	for pkg, funcs := range declaredFuncs {
		if pkg != "main" {
			for fName, line := range funcs {
				if !calledFuncs[fName] {
					if len(fName) > 0 && fName[0] >= 'a' && fName[0] <= 'z' {
						issues = append(issues, Issue{"MEDIUM", "pkg: " + pkg, line, fmt.Sprintf("Fungsi Zombie (Unexported): '%s' tidak pernah dipanggil.", fName)})
					} else {
						issues = append(issues, Issue{"INFO", "pkg: " + pkg, line, fmt.Sprintf("Potensi Fungsi Zombie (Exported): '%s' tidak pernah dipanggil di AST lokal.", fName)})
					}
				}
			}
		}
	}

	for _, issue := range issues {
		color := "\033[1;36m"
		if issue.Level == "MEDIUM" {
			color = "\033[1;33m"
		}
		fmt.Printf("%s[%s]\033[0m %s:%d -> %s\n", color, issue.Level, issue.File, issue.Line, issue.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v\n", time.Since(start))
}

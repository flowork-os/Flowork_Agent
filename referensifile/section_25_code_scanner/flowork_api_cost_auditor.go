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
	fmt.Println("💸 [" + "\033[1;32m" + "ACCOUNTANT SCANNER" + "\033[0m" + "] Menganalisis potensi eksploitasi API Billing (Cost Leak)...")
	fset := token.NewFileSet()
	var issues []Issue
	var mu sync.Mutex

	var filesToScan []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || (info.IsDir() && (strings.Contains(path, ".git") || path == "scanner" || path == "_sgvp" || strings.HasPrefix(path, "_sgvp/") || strings.HasPrefix(path, "_sgvp\\") || path == "vendor" || path == "tools_temp")) {
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

	var wg sync.WaitGroup
	for _, file := range filesToScan {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
			if err != nil {
				return
			}

			var localIssues []Issue

			// Fungsi internal untuk memindai isi for statement
			var scanLoop func(body ast.Node)
			scanLoop = func(body ast.Node) {
				ast.Inspect(body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							funcName := sel.Sel.Name
							if id, ok := sel.X.(*ast.Ident); ok {
								pkgName := id.Name
								// Jika di dalam loop melakukan panggilan HTTP (Get/Post) atau call LLM API
								if (pkgName == "http" && (funcName == "Get" || funcName == "Post")) || strings.Contains(strings.ToLower(funcName), "completion") || strings.Contains(strings.ToLower(funcName), "generate") {
									localIssues = append(localIssues, Issue{"CRITICAL", path, fset.Position(n.Pos()).Line, fmt.Sprintf("API Cost Bomb: Memanggil API eksternal jarak jauh (`%s.%s`) di dalam struktur Loop! Resiko tagihan meledak jika loop macet.", pkgName, funcName)})
								}
							}
						}
					}
					// Nested loops tidak peru discan ulang karena ast.Inspect body sudah menelan kedalamannya.
					return true
				})
			}

			ast.Inspect(node, func(n ast.Node) bool {
				if forStmt, ok := n.(*ast.ForStmt); ok {
					scanLoop(forStmt.Body)
				}
				if rangeStmt, ok := n.(*ast.RangeStmt); ok {
					scanLoop(rangeStmt.Body)
				}
				return true
			})

			mu.Lock()
			issues = append(issues, localIssues...)
			mu.Unlock()
		}(file)
	}
	wg.Wait()

	for _, issue := range issues {
		color := "\033[1;31m" // Red
		fmt.Printf("%s[%s]\033[0m %s:%d -> %s\n", color, issue.Level, issue.File, issue.Line, issue.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v\n", time.Since(start))
}

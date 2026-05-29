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
	fmt.Println("💾 [" + "\033[1;34m" + "MEMORY SCANNER" + "\033[0m" + "] Mendiagnosis Stabilitas Stateful OS...")
	fset := token.NewFileSet()
	var issues []Issue
	var mu sync.Mutex

	var filesToScan []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || (info.IsDir() && (strings.Contains(path, ".git") || path == "scanner" || path == "_sgvp" || strings.HasPrefix(path, "_sgvp/") || strings.HasPrefix(path, "_sgvp\\") || path == "vendor")) {
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
			// rc117 FP-reduction: downgrade severity untuk file yang pola-nya
			// write-once (snapshot, temp, .meta) atau file di lokasi non-state.
			// Validated sample: session/filehistory.go pakai UnixNano ID per file
			// — tidak butuh atomic write karena write-once immutable.
			isWriteOnceContext := strings.Contains(path, "session/filehistory") ||
				strings.Contains(path, "session\\filehistory") ||
				strings.Contains(path, "snapshot") ||
				strings.Contains(path, "dreamstate") ||
				strings.Contains(path, "/cache/") || strings.Contains(path, "\\cache\\")
			ast.Inspect(node, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok {
							if sel.Sel.Name == "WriteFile" && (id.Name == "os" || id.Name == "ioutil") {
								severity := "CRITICAL"
								msg := "Memory Amensia: Resiko korup jika OS mati. Gunakan Atomic Write."
								if isWriteOnceContext {
									severity = "INFO"
									msg = "WriteFile di write-once/snapshot path — atomic tidak wajib (file di-ID unik per-write)."
								}
								localIssues = append(localIssues, Issue{severity, path, fset.Position(n.Pos()).Line, msg})
							}
							if sel.Sel.Name == "ReadAll" && (id.Name == "io" || id.Name == "ioutil") {
								localIssues = append(localIssues, Issue{"HIGH", path, fset.Position(n.Pos()).Line, "RAM OOM Bomb: Membaca memori tanpa batas io.LimitReader."})
							}
						}
					}
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
		color := "\033[1;35m"
		if issue.Level == "CRITICAL" {
			color = "\033[1;31m"
		}
		fmt.Printf("%s[%s]\033[0m %s:%d -> %s\n", color, issue.Level, issue.File, issue.Line, issue.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v\n", time.Since(start))
}

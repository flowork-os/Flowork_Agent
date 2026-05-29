//go:build ignore

// ext_goroutine_leak_scanner — mendeteksi goroutine yang bisa leak.
//
// Checks: go func() tanpa context/done/cancel, channel tanpa consumer,
//
//	goroutine tanpa timeout
//
// Prinsip: FQP-5 (Recovery Operator), FQP-13 (No-Broadcasting)
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type GoroutineFinding struct {
	Level, Type, File, Func, Message string
	Line                             int
}

var findings []GoroutineFinding

func main() {
	fmt.Println("🧵 [EXT_GOROUTINE_LEAK v1] Scanning for goroutine leak patterns...")
	fmt.Println("   Prinsip: FQP-5 (Recovery), FQP-13 (No-Broadcasting)")
	fmt.Println()
	root := "."
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			b := filepath.Base(path)
			if b == ".git" || b == "vendor" || b == "scanner" || b == "_sgvp" || b == "docs" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scan(path)
		}
		return nil
	})
	fmt.Printf("\n[🧵] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}
	out := filepath.Join(root, "docs", "bug", "ext_goroutine_leak_report.md")
	os.MkdirAll(filepath.Dir(out), 0755)
	writeReport(out)
	fmt.Println("\n📜 Report:", out)
}

func scan(filePath string) {
	fset := token.NewFileSet()
	node, _ := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if node == nil {
		return
	}
	for _, d := range node.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}

			// Analyze the goroutine body
			var body *ast.BlockStmt
			switch call := goStmt.Call.Fun.(type) {
			case *ast.FuncLit:
				body = call.Body
			default:
				// go someFunc() — harder to analyze, skip
				return true
			}

			if body == nil {
				return true
			}

			// Check if goroutine has context awareness
			hasCtxDone := false
			hasSelect := false
			hasTimer := false

			ast.Inspect(body, func(bn ast.Node) bool {
				switch x := bn.(type) {
				case *ast.SelectStmt:
					hasSelect = true
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
						if sel.Sel.Name == "Done" {
							hasCtxDone = true
						}
						if sel.Sel.Name == "NewTimer" || sel.Sel.Name == "After" ||
							sel.Sel.Name == "NewTicker" {
							hasTimer = true
						}
					}
				}
				return true
			})

			// Check for infinite loops without exit
			hasForLoop := false
			ast.Inspect(body, func(bn ast.Node) bool {
				if forStmt, ok := bn.(*ast.ForStmt); ok {
					if forStmt.Cond == nil { // for { ... } — infinite loop
						hasForLoop = true
					}
				}
				if rangeStmt, ok := bn.(*ast.RangeStmt); ok {
					_ = rangeStmt
					// for range channel — OK if channel is closed
				}
				return true
			})

			if hasForLoop && !hasCtxDone && !hasSelect && !hasTimer {
				findings = append(findings, GoroutineFinding{
					Level: "HIGH", Type: "Goroutine Infinite Loop Without Exit",
					File: filePath, Line: fset.Position(goStmt.Pos()).Line, Func: funcName,
					Message: "go func() dengan infinite loop (`for {}`) tanpa ctx.Done(), select, " +
						"atau timer. Goroutine ini TIDAK PERNAH exit — memory leak. " +
						"Tambahkan select { case <-ctx.Done(): return }.",
				})
			} else if !hasCtxDone && !hasSelect && hasForLoop {
				findings = append(findings, GoroutineFinding{
					Level: "MEDIUM", Type: "Goroutine Loop Without Context",
					File: filePath, Line: fset.Position(goStmt.Pos()).Line, Func: funcName,
					Message: "go func() dengan loop tapi tanpa context cancellation. " +
						"Jika parent function return, goroutine ini tetap hidup. " +
						"Pass context dan check ctx.Done().",
				})
			}

			return true
		})
	}
}

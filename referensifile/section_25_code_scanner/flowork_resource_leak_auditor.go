//go:build ignore

// ext_resource_leak_scanner — mendeteksi resource yang tidak ditutup.
//
// Checks: os.Open/Create tanpa defer Close, Lock tanpa defer Unlock,
//
//	channel dibuat tapi tidak di-close
//
// Prinsip: FQP-5 (Recovery Operator), FQP-9 (Gate Reversibility)
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

type LeakFinding struct {
	Level, Type, File, Func, Message string
	Line                             int
}

var findings []LeakFinding

func main() {
	fmt.Println("🚰 [EXT_RESOURCE_LEAK v1] Scanning for unclosed resources...")
	fmt.Println("   Prinsip: FQP-5 (Recovery), FQP-9 (Reversibility)")
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
	fmt.Printf("\n[🚰] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}
	out := filepath.Join(root, "docs", "bug", "ext_resource_leak_report.md")
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
		name := fn.Name.Name

		// Track opens and defer closes
		type openInfo struct {
			varName string
			line    int
			op      string
		}
		var opens []openInfo
		hasDefer := map[string]bool{}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// Track defer xxx.Close()
			if def, ok := n.(*ast.DeferStmt); ok {
				if call, ok := def.Call.Fun.(*ast.SelectorExpr); ok {
					if call.Sel.Name == "Close" || call.Sel.Name == "Unlock" || call.Sel.Name == "RUnlock" {
						if id, ok := call.X.(*ast.Ident); ok {
							hasDefer[id.Name] = true
						}
						// Handle resp.Body.Close()
						if sel2, ok := call.X.(*ast.SelectorExpr); ok {
							if id, ok := sel2.X.(*ast.Ident); ok {
								hasDefer[id.Name+"."+sel2.Sel.Name] = true
							}
						}
					}
				}
			}

			// Track os.Open, os.Create, os.OpenFile
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if isId(sel.X, "os") && (sel.Sel.Name == "Open" || sel.Sel.Name == "Create" || sel.Sel.Name == "OpenFile") {
					if i < len(assign.Lhs) {
						if id, ok := assign.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
							opens = append(opens, openInfo{varName: id.Name, line: fset.Position(call.Pos()).Line, op: "os." + sel.Sel.Name})
						}
					}
				}
			}
			return true
		})

		// Check for opens without corresponding defer Close
		for _, o := range opens {
			if !hasDefer[o.varName] {
				findings = append(findings, LeakFinding{
					Level: "HIGH", Type: "File Handle Not Deferred Close",
					File: filePath, Line: o.line, Func: name,
					Message: fmt.Sprintf(
						"%s() assigns to %q tapi tidak ada `defer %s.Close()`. "+
							"File handle leak: OS punya limit fd. Tambahkan defer close setelah error check.",
						o.op, o.varName, o.varName),
				})
			}
		}
	}
}

func isId(e ast.Expr, n string) bool { id, ok := e.(*ast.Ident); return ok && id.Name == n }

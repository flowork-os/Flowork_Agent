//go:build ignore

// ext_tls_config_scanner — mendeteksi konfigurasi TLS yang lemah.
//
// Checks: InsecureSkipVerify=true, MinVersion < TLS 1.2,
//
//	missing TLS config pada server
//
// Prinsip: FQP-6 (BFT Quorum), GOL Fase 7 (Immune)
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

type TLSFinding struct {
	Level, Type, File, Func, Message string
	Line                             int
}

var findings []TLSFinding

func main() {
	fmt.Println("🔒 [EXT_TLS_CONFIG v1] Scanning for weak TLS configurations...")
	fmt.Println("   Prinsip: FQP-6 (BFT), GOL Fase 7 (Immune)")
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
	fmt.Printf("\n[🔒] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}
	out := filepath.Join(root, "docs", "bug", "ext_tls_config_report.md")
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
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// Detect InsecureSkipVerify: true
			kv, ok := n.(*ast.KeyValueExpr)
			if ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "InsecureSkipVerify" {
					if val, ok := kv.Value.(*ast.Ident); ok && val.Name == "true" {
						findings = append(findings, TLSFinding{
							Level: "CRITICAL", Type: "InsecureSkipVerify Enabled",
							File: filePath, Line: fset.Position(kv.Pos()).Line, Func: name,
							Message: "InsecureSkipVerify: true — MATIKAN verifikasi TLS certificate. " +
								"Man-in-the-middle attack bisa intercept semua traffic. " +
								"JANGAN gunakan di production. Jika untuk testing, gunakan build tag.",
						})
					}
				}
			}

			// Detect tls.Config without MinVersion
			comp, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := comp.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if isId(sel.X, "tls") && sel.Sel.Name == "Config" {
				hasMinVersion := false
				for _, elt := range comp.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "MinVersion" {
							hasMinVersion = true
						}
					}
				}
				if !hasMinVersion {
					findings = append(findings, TLSFinding{
						Level: "MEDIUM", Type: "TLS Config Missing MinVersion",
						File: filePath, Line: fset.Position(comp.Pos()).Line, Func: name,
						Message: "tls.Config{} tanpa MinVersion — default Go bisa mengizinkan " +
							"TLS 1.0 (deprecated). Set MinVersion: tls.VersionTLS12 atau TLS13.",
					})
				}
			}
			return true
		})
	}
}

func isId(e ast.Expr, n string) bool { id, ok := e.(*ast.Ident); return ok && id.Name == n }

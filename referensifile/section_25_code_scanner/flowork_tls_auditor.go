//go:build ignore

// tls_insecure_skip_scanner — Tier 3 CATEGORY V (FAANG-Grade Resiliency).
//
// Fungsi: mendeteksi `tls.Config{InsecureSkipVerify: true}` di codebase.
// Flag ini disable TLS cert verification → MITM attack surface + credential
// exfil. Tidak boleh dipakai even untuk testing (pakai test-specific server
// dengan self-signed cert + RootCAs pool).
//
// Pattern detect: KeyValueExpr `InsecureSkipVerify: true` di AST.
// False positive guard: flag literal `true` only, bukan variable.
//
// Policy: tiap flag CRITICAL — zero tolerance per GOL_FLOWORK.MD security.
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
	fmt.Println("🔓 [\033[1;31mTLS INSECURE SKIP SCANNER\033[0m] Cari InsecureSkipVerify: true di TLS config...")
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
			ast.Inspect(node, func(n ast.Node) bool {
				kv, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				idKey, ok := kv.Key.(*ast.Ident)
				if !ok || idKey.Name != "InsecureSkipVerify" {
					return true
				}
				// Flag hanya kalau value literal `true`.
				// `var skip = cfg.Dev; tls.Config{InsecureSkipVerify: skip}`
				// tidak di-flag (value dari variable, trust caller).
				idVal, ok := kv.Value.(*ast.Ident)
				if !ok || idVal.Name != "true" {
					return true
				}
				localIssues = append(localIssues, Issue{
					"CRITICAL", path, fset.Position(n.Pos()).Line,
					"TLS Insecure Skip: `InsecureSkipVerify: true` disable cert verification — MITM + credential exfil risk. Pakai RootCAs pool dengan cert self-signed kalau butuh testing, jangan skip verify.",
				})
				return true
			})

			mu.Lock()
			issues = append(issues, localIssues...)
			mu.Unlock()
		}(file)
	}
	wg.Wait()

	if len(issues) == 0 {
		fmt.Println("✅ Tidak ada InsecureSkipVerify: true di codebase.")
	}
	for _, issue := range issues {
		fmt.Printf("\033[1;31m[%s]\033[0m %s:%d -> %s\n", issue.Level, issue.File, issue.Line, issue.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v | %d temuan\n", time.Since(start), len(issues))
}

//go:build ignore

// sandbox_escape_scanner — Tier 3 CATEGORY VII (Network Attack Shields).
//
// Fungsi: mendeteksi pattern `exec.Command("bash", "-c", <var>)` atau shell
// interpreter yang menerima user-input langsung. Ini lubang RCE klasik:
// AI halusinasi kirim string ke bash -c → arbitrary code execution.
//
// Pattern detect:
//   - exec.Command("bash", "-c", ...) / exec.CommandContext(..., "bash", "-c", ...)
//   - exec.Command("sh", "-c", ...) / exec.CommandContext(..., "sh", "-c", ...)
//   - exec.Command("cmd", "/C", ...) / exec.Command("cmd.exe", "/C", ...)
//   - exec.Command("powershell", "-Command", <var>) kalau arg bukan literal const
//
// Tidak flag:
//   - exec.Command("bash") tanpa -c (interactive shell, bukan direct exec)
//   - Pure argv invocation: exec.Command("git", "push") — tiap arg jadi argv,
//     tidak shell-parsed. Safe.
//
// Alasan policy: bash -c = shell interpret → expand $var, backtick, pipe.
// Kalau ada variable input di string third-arg, attacker bisa inject ; && |.
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

// shellInterpreters + flag yang trigger shell-parse mode.
var shellTriggers = map[string][]string{
	"bash":       {"-c"},
	"sh":         {"-c"},
	"zsh":        {"-c"},
	"fish":       {"-c"},
	"cmd":        {"/c", "/C"},
	"cmd.exe":    {"/c", "/C"},
	"powershell": {"-Command", "-command", "-c", "-C"},
	"pwsh":       {"-Command", "-command", "-c", "-C"},
}

func main() {
	start := time.Now()
	fmt.Println("🚪 [\033[1;31mSANDBOX ESCAPE SCANNER\033[0m] Cari exec.Command(shell, '-c', ...) pattern RCE...")
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
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				idPkg, ok := sel.X.(*ast.Ident)
				if !ok || idPkg.Name != "exec" {
					return true
				}
				var shellIdx, flagIdx, payloadIdx int
				switch sel.Sel.Name {
				case "Command":
					// exec.Command(name, args...) — args[0]=name
					shellIdx, flagIdx, payloadIdx = 0, 1, 2
				case "CommandContext":
					// exec.CommandContext(ctx, name, args...) — args[1]=name
					shellIdx, flagIdx, payloadIdx = 1, 2, 3
				default:
					return true
				}
				if len(call.Args) <= payloadIdx {
					return true
				}
				shellLit, ok := call.Args[shellIdx].(*ast.BasicLit)
				if !ok || shellLit.Kind != token.STRING {
					return true
				}
				shell := strings.Trim(shellLit.Value, `"`)
				// Strip path prefix if any (e.g. "/bin/bash" → "bash")
				shellBase := filepath.Base(shell)
				triggers, known := shellTriggers[shellBase]
				if !known {
					return true
				}
				flagArg, ok := call.Args[flagIdx].(*ast.BasicLit)
				if !ok || flagArg.Kind != token.STRING {
					return true
				}
				flag := strings.Trim(flagArg.Value, `"`)
				triggered := false
				for _, t := range triggers {
					if flag == t {
						triggered = true
						break
					}
				}
				if !triggered {
					return true
				}
				// Shell-parse mode confirmed. Flag regardless of payload origin
				// — even "literal-only" bash -c is brittle kalau future refactor
				// swap ke variable. Policy: pakai argv split, bukan shell.
				localIssues = append(localIssues, Issue{
					"CRITICAL", path, fset.Position(n.Pos()).Line,
					fmt.Sprintf("Sandbox Escape Risk: %s.%s(%q, %q, ...) shell-parse mode — kalau arg-3 dari variable, RCE via ; && | expansion. Pakai exec.Command(%q, \"arg1\", \"arg2\", ...) argv split, tanpa -c.", idPkg.Name, sel.Sel.Name, shell, flag, shellBase),
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
		fmt.Println("✅ Tidak ada exec.Command shell-parse mode pattern.")
	}
	for _, issue := range issues {
		fmt.Printf("\033[1;31m[%s]\033[0m %s:%d -> %s\n", issue.Level, issue.File, issue.Line, issue.Message)
	}
	fmt.Printf("\n⏱️  Selesai dalam %v | %d temuan\n", time.Since(start), len(issues))
}

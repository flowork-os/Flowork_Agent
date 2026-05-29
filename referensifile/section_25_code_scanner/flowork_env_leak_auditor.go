//go:build ignore

// ext_env_leak_scanner — mendeteksi kebocoran os.Environ() ke subprocess.
//
// EXTBUG-003 & EXTBUG-020: Menemukan pattern `cmd.Env = os.Environ()` atau
// `append(os.Environ(), ...)` dimana seluruh env (termasuk API keys) dikirim
// ke subprocess pihak ketiga (MCP servers, hook shells, dll).
//
// Prinsip Kuantum Dilanggar: FQP-4 (SGVP), FQP-6 (BFT Quorum)
// GOL_FLOWORK: §C (Sensitive Config), §F (Gerbang Pertahanan)
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

type EnvLeakFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var envLeakFindings []EnvLeakFinding

// safeCallers are functions known to properly filter env vars.
var safeCallers = []string{
	"safeGitEnv", "filterEnv", "buildSandboxEnv", "buildMCPSafeEnv",
}

func main() {
	fmt.Println("🔑 [EXT_ENV_LEAK v1] Scanning for os.Environ() leak to subprocesses...")
	fmt.Println("   Prinsip: FQP-4 (SGVP Guard), GOL §C (Sensitive Config)")
	fmt.Println()

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "scanner" || base == "_sgvp" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanEnvLeak(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("❌ Walk error:", err)
		return
	}

	// Print results
	fmt.Printf("\n[🔑] Selesai! Findings: %d\n", len(envLeakFindings))
	for _, f := range envLeakFindings {
		fmt.Printf("🚨 [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_env_leak_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeEnvLeakReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanEnvLeak(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			funcName = "(*T)." + fn.Name.Name
		}

		// Skip known-safe functions
		isSafe := false
		for _, safe := range safeCallers {
			if strings.Contains(funcName, safe) {
				isSafe = true
				break
			}
		}
		if isSafe {
			continue
		}

		// Check 1: cmd.Env = os.Environ() or cmd.Env = append(os.Environ(), ...)
		hasExecCmd := false
		hasOsEnviron := false
		osEnvironLine := 0

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				// Detect exec.Command / exec.CommandContext
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						if id.Name == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
							hasExecCmd = true
						}
					}
				}
				// Detect os.Environ()
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						if id.Name == "os" && sel.Sel.Name == "Environ" {
							hasOsEnviron = true
							osEnvironLine = fset.Position(x.Pos()).Line
						}
					}
				}
			case *ast.AssignStmt:
				// Detect cmd.Env = ... patterns
				for _, lhs := range x.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok {
						if sel.Sel.Name == "Env" {
							// Check if RHS contains os.Environ()
							for _, rhs := range x.Rhs {
								if containsOsEnviron(rhs) {
									envLeakFindings = append(envLeakFindings, EnvLeakFinding{
										Level:   "CRITICAL",
										Type:    "Full Env Leak to Subprocess",
										File:    filePath,
										Line:    fset.Position(x.Pos()).Line,
										Func:    funcName,
										Message: "cmd.Env = os.Environ() melewatkan SELURUH env (API keys, tokens, passwords) ke subprocess. Gunakan whitelist env filter seperti safeGitEnv() atau buildSandboxEnv().",
									})
								}
							}
						}
					}
				}
			}
			return true
		})

		// Check 2: Function has both exec.Command and os.Environ in same scope
		// but not caught by assign pattern (e.g. via helper function)
		if hasExecCmd && hasOsEnviron && osEnvironLine > 0 {
			// Already caught above? Check for duplicates
			alreadyCaught := false
			for _, f := range envLeakFindings {
				if f.File == filePath && f.Func == funcName {
					alreadyCaught = true
					break
				}
			}
			if !alreadyCaught {
				envLeakFindings = append(envLeakFindings, EnvLeakFinding{
					Level:   "HIGH",
					Type:    "os.Environ() in Subprocess Context",
					File:    filePath,
					Line:    osEnvironLine,
					Func:    funcName,
					Message: "os.Environ() digunakan dalam fungsi yang juga membuat subprocess (exec.Command). Jika env dikirim ke cmd tanpa filter, API keys bisa bocor.",
				})
			}
		}
	}
}

func containsOsEnviron(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					if id.Name == "os" && sel.Sel.Name == "Environ" {
						found = true
					}
				}
			}
		}
		return true
	})
	return found
}

func writeEnvLeakReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 🔑 EXT Environment Leak Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_env_leak_scanner v1\n")
	out.WriteString("> **Prinsip:** FQP-4 (SGVP), FQP-6 (BFT), GOL §C (Sensitive Config)\n")
	out.WriteString("> **Target:** `os.Environ()` passed to subprocess tanpa filter\n\n")

	if len(envLeakFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan kebocoran environment ke subprocess.*\n")
		return
	}

	out.WriteString(fmt.Sprintf("**Total Findings: %d**\n\n", len(envLeakFindings)))

	for i, f := range envLeakFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

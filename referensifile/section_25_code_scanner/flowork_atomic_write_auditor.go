//go:build ignore

// ext_atomic_write_scanner — mendeteksi os.WriteFile tanpa pattern atomic
// (tmp+rename) pada file state-critical dan credential.
//
// EXTBUG-005, -017, -022, -023: Crash mid-write pada file credential/state
// menghasilkan file corrupt. Pattern yang benar: write ke .tmp, lalu
// os.Rename() (atomic pada POSIX filesystem).
//
// Prinsip Kuantum Dilanggar: FQP-13 (No-Broadcasting), FQP-9 (Gate Reversibility)
// GOL_FLOWORK: §K (SELF-UPDATE), §D (Atomicity Wajib)
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

type AtomicWriteFinding struct {
	Level    string
	Type     string
	File     string
	Line     int
	Func     string
	PathHint string
	Message  string
}

var atomicFindings []AtomicWriteFinding

// criticalPathPatterns — paths that MUST use atomic write.
var criticalPathPatterns = []string{
	// Credential/auth files
	"token", "oauth", "auth", "credential", "keychain",
	"immune.key", "owner.hash", ".key",
	// State files
	"settings.json", "config.yaml", "config.json",
	"registry.json", "bridge.json", "cron",
	"state", ".flowork/", "memory/",
	"facts.jsonl", "mood.json", "manifest",
	// Wallet/finance
	"wallet", "portfolio", "balance",
}

// Functions that already do atomic write — skip these.
var atomicSafeFuncs = []string{
	"sessionAllow", "atomicWrite", "AtomicWrite",
}

func main() {
	fmt.Println("💾 [EXT_ATOMIC_WRITE v1] Scanning for non-atomic writes on critical files...")
	fmt.Println("   Prinsip: FQP-13 (No-Broadcasting), FQP-9 (Gate Reversibility)")
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
			scanAtomicWrite(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("❌ Walk error:", err)
		return
	}

	fmt.Printf("\n[💾] Selesai! Findings: %d\n", len(atomicFindings))
	for _, f := range atomicFindings {
		fmt.Printf("🚨 [%s] %s | %s:%d (func %s)\n   path_hint=%q\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.PathHint, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_atomic_write_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeAtomicReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanAtomicWrite(filePath string) {
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

		// Skip known-safe functions
		for _, safe := range atomicSafeFuncs {
			if strings.Contains(funcName, safe) {
				return
			}
		}

		// Check if function already has os.Rename (atomic pattern)
		hasRename := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if isIdentName(sel.X, "os") && sel.Sel.Name == "Rename" {
						hasRename = true
					}
				}
			}
			return !hasRename
		})

		// If function already uses Rename, it likely does atomic write — skip
		if hasRename {
			continue
		}

		// Find os.WriteFile calls with critical paths
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if !isIdentName(sel.X, "os") || sel.Sel.Name != "WriteFile" {
				return true
			}
			if len(call.Args) < 1 {
				return true
			}

			// Extract path hint from first argument
			pathHint := extractPathHint(call.Args[0])
			if pathHint == "" {
				return true
			}

			// Check if path matches critical patterns
			low := strings.ToLower(pathHint)
			for _, pat := range criticalPathPatterns {
				if strings.Contains(low, pat) {
					level := "HIGH"
					if strings.Contains(low, "token") || strings.Contains(low, "key") ||
						strings.Contains(low, "credential") || strings.Contains(low, "auth") ||
						strings.Contains(low, "config") {
						level = "CRITICAL"
					}

					// Check permission: 0644 on sensitive file?
					permIssue := ""
					if len(call.Args) >= 3 {
						permStr := fmt.Sprintf("%v", call.Args[2])
						if strings.Contains(permStr, "0644") || strings.Contains(permStr, "0o644") {
							permIssue = " Tambahan: permission 0644 (world-readable) pada file sensitif. Harus 0600."
						}
					}

					atomicFindings = append(atomicFindings, AtomicWriteFinding{
						Level:    level,
						Type:     "Non-Atomic Critical Write",
						File:     filePath,
						Line:     fset.Position(call.Pos()).Line,
						Func:     funcName,
						PathHint: pathHint,
						Message: fmt.Sprintf(
							"os.WriteFile ke path critical %q tanpa atomic pattern (tmp+Rename). "+
								"Crash mid-write = file corrupt. Gunakan: write ke .tmp → os.Rename().%s",
							pat, permIssue),
					})
					break
				}
			}
			return true
		})
	}
}

func extractPathHint(e ast.Expr) string {
	var parts []string
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				parts = append(parts, strings.Trim(x.Value, "`\""))
			}
		case *ast.Ident:
			if x.Name != "" {
				parts = append(parts, x.Name)
			}
		}
		return true
	})
	return strings.Join(parts, "/")
}

func isIdentName(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func writeAtomicReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 💾 EXT Atomic Write Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_atomic_write_scanner v1\n")
	out.WriteString("> **Prinsip:** FQP-13 (No-Broadcasting), FQP-9 (Gate Reversibility)\n")
	out.WriteString("> **Target:** `os.WriteFile` pada credential/state files tanpa atomic pattern (tmp+Rename)\n\n")

	if len(atomicFindings) == 0 {
		out.WriteString("✅ *Semua critical writes sudah menggunakan atomic pattern.*\n")
		return
	}

	crit := 0
	high := 0
	for _, f := range atomicFindings {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		}
	}

	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 Critical: %d | 🟠 High: %d)\n\n", len(atomicFindings), crit, high))

	for i, f := range atomicFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Path hint:** `%s`\n", f.PathHint))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

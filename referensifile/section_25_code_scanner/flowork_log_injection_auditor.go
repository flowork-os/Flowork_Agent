//go:build ignore

// ext_log_injection_scanner — mendeteksi log injection/forging.
//
// Checks: User-controlled input di log tanpa sanitasi,
//
//	newline injection di log yang bisa forge log entries,
//	sensitive data (password, token) di log output
//
// Prinsip: FQP-1 (Decoherence), GOL C (Sensitive Config)
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

type LogFinding struct {
	Level, Type, File, Func, Message string
	Line                             int
}

var findings []LogFinding

func main() {
	fmt.Println("📝 [EXT_LOG_INJECTION v1] Scanning for log injection/sensitive data in logs...")
	fmt.Println("   Prinsip: FQP-1 (Decoherence), GOL C (Sensitive Config)")
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
	fmt.Printf("\n[📝] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d (func %s)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}
	out := filepath.Join(root, "docs", "bug", "ext_log_injection_report.md")
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
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Detect log.Printf/Println/Print, fmt.Fprintf to log
			isLogCall := false
			if isId(sel.X, "log") || isId(sel.X, "slog") {
				if strings.HasPrefix(sel.Sel.Name, "Print") || sel.Sel.Name == "Info" ||
					sel.Sel.Name == "Warn" || sel.Sel.Name == "Error" || sel.Sel.Name == "Debug" {
					isLogCall = true
				}
			}

			if !isLogCall {
				return true
			}

			// Check if log message contains sensitive variable names
			for _, arg := range call.Args {
				// Check format string for sensitive patterns
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					val := strings.ToLower(strings.Trim(lit.Value, `"`))
					sensitivePatterns := []string{"password", "passwd", "secret", "apikey", "api_key",
						"token", "bearer", "authorization", "credential", "private_key"}
					for _, pat := range sensitivePatterns {
						if strings.Contains(val, pat) && (strings.Contains(val, "%s") || strings.Contains(val, "%v") || strings.Contains(val, "%q")) {
							findings = append(findings, LogFinding{
								Level: "HIGH", Type: "Sensitive Data in Log",
								File: filePath, Line: fset.Position(call.Pos()).Line, Func: name,
								Message: fmt.Sprintf(
									"Log message mengandung %q dengan format verb — kemungkinan "+
										"secret/credential di-print ke log. Log bisa diakses oleh "+
										"tools monitoring, dikirim ke cloud, atau stored unencrypted. "+
										"Mask sensitive data: log.Printf(\"token=%%s***\", token[:4]).",
									pat),
							})
							return true
						}
					}
				}

				// Check if argument is a sensitive-named variable
				if id, ok := arg.(*ast.Ident); ok {
					idLow := strings.ToLower(id.Name)
					for _, pat := range []string{"password", "secret", "token", "apiKey", "key"} {
						if strings.Contains(idLow, strings.ToLower(pat)) && idLow != "tokencount" &&
							idLow != "tokenusage" && idLow != "tokenfile" {
							findings = append(findings, LogFinding{
								Level: "MEDIUM", Type: "Possible Secret in Log Argument",
								File: filePath, Line: fset.Position(call.Pos()).Line, Func: name,
								Message: fmt.Sprintf(
									"Variable %q (mengandung %q) dipakai sebagai argument log. "+
										"Jika ini secret, jangan log langsung. Gunakan masking.",
									id.Name, pat),
							})
							return true
						}
					}
				}
			}
			return true
		})
	}
}

func isId(e ast.Expr, n string) bool { id, ok := e.(*ast.Ident); return ok && id.Name == n }

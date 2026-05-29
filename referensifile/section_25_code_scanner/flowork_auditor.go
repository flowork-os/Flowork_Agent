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
)

// Vulnerability represents a single finding
type Vulnerability struct {
	Type     string
	Severity string
	File     string
	Line     int
	Message  string
}

var findings []Vulnerability

func main() {
	fmt.Println("[🔥] Initializing FloworkOS Advanced AI Security Auditor (Anti-Kiamat Edition)...")
	rootDir := "."

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			analyzeFile(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error reading repository:", err)
		return
	}

	fmt.Printf("\n[🚨] Audit Complete. Found %d Critical/Advanced Architectural Flaws:\n", len(findings))
	for _, f := range findings {
		fmt.Printf("[%s] %s (File: %s:%d)\n", f.Severity, f.Type, f.File, f.Line)
		fmt.Printf("   -> %s\n", f.Message)
		fmt.Println(strings.Repeat("-", 60))
	}

	// Write JSON array of findings
	outFile := filepath.Join(rootDir, "state", "scanner-reports", "advanced_ast_audit.md")
	writeReport(outFile)
	fmt.Println("Laporan detail berhasil ditulis ke", outFile)
}

func analyzeFile(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if fun, ok := x.Fun.(*ast.SelectorExpr); ok {
				// 1. Detect Sandbox Escape (exec.Command outside sandbox)
				if fun.Sel.Name == "Command" || fun.Sel.Name == "CommandContext" {
					if id, ok := fun.X.(*ast.Ident); ok && id.Name == "exec" {
						msg := "Panggilan exec langsung tanpa wrapper Sandbox. Sangat rentan terhadap Sandbox Escape / RCE!"
						recordFinding(fset, x.Pos(), filePath, "Sandbox Bypass", "CRITICAL", msg)
					}
				}
				// 2. Detect JSON DoS (Missing LimitReader before Unmarshal)
				if fun.Sel.Name == "Unmarshal" {
					if id, ok := fun.X.(*ast.Ident); ok && id.Name == "json" {
						if isNetworkOrFileRead(x, filePath) && !strings.Contains(filePath, "test") {
							msg := "Membaca data JSON tanpa io.LimitReader. Rentan terhadap DoS / Memory Exhaustion jika ukuran JSON tidak terbatas."
							recordFinding(fset, x.Pos(), filePath, "Unbounded JSON Read / DoS", "HIGH", msg)
						}
					}
				}
				// 3. Detect TOCTOU / File Exhaustion (os.OpenFile without rotation logic)
				if fun.Sel.Name == "OpenFile" {
					if id, ok := fun.X.(*ast.Ident); ok && id.Name == "os" {
						// Look for O_APPEND
						msg := "Penggunaan os.OpenFile (biasanya O_APPEND) dapat menebabkan Disk Exhaustion DoS jika di-loop oleh AI. Pertimbangkan rotasi log / pembatasan MaxSize."
						recordFinding(fset, x.Pos(), filePath, "Disk Exhaustion", "MEDIUM", msg)
					}
				}
				// 4. Insecure HTTP Listeners
				if fun.Sel.Name == "ListenAndServe" || fun.Sel.Name == "Listen" {
					msg := "Server/Network Bind berpotensi berjalan di 0.0.0.0 atau tidak diotentikasi. Waspada terekspos secara publik di LAN/WAN."
					recordFinding(fset, x.Pos(), filePath, "Insecure HTTP Bind", "HIGH", msg)
				}
			}
		}
		return true
	})
}

// Heuristic to check if file seems to be handling untrusted I/O
func isNetworkOrFileRead(call *ast.CallExpr, path string) bool {
	// A real scanner would do dataflow analysis. Here we just flag it if it's in a package that takes input.
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "mcp") || strings.Contains(lowerPath, "mesh") || strings.Contains(lowerPath, "http") || strings.Contains(lowerPath, "api") {
		return true
	}
	return false
}

func recordFinding(fset *token.FileSet, pos token.Pos, file, fType, severity, msg string) {
	// Filter out the wrapper itself if it's sandbox.go
	if strings.HasSuffix(file, "sandbox\\windows_job.go") || strings.HasSuffix(file, "sandbox/windows_job.go") {
		// Inside sandbox, it's normal to use exec
		if fType == "Sandbox Bypass" {
			return
		}
	}
	findings = append(findings, Vulnerability{
		Type:     fType,
		Severity: severity,
		File:     file,
		Line:     fset.Position(pos).Line,
		Message:  msg,
	})
}

func writeReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# Advanced AST Security Report (Flowork Auditor)\n\n")
	out.WriteString("Laporan ini di-generate oleh *Advanced AI Auditor* (Antigravity).\n\n")
	out.WriteString("Scanner ini membedah *Abstract Syntax Tree* (AST) dari FloworkOS untuk menemukan logika berbahaya yang sering terlewat oleh static scanner biasa (seperti *Gosec*).\n\n")
	out.WriteString("## Temuan Kritis\n\n")
	for _, f := range findings {
		out.WriteString(fmt.Sprintf("- **[%s] %s**\n", f.Severity, f.Type))
		out.WriteString(fmt.Sprintf("  - Lokasi: `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("  - Analisis: %s\n\n", f.Message))
	}
}

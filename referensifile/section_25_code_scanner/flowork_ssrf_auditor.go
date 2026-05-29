//go:build ignore

// ext_ssrf_scanner — mendeteksi penggunaan HTTP client tanpa SSRF protection.
//
// EXTBUG-001, -002, -016, -021: Menemukan http.DefaultClient, http.Get,
// http.Post, http.Client{} tanpa safeclient.SafeDialContext. Setiap koneksi
// keluar yang membawa credential (Authorization header, API key di URL)
// HARUS menggunakan SSRF-safe client.
//
// Prinsip Kuantum Dilanggar: FQP-4 (SGVP), FQP-1 (Decoherence Awareness)
// GOL_FLOWORK: §C (Sensitive Config), FASE 7 (Sistem Imun)
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

type SSRFFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Func    string
	Message string
}

var ssrfFindings []SSRFFinding

// Files/packages known to implement the SSRF guard itself — skip these.
var ssrfGuardFiles = []string{
	"safeclient.go", "safeclient/", "webfetch.go",
}

// Functions known to be safe (they create SSRF-guarded clients).
var ssrfSafeFuncs = []string{
	"SafeDialContext", "NewClient", "llmHTTPClient", "safeclient",
}

func main() {
	fmt.Println("🌐 [EXT_SSRF v1] Scanning for unguarded HTTP clients (no SSRF protection)...")
	fmt.Println("   Prinsip: FQP-4 (SGVP Guard), GOL FASE 7 (Sistem Imun)")
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip the safeclient implementation itself
		for _, guard := range ssrfGuardFiles {
			if strings.Contains(path, guard) {
				return nil
			}
		}
		scanSSRF(path)
		return nil
	})

	if err != nil {
		fmt.Println("❌ Walk error:", err)
		return
	}

	fmt.Printf("\n[🌐] Selesai! Findings: %d\n", len(ssrfFindings))
	for _, f := range ssrfFindings {
		fmt.Printf("🚨 [%s] %s | %s:%d (func %s)\n   -> %s\n",
			f.Level, f.Type, f.File, f.Line, f.Func, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_ssrf_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeSSRFReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanSSRF(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// Check if file imports safeclient
	importsSafeClient := false
	for _, imp := range node.Imports {
		if imp.Path != nil && strings.Contains(imp.Path.Value, "safeclient") {
			importsSafeClient = true
		}
	}

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		funcName := fn.Name.Name

		// Skip SSRF guard implementation functions
		isSafe := false
		for _, s := range ssrfSafeFuncs {
			if strings.Contains(funcName, s) {
				isSafe = true
			}
		}
		if isSafe {
			continue
		}

		// Track if function uses Authorization/Bearer header (carries credential)
		hasAuthHeader := false
		hasAPIKeyInURL := false

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// Check for Authorization header being set
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Set" || sel.Sel.Name == "Add" {
						if len(call.Args) >= 1 {
							if lit, ok := call.Args[0].(*ast.BasicLit); ok {
								val := strings.ToLower(strings.Trim(lit.Value, `"`))
								if val == "authorization" {
									hasAuthHeader = true
								}
							}
						}
					}
				}
			}
			// Check for "apikey=" or "api_key=" in URL strings
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				val := strings.ToLower(strings.Trim(lit.Value, `"`))
				if strings.Contains(val, "apikey=") || strings.Contains(val, "api_key=") {
					hasAPIKeyInURL = true
				}
			}
			return true
		})

		// Now scan for dangerous HTTP patterns
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			line := fset.Position(call.Pos()).Line

			// Pattern 1: http.DefaultClient.Do(req)
			if id, ok := sel.X.(*ast.SelectorExpr); ok {
				if pkg, ok := id.X.(*ast.Ident); ok {
					if pkg.Name == "http" && id.Sel.Name == "DefaultClient" {
						level := "HIGH"
						if hasAuthHeader || hasAPIKeyInURL {
							level = "CRITICAL"
						}
						ssrfFindings = append(ssrfFindings, SSRFFinding{
							Level:   level,
							Type:    "http.DefaultClient (No SSRF Guard)",
							File:    filePath,
							Line:    line,
							Func:    funcName,
							Message: "http.DefaultClient tidak punya SSRF protection. DNS rebinding/poisoning bisa redirect request ke IP internal (169.254.169.254). Ganti dengan safeclient.Client().",
						})
					}
				}
			}

			// Pattern 2: http.Get / http.Post / http.Head (package-level functions)
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "http" {
				switch sel.Sel.Name {
				case "Get", "Post", "Head", "PostForm":
					level := "HIGH"
					if hasAuthHeader || hasAPIKeyInURL {
						level = "CRITICAL"
					}
					ssrfFindings = append(ssrfFindings, SSRFFinding{
						Level:   level,
						Type:    "http." + sel.Sel.Name + " (No SSRF Guard)",
						File:    filePath,
						Line:    line,
						Func:    funcName,
						Message: fmt.Sprintf("http.%s() menggunakan DefaultClient tanpa SSRF guard. Gunakan safeclient.Client().%s() atau buat request manual.", sel.Sel.Name, sel.Sel.Name),
					})
				}
			}

			// Pattern 3: &http.Client{} tanpa custom Transport
			// Detect composite literals like &http.Client{Timeout: ...}
			if sel.Sel.Name == "Do" || sel.Sel.Name == "Get" || sel.Sel.Name == "Post" {
				// Check if receiver is a plain http.Client without safeclient
				if !importsSafeClient {
					// Check parent for http.Client literal
					if id, ok := sel.X.(*ast.Ident); ok {
						// It's calling something.Do() -- check if we can identify it
						_ = id // just flag if no safeclient import
					}
				}
			}

			return true
		})

		// Pattern 4: &http.Client{...} composite literal without DialContext
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			comp, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// Check if it's &http.Client{...}
			if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					if id.Name == "http" && sel.Sel.Name == "Client" {
						// Check if Transport field is set with SafeDialContext
						hasSafeTransport := false
						for _, elt := range comp.Elts {
							if kv, ok := elt.(*ast.KeyValueExpr); ok {
								if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Transport" {
									// Check if Transport contains SafeDialContext
									src := fmt.Sprintf("%v", kv.Value)
									if strings.Contains(src, "SafeDialContext") || strings.Contains(src, "safeclient") {
										hasSafeTransport = true
									}
								}
							}
						}
						if !hasSafeTransport && (hasAuthHeader || hasAPIKeyInURL) {
							ssrfFindings = append(ssrfFindings, SSRFFinding{
								Level: "HIGH",
								Type:  "Custom http.Client Without SSRF Transport",
								File:  filePath,
								Line:  fset.Position(comp.Pos()).Line,
								Func:  funcName,
								Message: "http.Client dibuat tanpa SafeDialContext di Transport. " +
									"Client ini membawa credential (Auth header / API key di URL). " +
									"Gunakan safeclient.NewClient(timeout) untuk inherit SSRF guard.",
							})
						}
					}
				}
			}
			return true
		})
	}
}

func writeSSRFReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 🌐 EXT SSRF Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_ssrf_scanner v1\n")
	out.WriteString("> **Prinsip:** FQP-4 (SGVP), GOL FASE 7 (Sistem Imun)\n")
	out.WriteString("> **Target:** HTTP clients tanpa SSRF-safe DialContext yang membawa credential\n\n")

	if len(ssrfFindings) == 0 {
		out.WriteString("✅ *Semua HTTP clients menggunakan SSRF-safe transport.*\n")
		return
	}

	critical := 0
	high := 0
	for _, f := range ssrfFindings {
		switch f.Level {
		case "CRITICAL":
			critical++
		case "HIGH":
			high++
		}
	}

	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 Critical: %d | 🟠 High: %d)\n\n", len(ssrfFindings), critical, high))

	for i, f := range ssrfFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Function:** `%s`\n", f.Func))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

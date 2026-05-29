//go:build ignore

// Package budgetgate — scanner baru rc113 yang track LLM provider calls vs
// BudgetGuard coverage. Per memory project_crypto-survival-arch.md:
// FLOWORK_BUDGET_GUARD cegah wallet drain saat loop runaway. 1 path
// provider call yang SKIP BudgetGuard = potensi bleed $ dari wallet
// 0xd129...7eb1.
//
// Scanner ini:
//  1. Identify LLM provider call sites: http.NewRequest ke URL yang
//     contain "api.anthropic.com", "openrouter.ai", "api.openai.com",
//     "generativelanguage.googleapis.com", "api.deepseek.com".
//  2. Untuk setiap call site, trace upward dalam SAMA function body
//     apakah ada BudgetGuard gate: panggilan ke
//     `finance.Shared().CheckBudget(...)` atau `guard.CheckBudget(...)`
//     atau import `internal/finance` dengan pola Check panggilan.
//  3. Flag call site yang TIDAK punya gate sebelum HTTP request.
//
// Output: state/scanner-reports/budget_gate_coverage.md — per file:line list endpoint
// provider call dan whether gated atau tidak.
//
// Contoh positive pattern (safe, gated):
//
//	guard := finance.Shared()
//	if err := guard.CheckBudget(estimateUSD); err != nil { ... }
//	req, _ := http.NewRequestWithContext(ctx, "POST", openrouterURL, body)
//	resp, _ := client.Do(req)
//
// Contoh negative pattern (leak, ungated):
//
//	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", body)
//	resp, _ := client.Do(req)  // ← no budget gate, bleed on runaway loop
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

type GateFinding struct {
	Level    string
	File     string
	Line     int
	Endpoint string
	Message  string
}

var gateFindings []GateFinding

// LLM provider endpoints (substring match on URL literals).
var providerHosts = []string{
	"api.anthropic.com",
	"api.openai.com",
	"openrouter.ai",
	"generativelanguage.googleapis.com",
	"api.deepseek.com",
	"api.x.ai",
	"api.mistral.ai",
	"api.cohere.ai",
}

// BudgetGuard method names that count as a gate.
var gateMethodNames = map[string]bool{
	"CheckBudget":     true,
	"CheckBudgetFor":  true,
	"Check":           true, // ratelimit.Check
	"Reserve":         true,
	"CheckAndReserve": true,
}

func main() {
	fmt.Println("💰 [BUDGET-GATE v1 rc113] Scanning LLM provider call sites untuk BudgetGuard coverage.")
	fmt.Println("   Protects crypto wallet 0xd129...7eb1 dari runaway loop bleed.")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") ||
				strings.Contains(path, "state/") || strings.Contains(path, "scanner/") ||
				strings.Contains(path, "_sgvp/") || strings.Contains(path, "_sgvp\\") || path == "_sgvp" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanBudgetGate(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[💸] Selesai! Findings: %d provider call tanpa gate.\n", len(gateFindings))
	for _, f := range gateFindings {
		fmt.Printf("🚨 [%s] %s (%s)\n   %s:%d  %s\n", f.Level, "UNGATED", f.Endpoint, f.File, f.Line, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "budget_gate_coverage.md")
	writeGateReport(outFile)
	fmt.Println("\n📁 Report:", outFile)
}

func scanBudgetGate(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// For each FuncDecl: collect provider call positions + budget gate positions.
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		var providerCalls []providerHit
		hasGate := false

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Gate detection: guard.CheckBudget(...) or .Check(...) or Reserve(...)
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if gateMethodNames[sel.Sel.Name] {
					hasGate = true
				}
			}

			// Provider call detection: URL literal containing known host.
			if hit, ok := providerCallHit(call); ok {
				hit.pos = call.Pos()
				providerCalls = append(providerCalls, hit)
			}
			return true
		})

		if len(providerCalls) > 0 && !hasGate {
			fname := "(global)"
			if fn.Name != nil {
				fname = fn.Name.Name
			}
			for _, hit := range providerCalls {
				recordGate(filePath, fset.Position(hit.pos).Line, hit.endpoint,
					fmt.Sprintf("Provider LLM call di fungsi %s tanpa BudgetGuard gate. Runaway loop = wallet bleed. Tambah `if err := finance.Shared().CheckBudget(est); err != nil { return }` sebelum Do().", fname))
			}
		}
	}
}

type providerHit struct {
	endpoint string
	pos      token.Pos
}

// Non-metered URL paths — balance/auth/health/model-list endpoints yang
// tidak charge per-call. Scanner skip bila URL contain salah satu pattern.
var nonMeteredPaths = []string{
	"/auth/key", // OpenRouter balance query
	"/auth/",    // Auth endpoints (credential check)
	"/models",   // Model list (catalog)
	"/v1/models",
	"/health",
	"/ping",
	"/status",
	"/credits",
	"/usage", // Usage query (reporting, not generation)
	"/billing",
}

func isNonMeteredPath(url string) bool {
	for _, p := range nonMeteredPaths {
		if strings.Contains(url, p) {
			return true
		}
	}
	return false
}

func providerCallHit(call *ast.CallExpr) (providerHit, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return providerHit{}, false
	}
	// http.NewRequest / http.NewRequestWithContext / client.Post / client.Get
	method := sel.Sel.Name
	if method != "NewRequest" && method != "NewRequestWithContext" &&
		method != "Post" && method != "Get" && method != "Do" {
		return providerHit{}, false
	}
	// Find URL + HTTP method argument.
	urlIdx := 0
	httpMethodIdx := -1
	switch method {
	case "NewRequest":
		urlIdx = 1
		httpMethodIdx = 0
	case "NewRequestWithContext":
		urlIdx = 2
		httpMethodIdx = 1
	case "Post":
		urlIdx = 0
	case "Get":
		urlIdx = 0
	}
	if len(call.Args) <= urlIdx {
		return providerHit{}, false
	}

	// Skip HEAD method (non-metered probe).
	if httpMethodIdx >= 0 && httpMethodIdx < len(call.Args) {
		if mlit, ok := call.Args[httpMethodIdx].(*ast.BasicLit); ok && mlit.Kind == token.STRING {
			m := strings.ToUpper(strings.Trim(mlit.Value, "`\""))
			if m == "HEAD" || m == "OPTIONS" {
				return providerHit{}, false
			}
		}
	}

	arg := call.Args[urlIdx]
	lit, isLit := arg.(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return providerHit{}, false
	}
	raw := strings.Trim(lit.Value, "`\"")

	// Skip non-metered endpoints (balance/auth/models/health).
	if isNonMeteredPath(raw) {
		return providerHit{}, false
	}

	for _, host := range providerHosts {
		if strings.Contains(raw, host) {
			return providerHit{endpoint: host}, true
		}
	}
	return providerHit{}, false
}

func recordGate(file string, line int, endpoint, msg string) {
	gateFindings = append(gateFindings, GateFinding{
		Level:    "CRITICAL",
		File:     file,
		Line:     line,
		Endpoint: endpoint,
		Message:  msg,
	})
}

func writeGateReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 💰 Budget Gate Coverage Report\n\n")
	out.WriteString("> **Scanner baru rc113** — memastikan semua LLM provider call di-gate oleh `finance.BudgetGuard` sebelum fire HTTP request. Proteksi crypto wallet dari runaway loop bleed.\n\n")
	out.WriteString("## Metodologi\n\n")
	out.WriteString("- Scan file `.go` (exclude test, scanner/, state/).\n")
	out.WriteString("- Identify call ke provider host (OpenRouter, Anthropic, OpenAI, Gemini, DeepSeek, XAI, Mistral, Cohere) via URL literal.\n")
	out.WriteString("- Untuk setiap FuncDecl yang berisi provider call, cek apakah function body juga memanggil `CheckBudget`/`Reserve`/`Check`.\n")
	out.WriteString("- Flag kalau ada provider call TAPI TIDAK ada gate call.\n\n")

	if len(gateFindings) == 0 {
		out.WriteString("## ✅ Coverage 100%\n\n")
		out.WriteString("Tidak ada LLM provider call site yang bypass BudgetGuard. Wallet aman dari runaway bleed.\n")
		return
	}

	out.WriteString(fmt.Sprintf("## 🚨 Ditemukan %d Provider Call Ungated\n\n", len(gateFindings)))
	for _, f := range gateFindings {
		out.WriteString(fmt.Sprintf("---\n### 🚨 [%s] UNGATED → `%s`\n", f.Level, f.Endpoint))
		out.WriteString(fmt.Sprintf("**Lokasi**: `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("**Fix**: %s\n\n", f.Message))
	}
}

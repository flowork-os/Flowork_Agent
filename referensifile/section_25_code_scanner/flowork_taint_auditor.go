//go:build ignore

// Package taint — semantic-aware taint auditor untuk path traversal, SSRF,
// deserialization RCE, dan weak crypto RNG.
//
// Rewrite rc113 (Opus-2 2026-04-19): sebelumnya pattern-match menghasilkan
// 100+ FALSE POSITIVE karena:
//
//	(a) "Path Traversal" flag SEMUA `filepath.Join()` regardless of args.
//	    Realitas: mayoritas join pakai konstanta internal (os.UserHomeDir,
//	    filepath.Join(home, ".flowork", ...)), bukan user input.
//	(b) "SSRF" flag SEMUA http.Get/Post dengan non-literal URL. Realitas:
//	    banyak URL dinamis dari env var (internal loopback port) bukan
//	    dari user input.
//
// Rewrite ini tambahin taint-source tracking per-file:
//  1. isTaintSource(expr) — detect HTTP handler param (r.URL.Query,
//     r.Form, mux.Vars), bridge message body, websocket recv.
//  2. Scan: assignments yang propagate taint. Variabel yang pernah
//     di-assign dari taint source jadi "tainted".
//  3. filepath.Join hanya di-flag kalau ADA arg yang reference tainted var.
//  4. http.Get/Post hanya di-flag kalau URL reference tainted var DAN
//     file tidak import `safeclient` (defense allowlist).
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

type GodModeVuln struct {
	Category string
	File     string
	Line     int
	Message  string
}

var godModeFindings []GodModeVuln

func main() {
	fmt.Println("☠️  [TAINT v2 rc113] Semantic taint tracking: path traversal + SSRF + weak crypto.")
	fmt.Println("Filter FP: filepath.Join dengan konstanta internal di-skip, SSRF di-skip bila safeclient imported.")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") ||
				strings.Contains(path, "state/") {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanGodMode(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[⚠️] Taint audit selesai. Findings: %d (semantic filter aktif).\n", len(godModeFindings))
	for _, f := range godModeFindings {
		fmt.Printf("🔥 [%s] %s:%d\n   -> %s\n", f.Category, f.File, f.Line, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "elite_taint_audit.md")
	writeEliteReport(outFile)
	fmt.Println("\n📁 Report:", outFile)
}

// fileAnalysis holds per-file taint tracking state.
type fileAnalysis struct {
	importsSafeClient bool
	taintedVars       map[string]bool        // variable names that transitively touch user input
	sanitizedInFn     map[*ast.FuncDecl]bool // FuncDecl yang punya sanitize guard pattern
}

// funcBodyHasSanitizeGuard returns true kalau body berisi salah satu pattern:
//   - strings.ContainsAny(x, "/\\")
//   - strings.Contains(x, "..")
//   - strings.HasPrefix(x, "..") / HasSuffix(x, "..")
//   - filepath.Base(x) — reduces to basename, strips traversal
//   - Map-whitelist gate: `if !allowedXxx[x]` return
//   - os.ReadDir loop (e.Name() safe basenames)
//
// Kalau ADA, asumsi function sudah sanitize input sebelum sink (filepath.Join).
// Heuristic — bukan proof, tapi acceptable FP reduction.
func funcBodyHasSanitizeGuard(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		// Pattern: `if !someMap[key]` / `if _, ok := someMap[key]; !ok` — whitelist gate.
		if ifs, ok := n.(*ast.IfStmt); ok {
			if hasMapWhitelistGate(ifs) {
				found = true
				return false
			}
		}

		// Pattern: os.ReadDir return iterated — e.Name() is always a safe basename.
		if rng, ok := n.(*ast.RangeStmt); ok {
			if rangeSourceIsReadDir(rng) {
				found = true
				return false
			}
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Pattern: call ke function yang name mirrors sanitize intent —
		// `sanitizeXxx(y)`, `validateXxx(y)`, `normalizeXxx(y)`, `cleanXxx(y)`.
		// Project custom sanitize helper umumnya ikut convention ini.
		if id, ok := call.Fun.(*ast.Ident); ok {
			low := strings.ToLower(id.Name)
			if strings.HasPrefix(low, "sanitize") || strings.HasPrefix(low, "validate") ||
				strings.HasPrefix(low, "normalize") || strings.HasPrefix(low, "cleanpath") ||
				strings.HasPrefix(low, "safepath") {
				found = true
				return false
			}
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch id.Name {
		case "strings":
			if sel.Sel.Name == "ContainsAny" || sel.Sel.Name == "Contains" ||
				sel.Sel.Name == "HasPrefix" || sel.Sel.Name == "HasSuffix" {
				// Check second arg for traversal marker.
				if len(call.Args) >= 2 {
					if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						raw := lit.Value
						if strings.Contains(raw, "..") || strings.Contains(raw, "/") || strings.Contains(raw, "\\") {
							found = true
							return false
						}
					}
				}
			}
		case "filepath":
			if sel.Sel.Name == "Base" || sel.Sel.Name == "Clean" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// hasMapWhitelistGate detect pattern `if !m[x] { ... return }` or
// `if _, ok := m[x]; !ok { ... return }` — common whitelist sanitize.
func hasMapWhitelistGate(ifs *ast.IfStmt) bool {
	// Case 1: if !m[x]
	if unary, ok := ifs.Cond.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		if _, ok := unary.X.(*ast.IndexExpr); ok {
			return true
		}
	}
	// Case 2: if _, ok := m[x]; !ok
	if ifs.Init != nil {
		if assign, ok := ifs.Init.(*ast.AssignStmt); ok {
			for _, rhs := range assign.Rhs {
				if _, ok := rhs.(*ast.IndexExpr); ok {
					return true
				}
			}
		}
	}
	return false
}

// rangeSourceIsReadDir detect `for _, e := range entries` dimana entries
// assigned dari `os.ReadDir(...)` — e.Name() always safe basename.
func rangeSourceIsReadDir(rng *ast.RangeStmt) bool {
	// Heuristic: range variable name `e`/`entry`/`fi` yang typical dari ReadDir.
	if id, ok := rng.X.(*ast.Ident); ok {
		low := strings.ToLower(id.Name)
		if strings.Contains(low, "entr") || strings.Contains(low, "dirent") || strings.Contains(low, "fileinfo") {
			return true
		}
	}
	return false
}

// posIsInSanitizedFunc returns true kalau pos ada di dalam FuncDecl yang
// sudah mark sanitized.
func (fa *fileAnalysis) posIsInSanitizedFunc(pos token.Pos, node *ast.File) bool {
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if !fa.sanitizedInFn[fn] {
			continue
		}
		if pos >= fn.Body.Lbrace && pos <= fn.Body.Rbrace {
			return true
		}
	}
	return false
}

func scanGodMode(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	fa := &fileAnalysis{
		taintedVars:   make(map[string]bool),
		sanitizedInFn: make(map[*ast.FuncDecl]bool),
	}

	// Pass 0: detect if this file imports `safeclient` (defense allowlist).
	for _, imp := range node.Imports {
		if strings.Contains(imp.Path.Value, "safeclient") {
			fa.importsSafeClient = true
		}
	}

	// Pass 0b: rc122 scanner improvement — per-FuncDecl detect sanitize
	// pattern. Function body yang contain salah satu guard pattern:
	//   - strings.ContainsAny(x, "/\\") / ContainsAny(..., "/\\")
	//   - strings.Contains(x, "..")
	//   - strings.HasPrefix(x, "..")
	//   - filepath.Clean(x) return used for HasPrefix/HasSuffix check
	// ...dianggap "sanitized scope" — flag filepath.Join di function ini
	// di-downgrade ke informational (skip, presume guarded).
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if funcBodyHasSanitizeGuard(fn.Body) {
			fa.sanitizedInFn[fn] = true
		}
	}

	// Pass 1: collect tainted variable names from HTTP handler params +
	// assignments that propagate taint (e.g. `name := r.FormValue("x")`).
	ast.Inspect(node, func(n ast.Node) bool {
		// Function params of type http.Request → *r* is tainted.
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Type.Params != nil {
			for _, p := range fn.Type.Params.List {
				if isHTTPRequestType(p.Type) {
					for _, ident := range p.Names {
						fa.taintedVars[ident.Name] = true
					}
				}
			}
		}
		if fn, ok := n.(*ast.FuncLit); ok && fn.Type.Params != nil {
			for _, p := range fn.Type.Params.List {
				if isHTTPRequestType(p.Type) {
					for _, ident := range p.Names {
						fa.taintedVars[ident.Name] = true
					}
				}
			}
		}

		// Assignments: LHS = taintedExpr propagates.
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assign.Lhs {
				if i >= len(assign.Rhs) {
					continue
				}
				if fa.exprReferencesTaint(assign.Rhs[i]) {
					if id, ok := lhs.(*ast.Ident); ok {
						fa.taintedVars[id.Name] = true
					}
				}
			}
		}
		return true
	})

	// Pass 2: flag suspicious sinks.
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fun, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// 1. Weak PRNG — tetap flag (selalu valid security concern).
		if fun.Sel.Name == "Intn" || fun.Sel.Name == "Float64" || fun.Sel.Name == "Seed" {
			if id, ok := fun.X.(*ast.Ident); ok && id.Name == "rand" {
				// Suppress if file is explicitly non-security random use.
				low := strings.ToLower(filePath)
				if strings.Contains(low, "jitter") || strings.Contains(low, "backoff") ||
					strings.Contains(low, "/music/") || strings.Contains(low, "/dreamstate/") ||
					strings.Contains(low, "/ideabank") || strings.Contains(low, "metadata.go") {
					// Music track picker, dreamstate sampling — not security-sensitive.
					return true
				}
				msg := "math/rand (bukan crypto/rand). Jangan dipakai untuk ID sesi / token / money. Pakai crypto/rand untuk security-sensitive random."
				recordVuln(fset, call.Pos(), filePath, "Weak Cryptography", msg)
			}
		}

		// 2. Unsafe Deserialization.
		if fun.Sel.Name == "NewDecoder" {
			if id, ok := fun.X.(*ast.Ident); ok && id.Name == "gob" {
				msg := "gob.NewDecoder dari sumber tak tepercaya = RCE via type juggling."
				recordVuln(fset, call.Pos(), filePath, "Unsafe Deserialization", msg)
			}
		}

		// 3. SSRF — hanya flag bila URL reference tainted var AND file
		// TIDAK import safeclient (yang sudah blocklist CIDR private).
		if fun.Sel.Name == "Get" || fun.Sel.Name == "Post" ||
			fun.Sel.Name == "NewRequest" || fun.Sel.Name == "NewRequestWithContext" {
			if id, ok := fun.X.(*ast.Ident); ok && id.Name == "http" {
				if fa.importsSafeClient {
					return true // defense allowlist
				}
				urlIdx := 0
				if fun.Sel.Name == "NewRequest" {
					urlIdx = 1
				} else if fun.Sel.Name == "NewRequestWithContext" {
					urlIdx = 2
				}
				if len(call.Args) <= urlIdx {
					return true
				}
				arg := call.Args[urlIdx]
				if _, isLit := arg.(*ast.BasicLit); isLit {
					return true // hardcoded URL, safe
				}
				if !fa.exprReferencesTaint(arg) {
					return true // dynamic but not user-tainted
				}
				msg := "HTTP request dari URL user-tainted tanpa safeclient. Risiko SSRF ke metadata/LAN/private CIDR."
				recordVuln(fset, call.Pos(), filePath, "SSRF Vulnerability", msg)
			}
		}

		// 4. Path Traversal — filepath.Join dengan taint source.
		if fun.Sel.Name == "Join" {
			if id, ok := fun.X.(*ast.Ident); ok && id.Name == "filepath" {
				// Skip if enclosing function body has sanitize guard
				// (strings.ContainsAny "/\\" / Contains ".." / HasPrefix "..").
				if fa.posIsInSanitizedFunc(call.Pos(), node) {
					return true
				}
				// Skip if all args are literal / constant / filepath.Base(...)
				if fa.hasTaintedArg(call.Args) {
					msg := "filepath.Join dengan arg dari user input. Risk: `../../etc/passwd` traversal. Sanitize via filepath.Base() + Clean() + validate prefix."
					recordVuln(fset, call.Pos(), filePath, "Path Traversal Risk", msg)
				}
			}
		}

		return true
	})
}

// exprReferencesTaint returns true if the expression transitively touches
// a tainted variable or a known taint source.
func (fa *fileAnalysis) exprReferencesTaint(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.Ident:
			if fa.taintedVars[x.Name] {
				found = true
			}
		case *ast.SelectorExpr:
			// Direct taint sources: r.URL, r.Form, r.Body, r.Header
			if id, ok := x.X.(*ast.Ident); ok {
				if fa.taintedVars[id.Name] {
					found = true
				}
			}
		case *ast.CallExpr:
			// Calls to known sink-free helpers: filepath.Base skip.
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					if id.Name == "filepath" && sel.Sel.Name == "Base" {
						return false // sanitized
					}
					if id.Name == "filepath" && sel.Sel.Name == "Clean" {
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

func (fa *fileAnalysis) hasTaintedArg(args []ast.Expr) bool {
	for _, a := range args {
		if fa.exprReferencesTaint(a) {
			return true
		}
	}
	return false
}

func isHTTPRequestType(t ast.Expr) bool {
	// *http.Request
	if star, ok := t.(*ast.StarExpr); ok {
		if sel, ok := star.X.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				return id.Name == "http" && sel.Sel.Name == "Request"
			}
		}
	}
	return false
}

func recordVuln(fset *token.FileSet, pos token.Pos, file, category, msg string) {
	// Skip scanner self.
	p := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(p, "scanner/") || strings.Contains(p, "/scanner/") {
		return
	}
	godModeFindings = append(godModeFindings, GodModeVuln{
		Category: category,
		File:     file,
		Line:     fset.Position(pos).Line,
		Message:  msg,
	})
}

func writeEliteReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# ☠️ ELITE Taint & Crypto Audit Report\n\n")
	out.WriteString("> **Scanner v2 (rc113)** — semantic taint tracking aktif. Hanya flag bila arg reference tainted variable (HTTP handler param, bridge body, dll). File yang import `safeclient` di-allowlist untuk SSRF.\n\n")

	if len(godModeFindings) == 0 {
		out.WriteString("*Tidak ada temuan. Scanner v2 filter FP yang sebelumnya 100+ noise dari filepath.Join/http.Get.*\n")
		return
	}

	out.WriteString("## Daftar Target Kritis\n\n")
	for _, f := range godModeFindings {
		out.WriteString(fmt.Sprintf("### 🎯 **[%s]** `%s:%d`\n", f.Category, f.File, f.Line))
		out.WriteString(fmt.Sprintf("> %s\n\n", f.Message))
	}
}

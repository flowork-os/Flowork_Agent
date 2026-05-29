//go:build ignore

// Package fortress — narrow-focus scanner untuk state-critical atomic write,
// unbounded ReadAll, eternal loop tanpa exit. Bukan flag bulk pada SEMUA
// os.WriteFile / json.Unmarshal (FP-heavy di scanner v1).
//
// Rewrite rc113 (Opus-2 2026-04-19):
//
//	(a) "Memory Amnesia" os.WriteFile — hanya flag bila path reference
//	    kategori critical: state/, .flowork/, memory/, registry.json,
//	    bridge.json. Cache/log/temporary file di-skip.
//	(b) "RAM Exhaustion" io.ReadAll — hanya flag bila panggilan ADA di
//	    function yang juga panggil http.Do (membaca HTTP body tanpa
//	    LimitReader = real OOM risk); kalau bukan HTTP, tidak flag.
//	(c) "Data Poisoning" json.Unmarshal — DIHAPUS. FP 95% dari response
//	    unmarshal dari official API (atproto, Mastodon, Telegram). Tim
//	    sudah tahu kalau API berubah schema = panic; itu maintenance risk
//	    bukan security bug.
//	(d) "Eternal Loop" for{} — tetap, tapi skip kalau body berisi call ke
//	    function yang namanya mengandung `WaitFor...`, `Until...`, atau
//	    `Listen...` (typical long-running daemon primitive).
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

type FortressVuln struct {
	Level   string
	Type    string
	File    string
	Line    int
	Message string
}

var fortressFindings []FortressVuln

// Substring markers in file path literals that mark a write as
// state-critical (agen memory, crypto wallet snapshot, etc).
var criticalPathMarkers = []string{
	".flowork/", "state/", "memory/", "bridge.json", "registry.json",
	"wallet", "facts.jsonl", "mood.json", "owner.hash", "claims/",
}

func main() {
	fmt.Println("🏰 [FORTRESS v2 rc113] Narrow: state-critical WriteFile + HTTP unbounded ReadAll + eternal loop.")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") ||
				strings.Contains(path, "state/") ||
				strings.Contains(path, "_sgvp") /* SGVP test fixtures intentional */ {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanFortress(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Gagal:", err)
		return
	}

	fmt.Printf("\n[🛡️] Selesai! Findings: %d.\n", len(fortressFindings))
	for _, f := range fortressFindings {
		fmt.Printf("🧱 [%s] %s | %s:%d\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "the_fortress_audit.md")
	writeFortressReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanFortress(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// Per-function aggregation for ReadAll detection (need to know if
	// same function has http.Do/Post/Get).
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		hasHTTPCall := false
		hasLimitReader := false

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						switch id.Name {
						case "http":
							if sel.Sel.Name == "Get" || sel.Sel.Name == "Post" ||
								sel.Sel.Name == "NewRequest" || sel.Sel.Name == "NewRequestWithContext" {
								hasHTTPCall = true
							}
						case "io":
							if sel.Sel.Name == "LimitReader" {
								hasLimitReader = true
							}
						}
					}
					if sel.Sel.Name == "Do" {
						hasHTTPCall = true
					}
				}
			}
			return true
		})

		// Scan body for suspicious calls.
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fun, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// 1. State-critical WriteFile (non-atomic).
			if fun.Sel.Name == "WriteFile" && (isIdent(fun.X, "os") || isIdent(fun.X, "ioutil")) {
				if len(call.Args) == 0 {
					return true
				}
				// Walk the path arg for critical path markers.
				if pathLooksCritical(call.Args[0]) {
					msg := "WriteFile ke path state-critical tanpa atomic pattern (.tmp + Rename). OS crash mid-write = state korup. Pakai helper atomic write."
					recordFortress(fset, call.Pos(), filePath, "HIGH", "Non-Atomic State Write", msg)
				}
			}

			// 2. Unbounded ReadAll di HTTP path tanpa LimitReader.
			if fun.Sel.Name == "ReadAll" && (isIdent(fun.X, "io") || isIdent(fun.X, "ioutil")) {
				if hasHTTPCall && !hasLimitReader {
					msg := "io.ReadAll dari HTTP response tanpa io.LimitReader di scope function yang sama. Tarpit server bisa kirim gigabyte = OOM. Wrap dengan `io.LimitReader(resp.Body, maxBytes)`."
					recordFortress(fset, call.Pos(), filePath, "HIGH", "Unbounded HTTP ReadAll", msg)
				}
			}

			return true
		})
	}

	// Eternal loop check (file-wide).
	ast.Inspect(node, func(n ast.Node) bool {
		forStmt, ok := n.(*ast.ForStmt)
		if !ok {
			return true
		}
		if forStmt.Cond != nil {
			return true
		}
		// Bounded? Check for break/return/select/known daemon primitives.
		hasExit := false
		ast.Inspect(forStmt.Body, func(bn ast.Node) bool {
			if hasExit {
				return false
			}
			switch x := bn.(type) {
			case *ast.BranchStmt, *ast.ReturnStmt, *ast.SelectStmt:
				hasExit = true
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					n := sel.Sel.Name
					if strings.HasPrefix(n, "WaitFor") || strings.HasPrefix(n, "Listen") ||
						strings.HasPrefix(n, "Accept") || n == "Wait" || n == "Recv" {
						hasExit = true
					}
				}
			}
			return true
		})
		if !hasExit {
			msg := "Infinite loop `for {}` tanpa break/return/select/daemon primitive. Agent bisa terjebak selamanya."
			recordFortress(fset, forStmt.Pos(), filePath, "CRITICAL", "Eternal Loop Prison", msg)
		}
		return true
	})
}

// pathLooksCritical returns true if the AST expr for a file path reference
// appears to target state/ / .flowork/ / memory/ / bridge.json.
func pathLooksCritical(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			raw := strings.ToLower(strings.Trim(lit.Value, "`\""))
			for _, m := range criticalPathMarkers {
				if strings.Contains(raw, m) {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func isIdent(x ast.Expr, name string) bool {
	id, ok := x.(*ast.Ident)
	return ok && id.Name == name
}

func recordFortress(fset *token.FileSet, pos token.Pos, file, level, fType, msg string) {
	p := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(p, "scanner/") || strings.Contains(p, "/scanner/") {
		return
	}
	if strings.Contains(file, "_test") {
		return
	}
	fortressFindings = append(fortressFindings, FortressVuln{
		Level:   level,
		Type:    fType,
		File:    file,
		Line:    fset.Position(pos).Line,
		Message: msg,
	})
}

func writeFortressReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 🏰 Fortress Audit — State Write + HTTP ReadAll + Eternal Loop\n\n")
	out.WriteString("> **Scanner v2 (rc113)** — fokus sempit: WriteFile ke state-critical path, HTTP ReadAll tanpa LimitReader di function yang sama, infinite loop tanpa exit. \"Data Poisoning\" bulk-flag dihapus.\n\n")

	if len(fortressFindings) == 0 {
		out.WriteString("*Tidak ada tiang rapuh yang teridentifikasi.*\n")
		return
	}

	for _, f := range fortressFindings {
		out.WriteString(fmt.Sprintf("---\n### 🧱 [%s] %s\n", f.Level, f.Type))
		out.WriteString(fmt.Sprintf("**Zona Rapuh**: `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("**Perlindungan Mutlak**: %s\n\n", f.Message))
	}
}

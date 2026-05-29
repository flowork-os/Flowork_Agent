//go:build ignore

// Package trillion — network exposure auditor. Fokus sempit + signal tinggi:
// detect HTTP/TCP listener yang bind ke 0.0.0.0 (via `:port` shorthand atau
// eksplisit "0.0.0.0:port") — ini real risk LAN exposure.
//
// Rewrite rc113 (Opus-2 2026-04-19): sebelumnya scanner ini juga flag SEMUA
// `_ = foo()` sebagai "Swallowed Return Pattern" — FP total karena:
//
//	(a) Banyak `_ = foo()` legitimate (fmt.Fprintln flush, log.Write, etc).
//	(b) Tanpa type info, tidak bisa distinguish error-return vs value-return.
//	(c) Scanner flag ratusan baris tanpa value ke tim.
//
// "Swallowed return" dihapus. Network bind scanner tetap.
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

type TrillionVuln struct {
	Level   string
	Type    string
	File    string
	Line    int
	Bind    string
	Message string
}

var trillionFindings []TrillionVuln

func main() {
	fmt.Println("💎 [TRILLION v2 rc113] Network bind exposure scanner — skip swallowed-return FP.")

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
			scanTrillion(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[💎] Selesai! Findings: %d network bind exposures.\n", len(trillionFindings))
	for _, f := range trillionFindings {
		fmt.Printf("💵 [%s] %s | %s:%d (bind=%q)\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Bind, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "trillion_dollar_audit.md")
	writeTrillionReport(outFile)
	fmt.Println("\n🏦 Report:", outFile)
}

func scanTrillion(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		fun, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// http.ListenAndServe / http.ListenAndServeTLS
		if (fun.Sel.Name == "ListenAndServe" || fun.Sel.Name == "ListenAndServeTLS") && isFromHTTP(fun.X) {
			if len(call.Args) == 0 {
				return true
			}
			if bind, ok := asStringLit(call.Args[0]); ok && isPublicBind(bind) {
				msg := "HTTP server bind ke " + describeBind(bind) + ". Terekspos ke seluruh LAN (WiFi kos/kampus) — combine dengan CORS permissive = RCE via CSRF. Fix: bind `127.0.0.1:<port>` atau tambah `--lan` flag opt-in."
				recordTrillion(fset, call.Pos(), filePath, "CRITICAL", "Public Network Exposure", bind, msg)
			}
		}

		// (&http.Server{Addr:...}).ListenAndServe() — check the struct composite literal.
		if fun.Sel.Name == "ListenAndServe" {
			if star, ok := fun.X.(*ast.StarExpr); ok {
				if cl, ok := star.X.(*ast.CompositeLit); ok {
					if bind, ok := extractHTTPServerAddr(cl); ok && isPublicBind(bind) {
						msg := "http.Server.Addr = " + describeBind(bind) + " → bind public. Lihat BUG-A01 pattern di flowork-gui rc111."
						recordTrillion(fset, call.Pos(), filePath, "CRITICAL", "Public Network Exposure", bind, msg)
					}
				}
			}
		}

		// net.Listen("tcp", ":port") — raw TCP socket.
		if fun.Sel.Name == "Listen" && len(call.Args) >= 2 {
			if netIdent, ok := fun.X.(*ast.Ident); ok && netIdent.Name == "net" {
				if bind, ok := asStringLit(call.Args[1]); ok && isPublicBind(bind) {
					msg := "TCP Listen bind " + describeBind(bind) + " → public socket. Hati-hati kalau ini bukan intentional mesh/LAN service. Contoh intentional OK: cmd/flowork-mesh (--lan opt-in)."
					recordTrillion(fset, call.Pos(), filePath, "HIGH", "Socket Public Exposure", bind, msg)
				}
			}
		}

		return true
	})
}

// asStringLit extracts string literal value from AST expr.
func asStringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, "`\""), true
}

// extractHTTPServerAddr looks for `Addr: "..."` in a composite literal.
func extractHTTPServerAddr(cl *ast.CompositeLit) (string, bool) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Addr" {
			continue
		}
		// Value may be string literal or fmt.Sprintf(...) — only literal is analyzable.
		if bind, ok := asStringLit(kv.Value); ok {
			return bind, true
		}
	}
	return "", false
}

// isPublicBind flags binding string that exposes to 0.0.0.0.
func isPublicBind(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// ":8080" → INADDR_ANY (0.0.0.0)
	if strings.HasPrefix(s, ":") {
		return true
	}
	if strings.HasPrefix(s, "0.0.0.0") {
		return true
	}
	if strings.HasPrefix(s, "[::]") {
		return true
	}
	return false
}

func describeBind(s string) string {
	if strings.HasPrefix(s, ":") {
		return fmt.Sprintf("%q (shorthand = 0.0.0.0)", s)
	}
	return fmt.Sprintf("%q", s)
}

func isFromHTTP(x ast.Expr) bool {
	id, ok := x.(*ast.Ident)
	return ok && id.Name == "http"
}

func recordTrillion(fset *token.FileSet, pos token.Pos, file, level, fType, bind, msg string) {
	p := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(p, "scanner/") || strings.Contains(p, "/scanner/") {
		return
	}
	if strings.Contains(file, "_test") {
		return
	}
	trillionFindings = append(trillionFindings, TrillionVuln{
		Level:   level,
		Type:    fType,
		File:    file,
		Line:    fset.Position(pos).Line,
		Bind:    bind,
		Message: msg,
	})
}

func writeTrillionReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 💎 Network Bind Exposure Audit\n\n")
	out.WriteString("> **Scanner v2 (rc113)** — fokus sempit: deteksi `ListenAndServe(\":port\")` atau `net.Listen(\"tcp\", \":port\")` yang bind ke INADDR_ANY. Swallowed-return FP dihapus.\n\n")

	if len(trillionFindings) == 0 {
		out.WriteString("*Tidak ada public bind ditemukan (atau semua memang intentional LAN service).*\n")
		return
	}

	for _, f := range trillionFindings {
		out.WriteString(fmt.Sprintf("---\n### 💵 [%s] %s\n", f.Level, f.Type))
		out.WriteString(fmt.Sprintf("**File**: `%s:%d`  **Bind**: `%s`\n", f.File, f.Line, f.Bind))
		out.WriteString(fmt.Sprintf("**Laporan**: %s\n\n", f.Message))
	}
}

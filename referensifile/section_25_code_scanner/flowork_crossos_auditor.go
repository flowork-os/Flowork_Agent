//go:build ignore

// Package crossos — scanner baru rc113 yang deteksi hardcoded path
// literal yang tidak cross-OS (`/tmp/...`, `C:\...`, `/home/...`, etc).
//
// Per GOL_FLOWORK.MD line 164: "JANGAN hardcode `/` atau `\`". Flowork
// target: Windows, Linux, Raspberry Pi, macOS. Hardcoded POSIX path akan
// pecah di Windows; hardcoded `C:\` pecah di Linux/macOS/RPi.
//
// Scanner ini:
//  1. Walk AST, cari string literal yang match pattern non-portable path:
//     - `/tmp/`, `/var/`, `/etc/`, `/home/`, `/usr/`, `/opt/` (POSIX root)
//     - `C:\`, `D:\`, `E:\` (Windows drive letter)
//     - `\\server\share` (UNC)
//     - `\Users\` or `\ProgramData\` (Windows user hardcode)
//  2. Flag site pakai severity HIGH — break build di OS lain.
//  3. Allowlist: test file, scanner itu sendiri, Windows-specific file
//     `*_windows.go` (build tag, memang platform-specific), comment.
//
// Fix: pakai `os.UserHomeDir()`, `os.TempDir()`, `filepath.Join(...)` dengan
// konstanta relatif, atau `runtime.GOOS` branch.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type PathFinding struct {
	Level   string
	File    string
	Line    int
	Literal string
	Message string
}

var pathFindings []PathFinding

var (
	posixRootPattern = regexp.MustCompile(`^/(tmp|var|etc|home|usr|opt|root)(/|$)`)
	winDrivePattern  = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	uncPattern       = regexp.MustCompile(`^\\\\[^\\]+\\`)
	winUserPattern   = regexp.MustCompile(`\\(Users|ProgramData|Windows)\\`)
)

// Skip file patterns that are legitimately OS-specific.
func isOSSpecificFile(path string) bool {
	base := filepath.Base(path)
	suffixes := []string{"_windows.go", "_linux.go", "_darwin.go", "_unix.go"}
	for _, s := range suffixes {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return false
}

func main() {
	fmt.Println("🌍 [CROSS-OS v1 rc113] Scanning hardcoded path literal yang bukan cross-OS.")
	fmt.Println("   Target: Windows + Linux + RaspberryPi + macOS. Per GOL_FLOWORK.MD aturan lintas-OS.")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			if strings.Contains(path, ".git") || strings.Contains(path, "tools_temp") ||
				strings.Contains(path, "state/") || strings.Contains(path, "scanner/") ||
				strings.Contains(path, "_sgvp") /* SGVP test fixtures intentional */ {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			if isOSSpecificFile(path) {
				return nil
			}
			scanCrossOS(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[🌍] Selesai! Findings: %d hardcoded non-portable path.\n", len(pathFindings))
	for _, f := range pathFindings {
		fmt.Printf("🚫 [%s] %s:%d  %q\n   -> %s\n", f.Level, f.File, f.Line, f.Literal, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "cross_os_path_audit.md")
	writeCrossOSReport(outFile)
	fmt.Println("\n📁 Report:", outFile)
}

func scanCrossOS(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// rc121 scanner improvement: collect positions of string literals yang
	// dipakai sebagai arg ke `strings.Contains/HasPrefix/HasSuffix/Index/
	// HasSubstr` — itu BLOCKLIST PATTERN, bukan path access. Skip dari flag.
	// Juga skip literals inside composite literals ([]string{...}, map{...})
	// yang tipenya []string or []byte — typically blocklist/allowlist data.
	blocklistLits := map[token.Pos]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "strings" {
			return true
		}
		switch sel.Sel.Name {
		case "Contains", "HasPrefix", "HasSuffix", "Index", "Count", "ContainsAny", "Split", "Replace", "ReplaceAll":
			for _, arg := range call.Args {
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					blocklistLits[lit.Pos()] = true
				}
			}
		}
		return true
	})

	// String literals inside composite literal ([]string{...}) — these are
	// typically blocklist/allowlist data, iterated later in strings.Contains
	// or similar. Skip from flag. Heuristic: if composite literal Type is
	// ArrayType of string or MapType with string key/value, treat all
	// child literals as patterns.
	ast.Inspect(node, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if !isStringContainerType(cl.Type) {
			return true
		}
		for _, elt := range cl.Elts {
			markStringLits(elt, blocklistLits)
		}
		return true
	})

	// Collect positions of string literals inside blocks guarded by
	// `runtime.GOOS == "windows"` (intentional Windows path hardcode).
	// Heuristic: walk IfStmt yang condition contain `runtime.GOOS`
	// selector; semua BasicLit di Body ditandai guarded.
	guardedLits := map[token.Pos]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !condHasGOOSCheck(ifs.Cond) {
			return true
		}
		ast.Inspect(ifs.Body, func(bn ast.Node) bool {
			if lit, ok := bn.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				guardedLits[lit.Pos()] = true
			}
			return true
		})
		return true
	})

	ast.Inspect(node, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if blocklistLits[lit.Pos()] || guardedLits[lit.Pos()] {
			return true
		}
		raw := strings.Trim(lit.Value, "`\"")
		if raw == "" || len(raw) > 512 {
			return true
		}

		switch {
		case posixRootPattern.MatchString(raw):
			// Allow short POSIX markers inside URL ("/v1/...") or HTTP endpoint.
			if strings.Contains(raw, "://") {
				return true
			}
			recordPath(filePath, fset.Position(lit.Pos()).Line, raw,
				"Hardcoded POSIX root path — akan pecah di Windows. Pakai `os.UserHomeDir()`, `os.TempDir()`, atau `filepath.Join(home, \".flowork\", ...)`.")
		case winDrivePattern.MatchString(raw):
			recordPath(filePath, fset.Position(lit.Pos()).Line, raw,
				"Hardcoded Windows drive letter — pecah di Linux/macOS/RPi. Pakai `os.UserHomeDir()` + filepath.Join.")
		case uncPattern.MatchString(raw):
			recordPath(filePath, fset.Position(lit.Pos()).Line, raw,
				"Hardcoded UNC path — only Windows. Kalau intentional, guard dengan runtime.GOOS check.")
		case winUserPattern.MatchString(raw):
			recordPath(filePath, fset.Position(lit.Pos()).Line, raw,
				"Hardcoded Windows user path fragment — pecah di non-Windows OS.")
		}
		return true
	})
}

// condHasGOOSCheck returns true kalau expr reference runtime.GOOS.
func condHasGOOSCheck(e ast.Expr) bool {
	if e == nil {
		return false
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "runtime" && sel.Sel.Name == "GOOS" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// isStringContainerType true kalau AST type expression = []string / map[...]string
// / map[string]... — typical blocklist/allowlist container.
func isStringContainerType(t ast.Expr) bool {
	if t == nil {
		return false
	}
	switch x := t.(type) {
	case *ast.ArrayType:
		if id, ok := x.Elt.(*ast.Ident); ok && id.Name == "string" {
			return true
		}
	case *ast.MapType:
		kOK := false
		vOK := false
		if id, ok := x.Key.(*ast.Ident); ok && id.Name == "string" {
			kOK = true
		}
		if id, ok := x.Value.(*ast.Ident); ok && (id.Name == "string" || id.Name == "bool") {
			vOK = true
		}
		return kOK || vOK
	}
	return false
}

// markStringLits flag semua BasicLit STRING di elt (termasuk nested).
func markStringLits(elt ast.Node, out map[token.Pos]bool) {
	ast.Inspect(elt, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out[lit.Pos()] = true
		}
		return true
	})
}

func recordPath(file string, line int, literal, msg string) {
	p := strings.ReplaceAll(file, "\\", "/")
	if strings.HasPrefix(p, "scanner/") || strings.Contains(p, "/scanner/") {
		return
	}
	pathFindings = append(pathFindings, PathFinding{
		Level:   "HIGH",
		File:    file,
		Line:    line,
		Literal: literal,
		Message: msg,
	})
}

func writeCrossOSReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 🌍 Cross-OS Path Audit\n\n")
	out.WriteString("> **Scanner baru rc113** — per GOL_FLOWORK.MD: \"JANGAN hardcode `/` atau `\\`\". Flowork target cross-OS (Win + Linux + RPi + macOS). Hardcoded path literal akan pecah di OS lain.\n\n")
	out.WriteString("## Pattern yang Di-flag\n\n")
	out.WriteString("- `/tmp/`, `/var/`, `/etc/`, `/home/`, `/usr/`, `/opt/` → POSIX root (pecah di Windows)\n")
	out.WriteString("- `C:\\`, `D:\\`, `E:\\` → Windows drive letter (pecah di non-Windows)\n")
	out.WriteString("- `\\\\server\\share` → UNC (Windows only)\n")
	out.WriteString("- `\\Users\\`, `\\ProgramData\\`, `\\Windows\\` → Windows user hardcode\n\n")
	out.WriteString("## Allowlist (di-skip)\n\n")
	out.WriteString("- File dengan suffix `_windows.go`, `_linux.go`, `_darwin.go`, `_unix.go` (platform-specific, build tag).\n")
	out.WriteString("- String yang contain `://` (URL path fragment).\n")
	out.WriteString("- Test files + folder scanner/ + state/.\n\n")

	if len(pathFindings) == 0 {
		out.WriteString("*Tidak ada hardcoded non-portable path ditemukan.*\n")
		return
	}

	out.WriteString(fmt.Sprintf("## 🚫 Ditemukan %d Hardcoded Path\n\n", len(pathFindings)))
	for _, f := range pathFindings {
		out.WriteString(fmt.Sprintf("---\n### [%s] `%s:%d`\n", f.Level, f.File, f.Line))
		out.WriteString(fmt.Sprintf("**Literal**: `%q`\n", f.Literal))
		out.WriteString(fmt.Sprintf("**Fix**: %s\n\n", f.Message))
	}
}

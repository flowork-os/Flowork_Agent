//go:build ignore

// deprecated_hash_scanner — deteksi penggunaan MD5/SHA1 di konteks keamanan.
//
// Pattern detect: import "crypto/md5", import "crypto/sha1", md5.New(), sha1.New(),
//
//	md5.Sum(), sha1.Sum().
//
// False positive guard: skip jika digunakan untuk non-security purpose (checksum only).
// Severity: HIGH — MD5/SHA1 collision attacks sudah practical sejak 2017.
// Policy: per GOL_FLOWORK §FASE 7 (Sistem Imun — anti crypto weakness).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Issue struct {
	Level   string
	File    string
	Line    int
	Message string
}

func main() {
	start := time.Now()
	fmt.Println("🔐 [\033[1;31mDEPRECATED HASH SCANNER\033[0m] Cari MD5/SHA1 di security-sensitive code...")

	fset := token.NewFileSet()
	var issues []Issue

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			b := filepath.Base(path)
			if b == ".git" || b == "vendor" || b == "scanner" || b == "_sgvp" || b == "node_modules" || b == "tools_temp" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// Check imports
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			pos := fset.Position(imp.Pos())

			switch importPath {
			case "crypto/md5":
				issues = append(issues, Issue{
					Level:   "HIGH",
					File:    pos.Filename,
					Line:    pos.Line,
					Message: "Import `crypto/md5` — MD5 sudah broken untuk security (collision attack 2^18 ops). Gunakan crypto/sha256 atau crypto/sha512. Hanya OK untuk non-security checksum (content-addressing, cache key).",
				})
			case "crypto/sha1":
				issues = append(issues, Issue{
					Level:   "HIGH",
					File:    pos.Filename,
					Line:    pos.Line,
					Message: "Import `crypto/sha1` — SHA1 sudah broken (Google SHAttered 2017, cost <$100K). Gunakan crypto/sha256 minimum. SHA1 hanya OK untuk git compatibility.",
				})
			}
		}

		// Check function calls
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok {
					funcName := pkg.Name + "." + sel.Sel.Name

					switch funcName {
					case "md5.New", "md5.Sum":
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Level:   "HIGH",
							File:    pos.Filename,
							Line:    pos.Line,
							Message: fmt.Sprintf("`%s()` — MD5 untuk security purposes = vulnerability. Collision attack practical. Migrasi ke sha256.New().", funcName),
						})
					case "sha1.New", "sha1.Sum":
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Level:   "MEDIUM",
							File:    pos.Filename,
							Line:    pos.Line,
							Message: fmt.Sprintf("`%s()` — SHA1 deprecated untuk security. SHAttered attack 2017 proved practical collision. Migrasi ke sha256.New().", funcName),
						})
					}
				}
			}
			return true
		})
		return nil
	})

	for _, i := range issues {
		fmt.Printf("[%s] %s:%d — %s\n", i.Level, i.File, i.Line, i.Message)
	}
	fmt.Printf("\n✅ DEPRECATED HASH scanner done in %s. %d findings.\n",
		time.Since(start).Truncate(time.Millisecond), len(issues))

	outFile := filepath.Join(".", "docs", "bug", "ext_deprecated_hash_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeReport(outFile, issues, "🔐 Deprecated Hash", "flowork_deprecated_hash_auditor.go", "MD5/SHA1 di security-sensitive code")
	fmt.Println("📜 Report:", outFile)

	if len(issues) > 0 {
		os.Exit(1)
	}
}

func writeReport(outFile string, issues []Issue, title, scanner, target string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()
	out.WriteString(fmt.Sprintf("# %s Scanner Report\n\n", title))
	out.WriteString(fmt.Sprintf("> **Scanner:** %s\n", scanner))
	out.WriteString(fmt.Sprintf("> **Target:** %s\n\n", target))
	if len(issues) == 0 {
		out.WriteString("✅ *Tidak ditemukan issue di codebase.*\n")
		return
	}
	crit, high, med, low := 0, 0, 0, 0
	for _, f := range issues {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		case "LOW":
			low++
		}
	}
	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 %d | 🟠 %d | 🟡 %d | 🔵 %d)\n\n", len(issues), crit, high, med, low))
	for i, f := range issues {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s]\n", i+1, f.Level))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

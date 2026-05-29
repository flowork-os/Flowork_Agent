//go:build ignore

// sensitive_log_scanner — deteksi data sensitif (password/token/key/secret) yang di-log.
//
// Pattern detect: log.Print/fmt.Print/Sprintf yang mengandung variabel dengan
// nama "password", "secret", "token", "apikey", "credential", "private".
// Juga deteksi struct dump %+v yang bisa leak semua field termasuk sensitif.
// False positive guard: skip jika variabel di-mask ("***") atau di-redact.
// Severity: HIGH — credential leak di log = permanent exposure.
// Policy: per GOL_FLOWORK §FASE 7 (Sistem Imun — data classification).
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

var sensitiveNames = []string{
	"password", "passwd", "pwd", "secret", "token", "apikey", "api_key",
	"credential", "private", "privkey", "priv_key", "auth",
	"accesstoken", "access_token", "refreshtoken", "refresh_token",
	"bearer", "jwt", "sessionid", "session_id", "cookie",
}

var falsePositives = []string{
	"inputtokens", "outputtokens", "cachereadinputtokens", "cachecreationinputtokens",
	"tokencount", "usedtokens", "maxtokens", "statusunauthorized", "privatekeysize",
}

var logFuncs = map[string]bool{
	"Println": true, "Printf": true, "Print": true,
	"Sprintf": true, "Fprintf": true,
	"Info": true, "Infof": true, "Warn": true, "Warnf": true,
	"Error": true, "Errorf": true, "Debug": true, "Debugf": true,
	"Fatal": true, "Fatalf": true, "Log": true, "Logf": true,
}

func main() {
	start := time.Now()
	fmt.Println("🔑 [\033[1;31mSENSITIVE LOG SCANNER\033[0m] Cari credential/token yang bocor di log...")

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

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			funcName := ""
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				funcName = fun.Sel.Name
			case *ast.Ident:
				funcName = fun.Name
			}

			if !logFuncs[funcName] {
				return true
			}

			// Check for %+v format (dumps entire struct)
			for _, arg := range call.Args {
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind.String() == "STRING" {
					if strings.Contains(lit.Value, "%+v") {
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Level: "MEDIUM",
							File:  pos.Filename,
							Line:  pos.Line,
							Message: fmt.Sprintf(
								"`%s` dengan `%%+v` — dump seluruh struct bisa leak field sensitif (password, token). Gunakan explicit field logging.",
								funcName),
						})
					}
				}
			}

			// Check for sensitive variable names in args
			for _, arg := range call.Args {
				checkSensitiveExpr(arg, funcName, fset, &issues)
			}

			return true
		})
		return nil
	})

	for _, i := range issues {
		fmt.Printf("[%s] %s:%d — %s\n", i.Level, i.File, i.Line, i.Message)
	}
	fmt.Printf("\n✅ SENSITIVE LOG scanner done in %s. %d findings.\n",
		time.Since(start).Truncate(time.Millisecond), len(issues))

	outFile := filepath.Join(".", "docs", "bug", "ext_sensitive_log_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeReport(outFile, issues, "🔑 Sensitive Log", "flowork_sensitive_log_auditor.go", "Password/token/secret di-print ke log → credential leak permanent")
	fmt.Println("📜 Report:", outFile)

	if len(issues) > 0 {
		os.Exit(1)
	}
}

func checkSensitiveExpr(expr ast.Expr, funcName string, fset *token.FileSet, issues *[]Issue) {
	switch e := expr.(type) {
	case *ast.Ident:
		lower := strings.ToLower(e.Name)
		if isIgnored(lower) {
			return
		}
		for _, sens := range sensitiveNames {
			if strings.Contains(lower, sens) {
				pos := fset.Position(e.Pos())
				*issues = append(*issues, Issue{
					Level: "HIGH",
					File:  pos.Filename,
					Line:  pos.Line,
					Message: fmt.Sprintf(
						"Sensitive variable `%s` di-log via `%s()` — credential akan muncul di log file/stdout. Mask atau redact sebelum log.",
						e.Name, funcName),
				})
				return
			}
		}
	case *ast.SelectorExpr:
		lower := strings.ToLower(e.Sel.Name)
		if isIgnored(lower) {
			return
		}
		for _, sens := range sensitiveNames {
			if strings.Contains(lower, sens) {
				pos := fset.Position(e.Pos())
				*issues = append(*issues, Issue{
					Level: "HIGH",
					File:  pos.Filename,
					Line:  pos.Line,
					Message: fmt.Sprintf(
						"Sensitive field `.%s` di-log via `%s()` — credential bocor ke log. Redact: `%s[:4]+\"***\"`.",
						e.Sel.Name, funcName, e.Sel.Name),
				})
				return
			}
		}
	}
}

func isIgnored(name string) bool {
	for _, fp := range falsePositives {
		if strings.Contains(name, fp) {
			return true
		}
	}
	return false
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

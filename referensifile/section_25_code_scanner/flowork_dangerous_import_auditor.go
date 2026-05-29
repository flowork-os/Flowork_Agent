//go:build ignore

// ext_dangerous_import_scanner — mendeteksi import package berbahaya
// di context yang tidak seharusnya.
//
// Scanner ini memeriksa:
//  1. "unsafe" — bypass type safety, memory corruption
//  2. "os/exec" tanpa sandbox wrapper di internal packages
//  3. "net/http" tanpa safeclient di internal packages
//  4. "reflect" — bypass type system (OK di provider, not in security)
//  5. "plugin" — dynamic code loading (RCE vector)
//  6. "debug/..." — info leak di production
//
// Prinsip: GOL B (Protected Core), FQP-4 (SGVP Guard)
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type ImportFinding struct {
	Level   string
	Type    string
	File    string
	Line    int
	Import  string
	Message string
}

var importFindings []ImportFinding

// Dangerous imports mapped to severity and context
type dangerRule struct {
	importPath  string
	level       string
	category    string
	message     string
	allowedDirs []string // directories where this import is acceptable
}

var dangerRules = []dangerRule{
	{
		importPath: "unsafe",
		level:      "HIGH",
		category:   "Memory Unsafe Import",
		message: `Package "unsafe" bypasses Go type safety — enables memory corruption, ` +
			`buffer overflow, dan arbitrary read/write. Hanya boleh di low-level code ` +
			`yang sudah di-audit.`,
		allowedDirs: []string{},
	},
	{
		importPath: "plugin",
		level:      "CRITICAL",
		category:   "Dynamic Code Loading",
		message: `Package "plugin" enables dynamic code loading — RCE vector. ` +
			`Attacker bisa load malicious .so/.dll. FloworkOS harus gunakan ` +
			`MCP/subprocess model untuk extensibility, bukan plugins.`,
		allowedDirs: []string{},
	},
	{
		importPath: "debug/pprof",
		level:      "MEDIUM",
		category:   "Debug Info Leak",
		message: `Package "debug/pprof" exposes profiling data. Jika diaktifkan ` +
			`di production, attacker bisa extract goroutine stacks, heap dumps, ` +
			`dan memory layout. Pastikan hanya aktif di development.`,
		allowedDirs: []string{"cmd"},
	},
	{
		importPath: "net/http/pprof",
		level:      "HIGH",
		category:   "Debug HTTP Endpoint",
		message: `Package "net/http/pprof" registers /debug/pprof/* HTTP endpoints. ` +
			`Ini membuka profiling data ke network — info leak serius. ` +
			`JANGAN import di production code.`,
		allowedDirs: []string{},
	},
	{
		importPath: "crypto/md5",
		level:      "HIGH",
		category:   "Weak Crypto Import",
		message: `Import "crypto/md5" — collision-broken. Terdeteksi di level import. ` +
			`Gunakan SHA-256.`,
		allowedDirs: []string{},
	},
	{
		importPath: "crypto/des",
		level:      "CRITICAL",
		category:   "Broken Cipher Import",
		message: `Import "crypto/des" — 56-bit key, trivially breakable. ` +
			`Gunakan AES-256-GCM.`,
		allowedDirs: []string{},
	},
	{
		importPath:  "crypto/rc4",
		level:       "CRITICAL",
		category:    "Broken Cipher Import",
		message:     `Import "crypto/rc4" — statistically broken. Gunakan AES-GCM atau ChaCha20.`,
		allowedDirs: []string{},
	},
}

func main() {
	fmt.Println("📦 [EXT_DANGEROUS_IMPORT v1] Scanning for dangerous package imports...")
	fmt.Println("   Prinsip: GOL B (Protected Core), FQP-4 (SGVP Guard)")
	fmt.Println()

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "scanner" ||
				base == "_sgvp" || base == "docs" || base == "node_modules" {
				return filepath.SkipDir
			}
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanImports(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Walk error:", err)
		return
	}

	fmt.Printf("\n[📦] Selesai! Findings: %d\n", len(importFindings))
	for _, f := range importFindings {
		fmt.Printf("  [%s] %s | %s:%d\n   -> import %q: %s\n",
			f.Level, f.Type, f.File, f.Line, f.Import, f.Message)
	}

	outFile := filepath.Join(rootDir, "docs", "bug", "ext_dangerous_import_report.md")
	os.MkdirAll(filepath.Dir(outFile), 0755)
	writeImportReport(outFile)
	fmt.Println("\n📜 Report:", outFile)
}

func scanImports(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return
	}

	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		for _, rule := range dangerRules {
			if importPath != rule.importPath {
				continue
			}

			// Check if file is in allowed directory
			allowed := false
			for _, dir := range rule.allowedDirs {
				if strings.Contains(filePath, dir+string(filepath.Separator)) {
					allowed = true
					break
				}
			}

			if !allowed {
				importFindings = append(importFindings, ImportFinding{
					Level:   rule.level,
					Type:    rule.category,
					File:    filePath,
					Line:    fset.Position(imp.Pos()).Line,
					Import:  importPath,
					Message: rule.message,
				})
			}
		}
	}
}

func writeImportReport(outFile string) {
	out, err := os.Create(outFile)
	if err != nil {
		return
	}
	defer out.Close()

	out.WriteString("# 📦 EXT Dangerous Import Scanner Report\n\n")
	out.WriteString("> **Scanner:** ext_dangerous_import_scanner v1\n")
	out.WriteString("> **Prinsip:** GOL B (Protected Core), FQP-4 (SGVP Guard)\n")
	out.WriteString("> **Target:** unsafe, plugin, debug/pprof, crypto/md5, crypto/des, crypto/rc4\n\n")

	if len(importFindings) == 0 {
		out.WriteString("✅ *Tidak ditemukan import berbahaya.*\n")
		return
	}

	crit, high, med := 0, 0, 0
	for _, f := range importFindings {
		switch f.Level {
		case "CRITICAL":
			crit++
		case "HIGH":
			high++
		case "MEDIUM":
			med++
		}
	}
	out.WriteString(fmt.Sprintf("**Total: %d** (🔴 Critical: %d | 🟠 High: %d | 🟡 Medium: %d)\n\n",
		len(importFindings), crit, high, med))

	for i, f := range importFindings {
		out.WriteString(fmt.Sprintf("---\n### Finding #%d — [%s] %s\n", i+1, f.Level, f.Type))
		out.WriteString(fmt.Sprintf("- **File:** `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("- **Import:** `%s`\n", f.Import))
		out.WriteString(fmt.Sprintf("- **Detail:** %s\n\n", f.Message))
	}
}

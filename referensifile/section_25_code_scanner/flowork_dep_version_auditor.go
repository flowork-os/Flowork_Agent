//go:build ignore

// ext_dep_version_scanner — mendeteksi dependency tanpa version pinning.
//
// Checks: go.sum integrity, replace directives, indirect deps,
//
//	dependency count assessment
//
// Prinsip: FQP-10 (Measurement Collapse), GOL B (Protected Core)
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DepFinding struct {
	Level, Type, File, Message string
	Line                       int
}

var findings []DepFinding

func main() {
	fmt.Println("📦 [EXT_DEP_VERSION v1] Scanning go.mod for dependency risks...")
	fmt.Println("   Prinsip: FQP-10 (Measurement), GOL B (Protected Core)")
	fmt.Println()

	root := "."

	// Check go.mod
	gomod := filepath.Join(root, "go.mod")
	if _, err := os.Stat(gomod); err != nil {
		fmt.Println("go.mod not found, skipping")
		return
	}

	scanGoMod(gomod)

	// Check go.sum exists
	gosum := filepath.Join(root, "go.sum")
	if _, err := os.Stat(gosum); err != nil {
		findings = append(findings, DepFinding{
			Level: "HIGH", Type: "Missing go.sum",
			File: "go.sum", Line: 0,
			Message: "go.sum tidak ditemukan. Tanpa go.sum, dependency integrity " +
				"tidak terverifikasi — supply chain attack bisa mengganti module.",
		})
	}

	fmt.Printf("\n[📦] Selesai! Findings: %d\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s | %s:%d\n   -> %s\n", f.Level, f.Type, f.File, f.Line, f.Message)
	}

	out := filepath.Join(root, "docs", "bug", "ext_dep_version_report.md")
	os.MkdirAll(filepath.Dir(out), 0755)
	writeReport(out)
	fmt.Println("\n📜 Report:", out)
}

func scanGoMod(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	directDeps := 0
	indirectDeps := 0
	hasReplace := false

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Count dependencies
		if strings.Contains(line, "// indirect") {
			indirectDeps++
		} else if strings.HasPrefix(line, "require") || (len(line) > 0 && !strings.HasPrefix(line, "//") &&
			!strings.HasPrefix(line, "module") && !strings.HasPrefix(line, "go ") &&
			!strings.HasPrefix(line, ")") && !strings.HasPrefix(line, "(") &&
			!strings.HasPrefix(line, "replace") && !strings.HasPrefix(line, "exclude") &&
			!strings.HasPrefix(line, "retract") && !strings.HasPrefix(line, "toolchain")) {
			if strings.Contains(line, " v") {
				directDeps++
			}
		}

		// Check for replace directives
		if strings.HasPrefix(line, "replace") {
			hasReplace = true
			// Check if replace points to local path
			if strings.Contains(line, "=>") {
				parts := strings.SplitN(line, "=>", 2)
				if len(parts) == 2 {
					target := strings.TrimSpace(parts[1])
					if strings.HasPrefix(target, ".") || strings.HasPrefix(target, "/") ||
						(len(target) >= 2 && target[1] == ':') {
						findings = append(findings, DepFinding{
							Level: "MEDIUM", Type: "Local Replace Directive",
							File: path, Line: lineNum,
							Message: fmt.Sprintf(
								"replace directive ke path lokal: %s. "+
									"Ini tidak portabel dan tidak akan bekerja di CI/CD atau mesin lain. "+
									"Gunakan versi tag yang di-publish.",
								target),
						})
					}
				}
			}
		}

		// Check for v0.x.x dependencies (unstable API)
		if strings.Contains(line, " v0.") && !strings.HasPrefix(line, "module") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				modName := parts[0]
				version := parts[1]
				// Only flag for direct deps (no // indirect)
				if !strings.Contains(line, "// indirect") {
					findings = append(findings, DepFinding{
						Level: "LOW", Type: "Pre-1.0 Dependency",
						File: path, Line: lineNum,
						Message: fmt.Sprintf(
							"Dependency %s@%s — versi v0.x (pre-1.0, API tidak stabil). "+
								"Breaking changes bisa terjadi kapan saja. Monitor changelog.",
							modName, version),
					})
				}
			}
		}

		// Check for pseudo-versions (commit hash based)
		if strings.Contains(line, "-0.") && strings.Contains(line, "-") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.Count(parts[1], "-") >= 3 {
				findings = append(findings, DepFinding{
					Level: "MEDIUM", Type: "Pseudo-version (Commit Hash)",
					File: path, Line: lineNum,
					Message: fmt.Sprintf(
						"Dependency %s menggunakan pseudo-version (commit-based). "+
							"Pseudo-version tidak stabil dan sulit di-audit. "+
							"Pin ke tagged release jika tersedia.",
						parts[0]),
				})
			}
		}
	}

	_ = hasReplace

	// Assess dependency count
	if directDeps+indirectDeps > 100 {
		findings = append(findings, DepFinding{
			Level: "LOW", Type: "High Dependency Count",
			File: path, Line: 0,
			Message: fmt.Sprintf(
				"Total dependencies: %d direct + %d indirect = %d. "+
					"Supply chain attack surface meningkat. "+
					"Review apakah semua dependency benar-benar dibutuhkan.",
				directDeps, indirectDeps, directDeps+indirectDeps),
		})
	}
}

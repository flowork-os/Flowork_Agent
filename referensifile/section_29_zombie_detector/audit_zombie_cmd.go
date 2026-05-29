// scripts/audit_zombie_cmd.go — Z8/Z10/Z13 zombie audit script.
//
// Scan cmd/ + scripts/*.go, cross-reference against boot/launch bat files
// and Go imports → output categorized report to reports/zombie_audit.md.
//
// Usage:
//   cd floworkos-go
//   go run scripts/audit_zombie_cmd.go
//
// Per ROADMAP_AKTIF decision #8/#9: Ayah review output → per-file keep/delete.
//
//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// status categories
const (
	statusActive       = "🟢 ACTIVE"
	statusInternalOnly = "🟡 INTERNAL-ONLY"
	statusZombie       = "🔴 ZOMBIE"
	statusScript       = "⚪ SCRIPT"
)

type entry struct {
	Path         string   // relative path from floworkos-go root
	Category     string   // "cmd", "cmd/daemons", "scripts"
	Status       string   // one of status* constants
	BootRefs     []string // bat files that reference this
	GoImporters  int      // count of Go files importing this package
	HasMainGo    bool
	Note         string
}

func main() {
	// Resolve project root (floworkos-go/)
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}

	// Verify we're in floworkos-go
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: run this from floworkos-go root (go.mod not found)\n")
		os.Exit(1)
	}

	projectRoot := filepath.Dir(root) // flowork_project root

	fmt.Println("[audit] Scanning cmd/ entries...")
	cmdEntries := scanCmdEntries(root)

	fmt.Println("[audit] Scanning scripts/*.go files...")
	scriptEntries := scanScriptEntries(root)

	fmt.Println("[audit] Loading bat file references...")
	batRefs := loadBatReferences(root, projectRoot)

	fmt.Println("[audit] Scanning Go import references...")
	goImports := scanGoImports(root)

	// Classify each entry
	allEntries := append(cmdEntries, scriptEntries...)
	for i := range allEntries {
		e := &allEntries[i]
		classifyEntry(e, batRefs, goImports)
	}

	// Sort by status then path
	sort.Slice(allEntries, func(i, j int) bool {
		if allEntries[i].Status != allEntries[j].Status {
			return statusOrder(allEntries[i].Status) < statusOrder(allEntries[j].Status)
		}
		return allEntries[i].Path < allEntries[j].Path
	})

	// Generate report
	report := generateReport(allEntries)

	// Write to reports/zombie_audit.md
	reportsDir := filepath.Join(root, "reports")
	os.MkdirAll(reportsDir, 0755)
	outPath := filepath.Join(reportsDir, "zombie_audit.md")
	if err := os.WriteFile(outPath, []byte(report), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	counts := map[string]int{}
	for _, e := range allEntries {
		counts[e.Status]++
	}
	fmt.Printf("\n[audit] DONE — %d entries scanned\n", len(allEntries))
	fmt.Printf("  %s: %d\n", statusActive, counts[statusActive])
	fmt.Printf("  %s: %d\n", statusInternalOnly, counts[statusInternalOnly])
	fmt.Printf("  %s: %d\n", statusZombie, counts[statusZombie])
	fmt.Printf("  %s: %d\n", statusScript, counts[statusScript])
	fmt.Printf("\nReport written to: %s\n", outPath)
}

func statusOrder(s string) int {
	switch s {
	case statusActive:
		return 0
	case statusInternalOnly:
		return 1
	case statusZombie:
		return 2
	case statusScript:
		return 3
	}
	return 4
}

// scanCmdEntries finds all cmd/*/main.go and cmd/daemons/*/main.go
func scanCmdEntries(root string) []entry {
	var entries []entry

	// Top-level cmd/*
	cmdDir := filepath.Join(root, "cmd")
	dirEntries, err := os.ReadDir(cmdDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read cmd/: %v\n", err)
		return entries
	}

	for _, d := range dirEntries {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		if name == "daemons" {
			continue // handle separately
		}
		relPath := filepath.Join("cmd", name)
		mainGo := filepath.Join(root, relPath, "main.go")
		hasMain := fileExists(mainGo)
		entries = append(entries, entry{
			Path:      relPath,
			Category:  "cmd",
			HasMainGo: hasMain,
		})
	}

	// cmd/daemons/*
	daemonsDir := filepath.Join(cmdDir, "daemons")
	daemonEntries, err := os.ReadDir(daemonsDir)
	if err == nil {
		for _, d := range daemonEntries {
			if !d.IsDir() {
				continue
			}
			relPath := filepath.Join("cmd", "daemons", d.Name())
			mainGo := filepath.Join(root, relPath, "main.go")
			hasMain := fileExists(mainGo)
			entries = append(entries, entry{
				Path:      relPath,
				Category:  "cmd/daemons",
				HasMainGo: hasMain,
			})
		}
	}

	return entries
}

// scanScriptEntries finds all scripts/*.go files
func scanScriptEntries(root string) []entry {
	var entries []entry
	scriptsDir := filepath.Join(root, "scripts")
	dirEntries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return entries
	}
	for _, d := range dirEntries {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			continue
		}
		entries = append(entries, entry{
			Path:      filepath.Join("scripts", d.Name()),
			Category:  "scripts",
			HasMainGo: true, // standalone scripts are their own main
		})
	}
	return entries
}

// loadBatReferences scans all .bat files for binary/cmd references
func loadBatReferences(root, projectRoot string) map[string][]string {
	refs := map[string][]string{} // binary-name → list of bat files referencing it

	batFiles := []string{}

	// Collect bat files from multiple locations
	locations := []string{
		projectRoot,                     // go_flowork.bat, training_menu.bat
		filepath.Join(root, "scripts"),  // launch-*.bat, run-*.bat
		filepath.Join(projectRoot, "qc", "sections"), // QC test bats
	}

	for _, loc := range locations {
		filepath.Walk(loc, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".bat") {
				batFiles = append(batFiles, path)
			}
			// Don't recurse too deep
			if info.IsDir() && path != loc {
				rel, _ := filepath.Rel(loc, path)
				if strings.Count(rel, string(os.PathSeparator)) > 2 {
					return filepath.SkipDir
				}
			}
			return nil
		})
	}

	// Also add flow.bat explicitly
	flowBat := filepath.Join(root, "flow.bat")
	if fileExists(flowBat) {
		batFiles = append(batFiles, flowBat)
	}

	for _, batPath := range batFiles {
		content, err := os.ReadFile(batPath)
		if err != nil {
			continue
		}
		contentStr := strings.ToLower(string(content))
		relBat, _ := filepath.Rel(projectRoot, batPath)
		if relBat == "" {
			relBat = filepath.Base(batPath)
		}

		// Check each possible binary name
		scanner := bufio.NewScanner(strings.NewReader(contentStr))
		for scanner.Scan() {
			line := scanner.Text()
			// Skip comments
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "rem ") || strings.HasPrefix(trimmed, "::") {
				continue
			}
			// Extract references to flowork-* or cmd/* patterns
			for _, ref := range extractBinaryRefs(line) {
				refs[ref] = appendUnique(refs[ref], relBat)
			}
		}
	}

	return refs
}

// extractBinaryRefs pulls binary names from a bat line
func extractBinaryRefs(line string) []string {
	var refs []string
	// Pattern 1: flowork-xxx.exe or flowork-xxx (binary name)
	words := strings.Fields(line)
	for _, w := range words {
		w = strings.Trim(w, `"'`)
		// Direct exe reference: build\flowork-xxx.exe or flowork-xxx.exe
		if strings.Contains(w, "flowork-") {
			// Extract the binary name
			base := filepath.Base(w)
			base = strings.TrimSuffix(base, ".exe")
			base = strings.TrimSuffix(base, ".log")
			if strings.HasPrefix(base, "flowork-") {
				refs = append(refs, base)
			}
		}
		// Pattern 2: ./cmd/xxx or cmd/xxx
		if strings.Contains(w, "cmd/") || strings.Contains(w, "cmd\\") {
			// Normalize
			w = strings.ReplaceAll(w, "\\", "/")
			parts := strings.Split(w, "/")
			for i, p := range parts {
				if p == "cmd" && i+1 < len(parts) {
					next := parts[i+1]
					if next == "daemons" && i+2 < len(parts) {
						refs = append(refs, parts[i+2])
					} else {
						// Remove trailing quotes, pipes, etc
						next = strings.TrimRight(next, `"'|&>`)
						refs = append(refs, next)
					}
				}
			}
		}
	}
	return refs
}

// scanGoImports checks if any Go file imports a package from cmd/
func scanGoImports(root string) map[string]int {
	imports := map[string]int{} // package-path-fragment → count

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "_sgvp" || base == "_tmpcheck" || base == "vendor" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip build-ignored files
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Simple import scan — look for cmd/ paths in import blocks
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "/cmd/") {
				// Extract the cmd package being imported
				// e.g. "github.com/teetah2402/flowork/cmd/flowork-scan"
				idx := strings.Index(trimmed, "/cmd/")
				if idx >= 0 {
					pkg := trimmed[idx+5:]
					pkg = strings.Trim(pkg, `"')`)
					pkg = strings.Split(pkg, "/")[0]
					imports[pkg]++
				}
			}
		}
		return nil
	})

	return imports
}

// classifyEntry determines the status of an entry
func classifyEntry(e *entry, batRefs map[string][]string, goImports map[string]int) {
	if e.Category == "scripts" {
		e.Status = statusScript
		baseName := strings.TrimSuffix(filepath.Base(e.Path), ".go")
		// Check if referenced in any bat
		for ref, bats := range batRefs {
			if strings.Contains(ref, baseName) {
				e.BootRefs = bats
				e.Status = statusActive
				break
			}
		}
		return
	}

	// For cmd entries, extract the binary name
	dirName := filepath.Base(e.Path)

	// Check bat references
	for ref, bats := range batRefs {
		if ref == dirName || ref == "flowork-"+dirName || dirName == "flowork-"+ref {
			e.BootRefs = appendUnique(e.BootRefs, bats...)
		}
		// Also check if the ref matches directly
		if strings.Contains(ref, dirName) || strings.Contains(dirName, ref) {
			if ref != "" && dirName != "" && len(ref) > 3 && len(dirName) > 3 {
				e.BootRefs = appendUnique(e.BootRefs, bats...)
			}
		}
	}

	// Check Go imports
	e.GoImporters = goImports[dirName]

	// Classify
	if len(e.BootRefs) > 0 {
		e.Status = statusActive
	} else if e.GoImporters > 0 {
		e.Status = statusInternalOnly
	} else {
		e.Status = statusZombie
	}
}

func generateReport(entries []entry) string {
	var b strings.Builder

	b.WriteString("# Zombie Audit Report — cmd/ + scripts/\n\n")
	b.WriteString(fmt.Sprintf("> **Generated:** %s\n", time.Now().Format("2006-01-02 15:04 MST")))
	b.WriteString("> **Tool:** `go run scripts/audit_zombie_cmd.go`\n")
	b.WriteString("> **Scope:** Z8 (scripts/*.go) + Z10/Z13 (cmd/* binaries)\n")
	b.WriteString("> **Action required:** Ayah review per entry — keep or delete\n\n")
	b.WriteString("---\n\n")

	// Summary counts
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Status]++
	}
	b.WriteString("## Summary\n\n")
	b.WriteString("| Status | Count | Meaning |\n")
	b.WriteString("|---|---|---|\n")
	b.WriteString(fmt.Sprintf("| %s | %d | Referenced in boot/launch scripts |\n", statusActive, counts[statusActive]))
	b.WriteString(fmt.Sprintf("| %s | %d | Imported by Go code but not in boot scripts |\n", statusInternalOnly, counts[statusInternalOnly]))
	b.WriteString(fmt.Sprintf("| %s | %d | No boot ref AND no Go import — candidate for delete |\n", statusZombie, counts[statusZombie]))
	b.WriteString(fmt.Sprintf("| %s | %d | Standalone scripts (one-shot migration/seed utilities) |\n", statusScript, counts[statusScript]))
	b.WriteString(fmt.Sprintf("| **TOTAL** | **%d** | |\n\n", len(entries)))

	// Detailed tables per status
	currentStatus := ""
	for _, e := range entries {
		if e.Status != currentStatus {
			currentStatus = e.Status
			b.WriteString(fmt.Sprintf("---\n\n## %s\n\n", currentStatus))
			b.WriteString("| Path | Category | Boot Refs | Go Imports | Ayah Decision |\n")
			b.WriteString("|---|---|---|---|---|\n")
		}

		bootRefs := "-"
		if len(e.BootRefs) > 0 {
			bootRefs = strings.Join(e.BootRefs, ", ")
			if len(bootRefs) > 60 {
				bootRefs = bootRefs[:57] + "..."
			}
		}

		goImp := "-"
		if e.GoImporters > 0 {
			goImp = fmt.Sprintf("%d files", e.GoImporters)
		}

		decision := "⬜ _pending_"
		if e.Status == statusActive {
			decision = "✅ keep"
		}

		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
			e.Path, e.Category, bootRefs, goImp, decision))
	}

	b.WriteString("\n---\n\n")
	b.WriteString("## Decision Guide\n\n")
	b.WriteString("- **🟢 ACTIVE**: These are used in production. Do NOT delete.\n")
	b.WriteString("- **🟡 INTERNAL-ONLY**: Used by other Go code but not boot scripts. Likely library packages — verify before delete.\n")
	b.WriteString("- **🔴 ZOMBIE**: Not referenced anywhere. Candidate for delete. Check git log for last meaningful change.\n")
	b.WriteString("- **⚪ SCRIPT**: One-shot migration/seed scripts. Keep if Ayah might re-run; delete if migration is permanent.\n\n")
	b.WriteString("**To mark decision:** Edit `Ayah Decision` column → `✅ keep` or `❌ delete` or `⏸️ defer`.\n\n")
	b.WriteString("**After decision:** AI external execute delete per Ayah's column marks + `go vet ./...` + `go build ./...` verify.\n")

	return b.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appendUnique(slice []string, items ...string) []string {
	seen := map[string]bool{}
	for _, s := range slice {
		seen[s] = true
	}
	for _, item := range items {
		if !seen[item] {
			slice = append(slice, item)
			seen[item] = true
		}
	}
	return slice
}

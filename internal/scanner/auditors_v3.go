// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 2 — 10 auditor lagi. Auto-register via init().
//
// auditors_v3.go — 10 auditor:
//   complexity, dockerfile_security, dep_version, atomic_write, concurrency,
//   dangerous_import, crossos, defer_close, empty_select, context_value.

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["complexity_auditor"]          = AuditComplexity
	Auditors["dockerfile_security_auditor"] = AuditDockerfileSecurity
	Auditors["dep_version_auditor"]         = AuditDepVersion
	Auditors["atomic_write_auditor"]        = AuditAtomicWrite
	Auditors["concurrency_auditor"]         = AuditConcurrency
	Auditors["dangerous_import_auditor"]    = AuditDangerousImport
	Auditors["crossos_auditor"]             = AuditCrossOS
	Auditors["defer_close_auditor"]         = AuditDeferClose
	Auditors["empty_select_auditor"]        = AuditEmptySelect
	Auditors["context_value_auditor"]       = AuditContextValue
}

// =============================================================================
// 1. complexity_auditor — function dengan >50 line / >5 nested level
// =============================================================================

var funcStartRE = regexp.MustCompile(`^func\s+(\(\s*\w+\s+\*?\w+\s*\)\s*)?\w+\s*\([^)]*\)`)

func AuditComplexity(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !funcStartRE.MatchString(strings.TrimSpace(line)) {
			continue
		}
		// Cari closing } di level 0
		depth := 0
		funcLine := i
		funcEnd := i
		started := false
		for j := i; j < len(lines); j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			if depth > 0 {
				started = true
			}
			if started && depth == 0 {
				funcEnd = j
				break
			}
		}
		funcLen := funcEnd - funcLine + 1
		if funcLen > 80 {
			out = append(out, Finding{
				Auditor:     "complexity_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  funcLine + 1,
				Message:     "function panjang (" + intToStr(funcLen) + " line) — high complexity, hard to test",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "split jadi helper function < 50 line; pertimbangkan extract method",
			})
		}
	}
	return out
}

// =============================================================================
// 2. dockerfile_security_auditor — Dockerfile USER root, no HEALTHCHECK
// =============================================================================

func AuditDockerfileSecurity(filePath, content string) []Finding {
	if filepathBase(filePath) != "Dockerfile" && !strings.HasSuffix(filePath, ".dockerfile") {
		return nil
	}
	out := []Finding{}
	hasUser := false
	hasHealthcheck := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "USER ") {
			hasUser = true
			rest := strings.TrimSpace(trimmed[5:])
			if rest == "root" || rest == "0" {
				out = append(out, Finding{
					Auditor:     "dockerfile_security_auditor",
					Severity:    SevHigh,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "Dockerfile USER root — container privilege escalation risk",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "ganti ke non-root user: `USER nobody` atau `USER appuser` setelah RUN useradd",
				})
			}
		}
		if strings.HasPrefix(upper, "HEALTHCHECK ") {
			hasHealthcheck = true
		}
		if strings.HasPrefix(upper, "ADD ") && strings.Contains(line, "http://") {
			out = append(out, Finding{
				Auditor:     "dockerfile_security_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "Dockerfile ADD via http:// — no integrity check, MITM risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai https:// + verify checksum (RUN sha256sum -c), atau COPY local file",
			})
		}
	}
	if !hasUser {
		out = append(out, Finding{
			Auditor:     "dockerfile_security_auditor",
			Severity:    SevMedium,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "Dockerfile tanpa USER directive — container default root",
			Snippet:     "",
			Remediation: "tambah `USER nobody` (atau dedicated non-root user) sebelum CMD/ENTRYPOINT",
		})
	}
	if !hasHealthcheck {
		out = append(out, Finding{
			Auditor:     "dockerfile_security_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "Dockerfile tanpa HEALTHCHECK — orchestrator ngga bisa detect zombie container",
			Snippet:     "",
			Remediation: "tambah `HEALTHCHECK --interval=30s CMD curl -f http://localhost:PORT/health || exit 1`",
		})
	}
	return out
}

func filepathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	if i := strings.LastIndexByte(p, '\\'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// =============================================================================
// 3. dep_version_auditor — go.mod pin version specific patterns
// =============================================================================

var unpinnedDepRE = regexp.MustCompile(`(github\.com/[^\s]+|golang\.org/[^\s]+)\s+(v0\.0\.0|latest)`)

func AuditDepVersion(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, "go.mod") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if unpinnedDepRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "dep_version_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "dependency tanpa version pin (v0.0.0 / latest) — supply chain risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pin ke specific version: `go get github.com/foo/bar@v1.2.3` atau commit hash",
			})
		}
	}
	return out
}

// =============================================================================
// 4. atomic_write_auditor — os.WriteFile / ioutil.WriteFile tanpa atomic rename
// =============================================================================

var writeFileRE = regexp.MustCompile(`(os|ioutil)\.WriteFile\s*\(`)

func AuditAtomicWrite(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if writeFileRE.MatchString(line) {
			// Heuristic: kalau line sebelum/sesudah ngga ada os.Rename, possibly non-atomic.
			out = append(out, Finding{
				Auditor:     "atomic_write_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "WriteFile non-atomic — kalau crash mid-write, file corrupted/partial",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "untuk critical config: tulis ke .tmp file dulu, lalu os.Rename ke target (atomic POSIX)",
			})
		}
	}
	return out
}

// =============================================================================
// 5. concurrency_auditor — global var write tanpa mutex
// =============================================================================

var globalWriteRE = regexp.MustCompile(`^\s*(var|map\[|\w+)\s*\[?[a-zA-Z_]\w*\]?\s*=`)
var rangeRE = regexp.MustCompile(`go func\s*\(\s*\)\s*\{[^}]*range\b`)

func AuditConcurrency(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if rangeRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "concurrency_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "goroutine `go func() { range ... }` — loop var capture issue (Go <1.22) atau shared slice race",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "Go 1.22+ aman per-iteration. Go <1.22 capture eksplisit: `go func(v T) { ... }(item)`",
			})
		}
	}
	return out
}

// =============================================================================
// 6. dangerous_import_auditor — unsafe, reflect, plugin
// =============================================================================

var dangerousImportRE = regexp.MustCompile(`"(unsafe|plugin|syscall|debug/elf|crypto/dsa)"`)

func AuditDangerousImport(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		m := dangerousImportRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pkg := m[1]
		sev := SevMedium
		if pkg == "unsafe" || pkg == "plugin" {
			sev = SevHigh
		}
		out = append(out, Finding{
			Auditor:     "dangerous_import_auditor",
			Severity:    sev,
			FilePath:    filePath,
			LineNumber:  i + 1,
			Message:     "import package potensi bahaya: " + pkg,
			Snippet:     truncateSnippet(line, 120),
			Remediation: "audit usage — unsafe.Pointer = memory safety bypass; plugin = sideload code; syscall = portability+security",
		})
	}
	return out
}

// =============================================================================
// 7. crossos_auditor — Linux/Unix-only syscalls in cross-platform file
// =============================================================================

var unixOnlyRE = regexp.MustCompile(`\b(syscall\.SIGKILL|syscall\.Kill|syscall\.Setuid|os\.Geteuid|os\.Setegid|syscall\.Chroot)\b`)

func AuditCrossOS(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	// Skip _linux.go / _windows.go / _darwin.go (intentional platform code).
	for _, plat := range []string{"_linux.go", "_windows.go", "_darwin.go", "_unix.go", "_bsd.go"} {
		if strings.HasSuffix(filePath, plat) {
			return nil
		}
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if unixOnlyRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "crossos_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "syscall Unix-only di file cross-platform — Windows build akan fail",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "move ke file `_unix.go` dengan //go:build !windows, atau pakai os.Process portable equivalent",
			})
		}
	}
	return out
}

// =============================================================================
// 8. defer_close_auditor — defer Close() without err handling
// =============================================================================

var deferCloseRE = regexp.MustCompile(`^\s*defer\s+\w+\.Close\s*\(\s*\)\s*$`)

func AuditDeferClose(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if deferCloseRE.MatchString(line) {
			// Skip kalau context atau cancel func.
			if strings.Contains(line, "cancel.Close") || strings.Contains(line, "ctx.Close") {
				continue
			}
			out = append(out, Finding{
				Auditor:     "defer_close_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "defer Close() tanpa err check — kehilangan write error untuk disk flush",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "untuk write file: `defer func() { if err := f.Close(); err != nil { ... } }()`",
			})
		}
	}
	return out
}

// =============================================================================
// 9. empty_select_auditor — select {} dead-block forever
// =============================================================================

var emptySelectRE = regexp.MustCompile(`^\s*select\s*\{\s*\}`)

func AuditEmptySelect(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if emptySelectRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "empty_select_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "select {} kosong — block goroutine selamanya (deadlock intentional?)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "kalau intent block main forever, eksplisit pakai `<-ctx.Done()` atau `runtime.Goexit()`",
			})
		}
	}
	return out
}

// =============================================================================
// 10. context_value_auditor — context.WithValue tanpa typed key
// =============================================================================

var ctxValueStringKeyRE = regexp.MustCompile(`context\.WithValue\s*\(\s*\w+\s*,\s*"`)

func AuditContextValue(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if ctxValueStringKeyRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "context_value_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "context.WithValue pakai string key — collision-prone + anti pattern Go",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "deklarasi unexported `type myKey struct{}; var myKeyInstance myKey` lalu `WithValue(ctx, myKeyInstance, val)`",
			})
		}
	}
	return out
}

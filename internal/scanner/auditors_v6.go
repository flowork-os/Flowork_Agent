// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 5 — 10 auditor.
//
// auditors_v6.go:
//   global_log_init, env_dependency, magic_number, struct_tag_typo,
//   integer_overflow, file_no_close, http_no_body_close,
//   string_concat_loop, slice_append_loop, sync_once_misuse.

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["global_log_init_auditor"]   = AuditGlobalLogInit
	Auditors["env_dependency_auditor"]    = AuditEnvDependency
	Auditors["magic_number_auditor"]      = AuditMagicNumber
	Auditors["struct_tag_typo_auditor"]   = AuditStructTagTypo
	Auditors["integer_overflow_auditor"]  = AuditIntegerOverflow
	Auditors["file_no_close_auditor"]     = AuditFileNoClose
	Auditors["http_no_body_close_auditor"] = AuditHTTPNoBodyClose
	Auditors["string_concat_loop_auditor"] = AuditStringConcatLoop
	Auditors["slice_append_loop_auditor"]  = AuditSliceAppendLoop
	Auditors["sync_once_misuse_auditor"]   = AuditSyncOnceMisuse
}

// =============================================================================
// 1. global_log_init_auditor — log.Println in package var = side effect on import
// =============================================================================

var globalLogInitRE = regexp.MustCompile(`^var\s+\w+\s*=\s*log\.`)

func AuditGlobalLogInit(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if globalLogInitRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "global_log_init_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "package-level var = log.* — runs at import time, side effect",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "move ke func init() yang explicit, atau lazy via sync.Once",
			})
		}
	}
	return out
}

// =============================================================================
// 2. env_dependency_auditor — os.Getenv tanpa fallback / required check
// =============================================================================

var envGetenvRE = regexp.MustCompile(`os\.Getenv\s*\(\s*"[A-Z_]+"\s*\)`)

func AuditEnvDependency(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !envGetenvRE.MatchString(line) {
			continue
		}
		// Heuristic: check 3 line setelahnya untuk `if X == "" {` (fallback handling)
		window := lines[i:minInt(i+5, len(lines))]
		hasFallback := false
		for _, w := range window {
			if (strings.Contains(w, `== ""`) || strings.Contains(w, `!= ""`)) && strings.Contains(w, "if") {
				hasFallback = true
				break
			}
			if strings.Contains(w, "LookupEnv") {
				hasFallback = true
				break
			}
		}
		if !hasFallback {
			out = append(out, Finding{
				Auditor:     "env_dependency_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "os.Getenv tanpa fallback check — kalau env absent, return zero string silently",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "guard: `if v := os.Getenv(\"X\"); v == \"\" { return error/default }`",
			})
		}
	}
	return out
}

// =============================================================================
// 3. magic_number_auditor — int literal > 100 di non-test code
// =============================================================================

var magicNumberRE = regexp.MustCompile(`\b(86400|3600|604800|31536000|65535|4096|8192|16384|32768)\b`)

func AuditMagicNumber(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "const") {
			continue
		}
		if magicNumberRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "magic_number_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "magic number — hardcoded constant time/size, extract ke named const",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "deklarasi `const dayInSeconds = 86400` / `const maxPacket = 65535`",
			})
		}
	}
	return out
}

// =============================================================================
// 4. struct_tag_typo_auditor — json tag typo (jsom, jsno, dst)
// =============================================================================

var structTagTypoRE = regexp.MustCompile("`(jsom|jsno|josn|jason|joson):\"")

func AuditStructTagTypo(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if structTagTypoRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "struct_tag_typo_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "struct tag typo (jsom/jsno/josn/etc) — JSON marshal silently ignore field",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti ke `json:\"...\"` lowercase exact",
			})
		}
	}
	return out
}

// =============================================================================
// 5. integer_overflow_auditor — int() cast dari unbounded source
// =============================================================================

var intCastRE = regexp.MustCompile(`int\s*\(\s*\w+\s*\.\s*(Size|Len|Length|Count)\(\)\s*\)`)

func AuditIntegerOverflow(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if intCastRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "integer_overflow_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "int() cast from Size/Len/Count — overflow risk on 32-bit platform",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "kalau bisa pakai int64, atau cap dengan `math.MaxInt32` check sebelum cast",
			})
		}
	}
	return out
}

// =============================================================================
// 6. file_no_close_auditor — os.Open tanpa defer Close
// =============================================================================

var osOpenRE = regexp.MustCompile(`\b(os\.Open|os\.Create|os\.OpenFile)\s*\(`)

func AuditFileNoClose(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !osOpenRE.MatchString(line) {
			continue
		}
		window := lines[i:minInt(i+8, len(lines))]
		hasClose := false
		for _, w := range window {
			if strings.Contains(w, "defer") && strings.Contains(w, ".Close()") {
				hasClose = true
				break
			}
		}
		if !hasClose {
			out = append(out, Finding{
				Auditor:     "file_no_close_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "os.Open/Create/OpenFile tanpa defer Close() dalam 7 line — file descriptor leak",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "tambah `defer f.Close()` setelah error check",
			})
		}
	}
	return out
}

// =============================================================================
// 7. http_no_body_close_auditor — http.Do tanpa defer resp.Body.Close
// =============================================================================

var httpDoRE = regexp.MustCompile(`(\.Do\(|http\.(Get|Post|Head|PostForm)\()`)

func AuditHTTPNoBodyClose(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !httpDoRE.MatchString(line) {
			continue
		}
		// Skip kalau di line yg sama / sebelumnya ada assign to var.
		window := lines[i:minInt(i+6, len(lines))]
		hasClose := false
		for _, w := range window {
			if strings.Contains(w, "defer") && strings.Contains(w, ".Body.Close()") {
				hasClose = true
				break
			}
		}
		if !hasClose {
			out = append(out, Finding{
				Auditor:     "http_no_body_close_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "HTTP request tanpa defer resp.Body.Close() — connection leak",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "tambah `defer resp.Body.Close()` segera setelah err check",
			})
		}
	}
	return out
}

// =============================================================================
// 8. string_concat_loop_auditor — `s += x` dalam for loop
// =============================================================================

func AuditStringConcatLoop(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	depth := 0
	loopStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "for ") && strings.HasSuffix(trimmed, "{") {
			loopStart = depth
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth < 0 {
			depth = 0
		}
		if depth <= loopStart {
			loopStart = -1
		}
		if loopStart >= 0 && strings.Contains(trimmed, "+= ") && strings.Contains(trimmed, "\"") {
			out = append(out, Finding{
				Auditor:     "string_concat_loop_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "string += dalam loop — O(N²), realloc per iterasi",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai `strings.Builder` atau `bytes.Buffer` untuk akumulasi O(N)",
			})
		}
	}
	return out
}

// =============================================================================
// 9. slice_append_loop_auditor — append dalam loop tanpa pre-allocate
// =============================================================================

func AuditSliceAppendLoop(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Pattern: `var X []T` followed by append in loop (no make(slice, 0, N))
		if strings.HasPrefix(trimmed, "var ") && strings.Contains(trimmed, "[]") && !strings.Contains(trimmed, "=") {
			// Look forward 20 line untuk make() atau append() di loop.
			window := lines[i:minInt(i+25, len(lines))]
			hasMake := false
			hasAppendInLoop := false
			depth := 0
			inLoop := false
			for _, w := range window {
				tt := strings.TrimSpace(w)
				if strings.Contains(w, "make([]") {
					hasMake = true
				}
				if strings.HasPrefix(tt, "for ") {
					inLoop = true
				}
				depth += strings.Count(w, "{") - strings.Count(w, "}")
				if depth == 0 {
					inLoop = false
				}
				if inLoop && strings.Contains(w, "append(") {
					hasAppendInLoop = true
				}
			}
			if hasAppendInLoop && !hasMake {
				out = append(out, Finding{
					Auditor:     "slice_append_loop_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "slice + append dalam loop tanpa pre-allocate make([]T, 0, N) — multiple realloc",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "kalau tau capacity, pakai `xs := make([]T, 0, N)` sebelum loop",
				})
			}
		}
	}
	return out
}

// =============================================================================
// 10. sync_once_misuse_auditor — sync.Once.Do dengan func yg dipanggil multi times
// =============================================================================

var syncOnceLocalRE = regexp.MustCompile(`^\s*\w+\s*:?=\s*&?sync\.Once\{?\}?\s*$`)

func AuditSyncOnceMisuse(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Pattern: sync.Once declared inside function (local) — fired every call.
		if syncOnceLocalRE.MatchString(line) {
			// Look back for func declaration.
			isLocal := false
			for j := i - 1; j >= maxInt(0, i-30); j-- {
				t := strings.TrimSpace(lines[j])
				if strings.HasPrefix(t, "func ") {
					isLocal = true
					break
				}
			}
			if isLocal {
				out = append(out, Finding{
					Auditor:     "sync_once_misuse_auditor",
					Severity:    SevMedium,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "sync.Once declared inside func — fired ulang tiap function call, defeats purpose",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "move sync.Once ke package-level var atau struct field",
				})
			}
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

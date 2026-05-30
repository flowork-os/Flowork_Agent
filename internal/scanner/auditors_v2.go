// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Phase 2 — port 10 high-value scanner auditors dari referensi.
//   Pattern-based (lib-style), extends locked auditors.go. Auditor reference
//   pakai AST + main() — terlalu heavy untuk dropin. File ini convert ke
//   regex pattern style sesuai AuditFunc(filePath, content) signature.
//
// auditors_v2.go — 10 auditors:
//   bare_goroutine, mutex_copy, nil_map_write, crypto_weakness,
//   context_leak, defer_in_loop, error_ignored, channel_unbuffered,
//   deprecated_api, hardcoded_path.

package scanner

import (
	"regexp"
	"strings"
)

// init — register v2 auditors ke global Auditors map.
func init() {
	Auditors["bare_goroutine_auditor"]     = AuditBareGoroutine
	Auditors["mutex_copy_auditor"]         = AuditMutexCopy
	Auditors["nil_map_write_auditor"]      = AuditNilMapWrite
	Auditors["crypto_weakness_auditor"]    = AuditCryptoWeakness
	Auditors["context_leak_auditor"]       = AuditContextLeak
	Auditors["defer_in_loop_auditor"]      = AuditDeferInLoop
	Auditors["error_ignored_auditor"]      = AuditErrorIgnored
	Auditors["channel_unbuffered_auditor"] = AuditChannelUnbuffered
	Auditors["deprecated_api_auditor"]     = AuditDeprecatedAPI
	Auditors["hardcoded_path_auditor"]     = AuditHardcodedPath
}

// =============================================================================
// 1. bare_goroutine_auditor — goroutine tanpa recover()
// =============================================================================

var bareGoroutineRE = regexp.MustCompile(`^\s*go\s+(func\s*\(|[a-zA-Z_]\w*\()`)

func AuditBareGoroutine(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !bareGoroutineRE.MatchString(line) {
			continue
		}
		// Heuristic: cek block 10 line setelahnya buat 'recover()'.
		window := lines[i:minInt(i+15, len(lines))]
		hasRecover := false
		for _, w := range window {
			if strings.Contains(w, "recover()") {
				hasRecover = true
				break
			}
		}
		if !hasRecover {
			out = append(out, Finding{
				Auditor:     "bare_goroutine_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "goroutine tanpa defer recover() — panic akan crash binary",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "wrap dengan `defer func() { if r := recover(); r != nil { log.Printf(\"panic: %v\", r) } }()`",
			})
		}
	}
	return out
}

// =============================================================================
// 2. mutex_copy_auditor — sync.Mutex passed by value
// =============================================================================

var mutexCopyRE = regexp.MustCompile(`func\s+\w*\s*\(\s*\w+\s+(\w+)\s*\)`)

func AuditMutexCopy(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	// Naive: struct containing Mutex/RWMutex passed by value receiver.
	for i, line := range strings.Split(content, "\n") {
		m := mutexCopyRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Check next 5 lines kalau receiver type embed Mutex.
		// Imperfect: butuh AST untuk akurat. Pattern hint saja.
		typeName := m[1]
		if strings.Contains(content, "type "+typeName+" struct") {
			structDef := extractStruct(content, typeName)
			if strings.Contains(structDef, "sync.Mutex") || strings.Contains(structDef, "sync.RWMutex") {
				if !strings.Contains(line, "*"+typeName) {
					out = append(out, Finding{
						Auditor:     "mutex_copy_auditor",
						Severity:    SevHigh,
						FilePath:    filePath,
						LineNumber:  i + 1,
						Message:     "method receiver value-passes struct that embeds sync.Mutex — copy = bug",
						Snippet:     truncateSnippet(line, 120),
						Remediation: "ganti receiver ke pointer: `func (s *" + typeName + ") ...`",
					})
				}
			}
		}
	}
	return out
}

// =============================================================================
// 3. nil_map_write_auditor — write ke map yang belum di-init
// =============================================================================

var nilMapWriteRE = regexp.MustCompile(`var\s+(\w+)\s+map\[`)
var mapWriteRE = regexp.MustCompile(`(\w+)\[[^\]]+\]\s*=`)

func AuditNilMapWrite(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	declaredNil := map[string]int{}
	for i, line := range strings.Split(content, "\n") {
		if m := nilMapWriteRE.FindStringSubmatch(line); m != nil {
			// Skip kalau initialized di same line.
			if !strings.Contains(line, "= make(") && !strings.Contains(line, "= map[") {
				declaredNil[m[1]] = i + 1
			}
		}
		// Cek write ke variable yang tracked nil.
		if m := mapWriteRE.FindStringSubmatch(line); m != nil {
			if declLine, ok := declaredNil[m[1]]; ok {
				out = append(out, Finding{
					Auditor:     "nil_map_write_auditor",
					Severity:    SevCritical,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "write ke map yang declare nil (line " + intToStr(declLine) + ") — panic runtime",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "init dengan `make(map[K]V)` sebelum write",
				})
				delete(declaredNil, m[1])
			}
		}
	}
	return out
}

// =============================================================================
// 4. crypto_weakness_auditor — md5/sha1/des/rc4
// =============================================================================

var weakCryptoRE = regexp.MustCompile(`(crypto/md5|crypto/sha1|crypto/des|crypto/rc4)\b|md5\.Sum|sha1\.Sum|des\.New|rc4\.NewCipher`)

func AuditCryptoWeakness(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if weakCryptoRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "crypto_weakness_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "weak cryptographic primitive (md5/sha1/des/rc4) — broken / collision-prone",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti ke SHA-256/SHA-512 untuk hash; AES-256-GCM untuk symmetric encryption",
			})
		}
	}
	return out
}

// =============================================================================
// 5. context_leak_auditor — context.WithCancel tanpa defer cancel()
// =============================================================================

var ctxWithCancelRE = regexp.MustCompile(`context\.With(Cancel|Timeout|Deadline)\s*\(`)

func AuditContextLeak(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !ctxWithCancelRE.MatchString(line) {
			continue
		}
		// Cek 5 line forward for `defer cancel()` atau equivalent.
		window := lines[i:minInt(i+8, len(lines))]
		hasDefer := false
		for _, w := range window {
			if strings.Contains(w, "defer cancel") || strings.Contains(w, "defer ctx") {
				hasDefer = true
				break
			}
		}
		if !hasDefer {
			out = append(out, Finding{
				Auditor:     "context_leak_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "context.WithCancel/Timeout/Deadline tanpa `defer cancel()` dalam ~7 line — potential ctx leak",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "selalu pasang `defer cancel()` segera setelah deklarasi context",
			})
		}
	}
	return out
}

// =============================================================================
// 6. defer_in_loop_auditor — defer dalam for loop
// =============================================================================

func AuditDeferInLoop(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	depth := 0
	loopStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Simple brace tracker — count `{` increase depth, `}` decrease.
		// Track loop start kalau ada `for ` di line yang ada `{`.
		if strings.HasPrefix(trimmed, "for ") && strings.HasSuffix(trimmed, "{") {
			loopStart = depth
		}
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		depth += opens - closes
		if depth < 0 {
			depth = 0
		}
		if depth <= loopStart {
			loopStart = -1
		}
		// Cek defer di dalam loop.
		if loopStart >= 0 && strings.HasPrefix(trimmed, "defer ") {
			out = append(out, Finding{
				Auditor:     "defer_in_loop_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "defer di dalam for loop — execute baru pas function return → resource leak per iterasi",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "wrap iterasi dalam func() inner, atau move defer ke setelah loop, atau Close manual di end of iter",
			})
		}
	}
	return out
}

// =============================================================================
// 7. error_ignored_auditor — err di-discard ke `_`
// =============================================================================

var errIgnoredRE = regexp.MustCompile(`(_,\s*\w+\s*:?=\s*\w+(\.\w+)*\(|\w+,\s*_\s*:?=\s*\w+(\.\w+)*\()`)
var errBlankAssignRE = regexp.MustCompile(`^\s*_\s*=\s*\w+(\.\w+)*\(`)

func AuditErrorIgnored(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Filter: skip `if _, ok := ...` (map lookup pattern OK), skip Close/Cancel
		// (intentional discard idiom).
		if strings.Contains(line, "if _, ok") || strings.Contains(line, "_ =") && (strings.Contains(line, ".Close()") || strings.Contains(line, ".Cancel()")) {
			continue
		}
		if errBlankAssignRE.MatchString(trimmed) && !strings.HasPrefix(trimmed, "// ") {
			// Skip ack-pattern `_ = json.Marshal(...)` etc — too noisy.
			// Only flag obvious like `_ = importantOp()`.
			if !strings.Contains(line, "json.Marshal") && !strings.Contains(line, ".Close") && !strings.Contains(line, ".Stop") {
				out = append(out, Finding{
					Auditor:     "error_ignored_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "error return discarded ke `_` — bisa miss critical failure",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "log atau handle error: `if err := op(); err != nil { ... }`",
				})
			}
		}
	}
	return out
}

// =============================================================================
// 8. channel_unbuffered_auditor — make(chan T) tanpa buffer di critical path
// =============================================================================

var unbufChanRE = regexp.MustCompile(`make\s*\(\s*chan\s+\w+\s*\)`)

func AuditChannelUnbuffered(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if unbufChanRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "channel_unbuffered_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "unbuffered channel — send block sampai receiver ready, bisa deadlock kalau single goroutine",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "kalau intentional (sync handshake), OK. Kalau buffered queue, pakai `make(chan T, N)` dengan N realistic",
			})
		}
	}
	return out
}

// =============================================================================
// 9. deprecated_api_auditor — io/ioutil + deprecated stdlib
// =============================================================================

var deprecatedRE = regexp.MustCompile(`\bio/ioutil\b|ioutil\.(ReadFile|WriteFile|ReadAll|TempFile|TempDir)`)

func AuditDeprecatedAPI(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if deprecatedRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "deprecated_api_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "io/ioutil deprecated di Go 1.16+ — pakai os/io equivalents",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ReadFile/WriteFile pindah ke os.ReadFile/WriteFile; ReadAll ke io.ReadAll; TempFile/Dir ke os.CreateTemp/MkdirTemp",
			})
		}
	}
	return out
}

// =============================================================================
// 10. hardcoded_path_auditor — /home/, C:\Users\, /Users/
// =============================================================================

var hardcodedPathRE = regexp.MustCompile(`["']/?(home/[a-zA-Z]\w+|Users/[a-zA-Z]\w+|tmp/[a-zA-Z]\w+|var/[a-zA-Z]\w+)|["']?[CD]:\\Users\\`)

func AuditHardcodedPath(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		// Skip comment lines.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if hardcodedPathRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "hardcoded_path_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "hardcoded absolute path — pelanggaran prinsip portable + multi-OS",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai os.UserHomeDir(), os.TempDir(), atau filepath.Join() dengan root dari config/env",
			})
		}
	}
	return out
}

// =============================================================================
// Helpers
// =============================================================================

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// extractStruct — return struct definition body untuk type name.
// Naive scanning: cari `type <name> struct {` lalu return sampai balancing `}`.
func extractStruct(content, typeName string) string {
	idx := strings.Index(content, "type "+typeName+" struct {")
	if idx < 0 {
		return ""
	}
	rest := content[idx:]
	depth := 0
	end := -1
	for i, ch := range rest {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		return rest
	}
	return rest[:end]
}

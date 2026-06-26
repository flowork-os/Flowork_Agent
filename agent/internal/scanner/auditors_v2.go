// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["bare_goroutine_auditor"] = AuditBareGoroutine
	Auditors["mutex_copy_auditor"] = AuditMutexCopy
	Auditors["nil_map_write_auditor"] = AuditNilMapWrite
	Auditors["crypto_weakness_auditor"] = AuditCryptoWeakness
	Auditors["context_leak_auditor"] = AuditContextLeak
	Auditors["defer_in_loop_auditor"] = AuditDeferInLoop
	Auditors["error_ignored_auditor"] = AuditErrorIgnored
	Auditors["channel_unbuffered_auditor"] = AuditChannelUnbuffered
	Auditors["deprecated_api_auditor"] = AuditDeprecatedAPI
	Auditors["hardcoded_path_auditor"] = AuditHardcodedPath
}

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

var mutexCopyRE = regexp.MustCompile(`func\s+\w*\s*\(\s*\w+\s+(\w+)\s*\)`)

func AuditMutexCopy(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}

	for i, line := range strings.Split(content, "\n") {
		m := mutexCopyRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}

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

var nilMapWriteRE = regexp.MustCompile(`var\s+(\w+)\s+map\[`)

var mapWriteRE = regexp.MustCompile(`(\w+)\[[^\]]+\]\s*=(?:[^=]|$)`)

var mapReInitRE = regexp.MustCompile(`(\w+)\s*=\s*(make\(\s*map\[|map\[)`)

var mapAllocByRefRE = regexp.MustCompile(`(?:Unmarshal|Decode)\([^)]*&(\w+)`)

func AuditNilMapWrite(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	declaredNil := map[string]int{}
	for i, line := range strings.Split(content, "\n") {
		if m := nilMapWriteRE.FindStringSubmatch(line); m != nil {

			if !strings.Contains(line, "= make(") && !strings.Contains(line, "= map[") {
				declaredNil[m[1]] = i + 1
			}
		}

		if m := mapReInitRE.FindStringSubmatch(line); m != nil {
			delete(declaredNil, m[1])
		}

		if m := mapAllocByRefRE.FindStringSubmatch(line); m != nil {
			delete(declaredNil, m[1])
		}

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

var errIgnoredRE = regexp.MustCompile(`(_,\s*\w+\s*:?=\s*\w+(\.\w+)*\(|\w+,\s*_\s*:?=\s*\w+(\.\w+)*\()`)
var errBlankAssignRE = regexp.MustCompile(`^\s*_\s*=\s*\w+(\.\w+)*\(`)

func AuditErrorIgnored(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(line, "if _, ok") || strings.Contains(line, "_ =") && (strings.Contains(line, ".Close()") || strings.Contains(line, ".Cancel()")) {
			continue
		}
		if errBlankAssignRE.MatchString(trimmed) && !strings.HasPrefix(trimmed, "// ") {

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

var hardcodedPathRE = regexp.MustCompile(`["']/?(home/[a-zA-Z]\w+|Users/[a-zA-Z]\w+|tmp/[a-zA-Z]\w+|var/[a-zA-Z]\w+)|["']?[CD]:\\Users\\`)

func AuditHardcodedPath(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {

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

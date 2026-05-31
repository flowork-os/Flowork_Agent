// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 25 phase 1 minimal viable scanner. 6 of 35 Tier 1 auditor
//   (hardcoded_secret, command_injection, sql_injection, path_traversal,
//   ssrf, token_leak) — high-value subset yang stdlib-only. Phase 2 add
//   sisanya (race, crypto, supply_chain, etc.) → tambah file baru.
//
// auditors.go — Section 25 phase 1: 6 critical auditors.

package scanner

import (
	"regexp"
	"strings"
)

// Severity enum.
const (
	SevCritical = "critical"
	SevHigh     = "high"
	SevMedium   = "medium"
	SevLow      = "low"
	SevInfo     = "info"
)

// Finding — one issue surfaced by an auditor.
type Finding struct {
	Auditor     string `json:"auditor"`
	Severity    string `json:"severity"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Message     string `json:"message"`
	Snippet     string `json:"snippet"`
	Remediation string `json:"remediation"`
}

// AuditFunc — Run terhadap text file. Caller pass filePath + content.
type AuditFunc func(filePath, content string) []Finding

// Auditors registry — caller iterate. Add to slice = self-register.
var Auditors = map[string]AuditFunc{
	"hardcoded_secret_auditor":   AuditHardcodedSecret,
	"command_injection_auditor": AuditCommandInjection,
	"sql_injection_auditor":     AuditSQLInjection,
	"path_traversal_auditor":    AuditPathTraversal,
	"ssrf_auditor":              AuditSSRF,
	"token_leak_auditor":        AuditTokenLeak,
}

// =============================================================================
// 1. hardcoded_secret_auditor
// =============================================================================

var hardcodedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)aws[_\-]?(secret[_\-]?)?access[_\-]?key.*[:=]\s*['"]?([A-Z0-9/+=]{16,40})['"]?`),
	regexp.MustCompile(`(?i)github[_\-]?token\s*[:=]\s*['"]?(gh[pousr]_[A-Za-z0-9]{36,40})['"]?`),
	regexp.MustCompile(`(?i)slack[_\-]?(bot|webhook)?[_\-]?token\s*[:=]\s*['"]?(xox[abp]-[A-Za-z0-9-]{10,})['"]?`),
	regexp.MustCompile(`(?i)stripe[_\-]?(secret|api)[_\-]?key\s*[:=]\s*['"]?(sk_(live|test)_[A-Za-z0-9]{20,})['"]?`),
	regexp.MustCompile(`(?i)openai[_\-]?api[_\-]?key\s*[:=]\s*['"]?(sk-[A-Za-z0-9]{20,})['"]?`),
	regexp.MustCompile(`(?i)telegram[_\-]?bot[_\-]?token\s*[:=]\s*['"]?(\d{8,}:[A-Za-z0-9_-]{30,})['"]?`),
}

func AuditHardcodedSecret(filePath, content string) []Finding {
	out := []Finding{}
	for lineNo, line := range strings.Split(content, "\n") {
		for _, re := range hardcodedPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "hardcoded_secret_auditor",
					Severity:    SevCritical,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "hardcoded secret/token detected",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "move to env var (os.Getenv) or secret manager; rotate exposed key immediately",
				})
				break
			}
		}
	}
	return out
}

// =============================================================================
// 2. command_injection_auditor
// =============================================================================

var commandInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`exec\.Command\s*\(\s*"(sh|bash|cmd|powershell)"\s*,\s*"-c"\s*,\s*[a-zA-Z_]\w*\s*\+`),
	regexp.MustCompile(`exec\.Command\s*\(\s*[a-zA-Z_]\w*\s*\+`),
	regexp.MustCompile(`exec\.CommandContext\s*\([^,]+,\s*"(sh|bash|cmd)"\s*,\s*"-c"\s*,\s*fmt\.Sprintf`),
	regexp.MustCompile(`os\.system\s*\(\s*.*\+\s*\w+`), // python style
}

func AuditCommandInjection(filePath, content string) []Finding {
	out := []Finding{}
	for lineNo, line := range strings.Split(content, "\n") {
		for _, re := range commandInjectionPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "command_injection_auditor",
					Severity:    SevHigh,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "potential command injection — string concat into exec",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "pass args sebagai slice []string ke exec.Command, JANGAN concat string ke shell -c",
				})
				break
			}
		}
	}
	return out
}

// =============================================================================
// 3. sql_injection_auditor
// =============================================================================

// sqlStmt — fragmen yang nandain string BENERAN SQL statement (bukan prosa yang
// kebetulan ada kata "delete"/"insert"). Wajib struktur: DELETE FROM / INSERT
// INTO / UPDATE..SET / SELECT..FROM / WHERE. Anti false-positive prosa kayak
// `"soft-delete missing: "+err` atau `"snapshot insert: "+err`.
const sqlStmt = `(SELECT\b[^"]*\bFROM\b|INSERT\s+INTO\b|UPDATE\s+\S+\s+SET\b|DELETE\s+FROM\b|\bWHERE\s)`

var sqlInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)fmt\.Sprintf\s*\(\s*"[^"]*` + sqlStmt + `[^"]*%s`),
	regexp.MustCompile(`(?i)"[^"]*` + sqlStmt + `[^"]*"\s*\+\s*\w+`),
	regexp.MustCompile(`(?i)db\.(Query|Exec)\s*\(\s*"[^"]*` + sqlStmt + `[^"]*"\s*\+`),
}

func AuditSQLInjection(filePath, content string) []Finding {
	out := []Finding{}
	for lineNo, line := range strings.Split(content, "\n") {
		for _, re := range sqlInjectionPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "sql_injection_auditor",
					Severity:    SevCritical,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "potential SQL injection — string concat in query",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "pakai parameterized query (`db.Query(\"... WHERE x = ?\", value)`)",
				})
				break
			}
		}
	}
	return out
}

// =============================================================================
// 4. path_traversal_auditor
// =============================================================================

var pathTraversalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`filepath\.Join\s*\([^)]*\w+\s*\)`), // join with user var
	regexp.MustCompile(`os\.Open\s*\(\s*\w+\s*\)`),         // open with raw var
	regexp.MustCompile(`os\.Create\s*\(\s*\w+\s*\)`),
	regexp.MustCompile(`ioutil\.ReadFile\s*\(\s*\w+\s*\)`),
	regexp.MustCompile(`os\.ReadFile\s*\(\s*\w+\s*\)`),
}

func AuditPathTraversal(filePath, content string) []Finding {
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for lineNo, line := range lines {
		// Heuristic: skip kalau line ada `filepath.Base(` (sanitization) atau
		// `filepath.Clean(` atau `strings.Contains(..., "..")` (defense).
		if strings.Contains(line, "filepath.Base") || strings.Contains(line, "filepath.Clean") {
			continue
		}
		for _, re := range pathTraversalPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "path_traversal_auditor",
					Severity:    SevHigh,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "potential path traversal — file ops with unvalidated var",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "validate path via filepath.Base + filepath.Rel + HasPrefix check",
				})
				break
			}
		}
	}
	return out
}

// =============================================================================
// 5. ssrf_auditor
// =============================================================================

var ssrfPatterns = []*regexp.Regexp{
	regexp.MustCompile(`http\.Get\s*\(\s*\w+\s*\)`),
	regexp.MustCompile(`http\.Post\s*\(\s*\w+`),
	regexp.MustCompile(`http\.NewRequest\s*\([^,]+,\s*\w+\s*,`),
	regexp.MustCompile(`http\.Client\{\}\.Do\s*\(`),
}

func AuditSSRF(filePath, content string) []Finding {
	out := []Finding{}
	for lineNo, line := range strings.Split(content, "\n") {
		// Skip kalau ada SSRF guard hint.
		if strings.Contains(line, "isPrivateIP") || strings.Contains(line, "allowedHosts") ||
			strings.Contains(line, "blocklist") || strings.Contains(line, "IsCloudMetadata") {
			continue
		}
		for _, re := range ssrfPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "ssrf_auditor",
					Severity:    SevHigh,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "potential SSRF — HTTP call with var URL no host whitelist",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "validate host via whitelist + block 169.254.x cloud metadata + private IP ranges",
				})
				break
			}
		}
	}
	return out
}

// =============================================================================
// 6. token_leak_auditor (log + fmt.Print)
// =============================================================================

var tokenLeakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)log\.(Print|Println|Printf|Fatal|Error|Warn).*\b(token|secret|password|key|apiKey)\b`),
	regexp.MustCompile(`(?i)fmt\.(Print|Println|Printf).*\b(token|secret|password|key|apiKey)\b`),
}

func AuditTokenLeak(filePath, content string) []Finding {
	out := []Finding{}
	for lineNo, line := range strings.Split(content, "\n") {
		for _, re := range tokenLeakPatterns {
			if re.MatchString(line) {
				out = append(out, Finding{
					Auditor:     "token_leak_auditor",
					Severity:    SevMedium,
					FilePath:    filePath,
					LineNumber:  lineNo + 1,
					Message:     "potential token/secret leak via log/print",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "mask atau redact secret sebelum log; pakai prefix only (mis. token[:8]+\"...\")",
				})
				break
			}
		}
	}
	return out
}

func truncateSnippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

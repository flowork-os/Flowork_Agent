// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 3 — 10 auditor lagi. Auto-register via init().
//
// auditors_v4.go — 10 auditor:
//   regex_complexity, sha_collision, time_zone, mutex_unlock_missing,
//   panic_in_init, large_struct, http_no_timeout, env_secret_log,
//   sql_concat, json_unmarshal_unchecked.

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["regex_complexity_auditor"]      = AuditRegexComplexity
	Auditors["sha_collision_auditor"]         = AuditSHACollision
	Auditors["time_zone_auditor"]             = AuditTimeZone
	Auditors["mutex_unlock_missing_auditor"]  = AuditMutexUnlockMissing
	Auditors["panic_in_init_auditor"]         = AuditPanicInInit
	Auditors["large_struct_auditor"]          = AuditLargeStruct
	Auditors["http_no_timeout_auditor"]       = AuditHTTPNoTimeout
	Auditors["env_secret_log_auditor"]        = AuditEnvSecretLog
	Auditors["sql_concat_auditor"]            = AuditSQLConcat
	Auditors["json_unmarshal_check_auditor"]  = AuditJSONUnmarshalCheck
}

// =============================================================================
// 1. regex_complexity_auditor — ReDoS risk (nested quantifier)
// =============================================================================

var nestedQuantRE = regexp.MustCompile(`regexp\.(MustCompile|Compile)\s*\(\s*\x60[^\x60]*\([^)]*[+*?][^)]*\)[+*?]`)

func AuditRegexComplexity(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if nestedQuantRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "regex_complexity_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "regex dengan nested quantifier — ReDoS (catastrophic backtracking) potential",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "rewrite ke non-overlapping pattern, atau pakai re2-safe constructs (Go regexp aman tapi NSC tooling beda)",
			})
		}
	}
	return out
}

// =============================================================================
// 2. sha_collision_auditor — sha1/md5 sebagai content integrity hash
// =============================================================================

var contentHashRE = regexp.MustCompile(`(sha1\.New|md5\.New|hmac\.New\(sha1\.New|hmac\.New\(md5\.New)`)

func AuditSHACollision(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if contentHashRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "sha_collision_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "sha1/md5 dipakai untuk hash — collision attack feasible (terutama buat content integrity)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti ke sha256/sha512. Kalau performance, BLAKE2 / xxhash juga OK untuk non-crypto",
			})
		}
	}
	return out
}

// =============================================================================
// 3. time_zone_auditor — time.Now() tanpa eksplisit zone
// =============================================================================

var timeNowLocalRE = regexp.MustCompile(`time\.Now\(\)\.Format`)

func AuditTimeZone(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		// time.Now().Format(...) without explicit .UTC() or .In(tz)
		if timeNowLocalRE.MatchString(line) && !strings.Contains(line, ".UTC()") && !strings.Contains(line, ".In(") {
			out = append(out, Finding{
				Auditor:     "time_zone_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "time.Now().Format(...) tanpa explicit timezone — server-local time, inconsistent across deployments",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai `time.Now().UTC().Format(time.RFC3339)` atau `.In(time.UTC)` untuk konsistensi",
			})
		}
	}
	return out
}

// =============================================================================
// 4. mutex_unlock_missing_auditor — Lock() tanpa defer Unlock()
// =============================================================================

var lockCallRE = regexp.MustCompile(`^\s*(\w+\.)?(mu|m|lock)\.Lock\(\)\s*$`)

func AuditMutexUnlockMissing(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !lockCallRE.MatchString(line) {
			continue
		}
		// Cek 3 line setelahnya untuk `defer ... Unlock` atau `defer ... RUnlock`.
		window := lines[i:minInt(i+5, len(lines))]
		hasDefer := false
		for _, w := range window {
			if strings.Contains(w, "defer") && (strings.Contains(w, "Unlock") || strings.Contains(w, "RUnlock")) {
				hasDefer = true
				break
			}
		}
		if !hasDefer {
			out = append(out, Finding{
				Auditor:     "mutex_unlock_missing_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "Lock() tanpa `defer Unlock()` dalam 4 line — deadlock risk kalau panic atau early return",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "selalu `mu.Lock(); defer mu.Unlock()` segera setelah Lock",
			})
		}
	}
	return out
}

// =============================================================================
// 5. panic_in_init_auditor — init() yang call panic
// =============================================================================

func AuditPanicInInit(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	inInit := false
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "func init()") {
			inInit = true
			depth = 0
		}
		if inInit {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth == 0 && i > 0 {
				inInit = false
				continue
			}
			if strings.HasPrefix(trimmed, "panic(") || strings.Contains(line, " panic(") {
				out = append(out, Finding{
					Auditor:     "panic_in_init_auditor",
					Severity:    SevMedium,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "panic() di func init() — binary fail to start kalau condition trigger",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "init() seharusnya idempotent + safe; kalau setup butuh validation, expose explicit `Init()` function caller bisa handle error",
				})
			}
		}
	}
	return out
}

// =============================================================================
// 6. large_struct_auditor — struct dengan banyak field (>20)
// =============================================================================

var structStartRE = regexp.MustCompile(`type\s+(\w+)\s+struct\s*\{`)

func AuditLargeStruct(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := structStartRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Hitung field sampai closing }.
		depth := 1
		fields := 0
		startLine := i
		for j := i + 1; j < len(lines) && depth > 0; j++ {
			t := strings.TrimSpace(lines[j])
			depth += strings.Count(t, "{") - strings.Count(t, "}")
			if depth == 1 && t != "" && !strings.HasPrefix(t, "//") {
				fields++
			}
		}
		if fields > 25 {
			out = append(out, Finding{
				Auditor:     "large_struct_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  startLine + 1,
				Message:     "struct " + m[1] + " punya " + intToStr(fields) + " field — pertimbangkan split / embed sub-struct",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "extract field cluster ke nested struct: `type X struct { common Common; meta Meta }`",
			})
		}
	}
	return out
}

// =============================================================================
// 7. http_no_timeout_auditor — http.Client{} default tanpa timeout
// =============================================================================

var httpClientRE = regexp.MustCompile(`http\.Client\s*\{\s*\}`)
var httpDefaultRE = regexp.MustCompile(`http\.(Get|Post|Head|PostForm)\s*\(`)

func AuditHTTPNoTimeout(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if httpClientRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "http_no_timeout_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "http.Client{} tanpa Timeout — request bisa hang forever",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "set Timeout: `http.Client{Timeout: 30 * time.Second}`",
			})
		} else if httpDefaultRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "http_no_timeout_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "http.Get/Post default client (no timeout) — pakai client explicit dengan timeout",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "buat client: `c := &http.Client{Timeout: ...}; resp, err := c.Get(url)`",
			})
		}
	}
	return out
}

// =============================================================================
// 8. env_secret_log_auditor — log os.Getenv("XXX_TOKEN/KEY/SECRET")
// =============================================================================

var envSecretLogRE = regexp.MustCompile(`(log\.|fmt\.Print|fmt\.Fprint).*os\.Getenv\s*\(\s*"[A-Z_]*(TOKEN|KEY|SECRET|PASSWORD|API)`)

func AuditEnvSecretLog(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if envSecretLogRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "env_secret_log_auditor",
				Severity:    SevCritical,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "log/print of secret env var — token leak via stdout/log file",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "JANGAN log secret raw. Mask: pakai prefix only (`token[:8]+\"...\"`)",
			})
		}
	}
	return out
}

// =============================================================================
// 9. sql_concat_auditor — fmt.Sprintf di SQL query (selain placeholder ?)
// =============================================================================

var sqlConcatRE = regexp.MustCompile(`(db\.Query|db\.Exec|tx\.Query|tx\.Exec)\s*\(\s*fmt\.Sprintf`)

func AuditSQLConcat(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if sqlConcatRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "sql_concat_auditor",
				Severity:    SevCritical,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "SQL query via fmt.Sprintf — SQL injection risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai parameterized: `db.Query(\"SELECT ... WHERE x = ?\", val)`",
			})
		}
	}
	return out
}

// =============================================================================
// 10. json_unmarshal_check_auditor — json.Unmarshal tanpa err check
// =============================================================================

var unmarshalIgnoreRE = regexp.MustCompile(`^\s*_?\s*=?\s*json\.(Unmarshal|NewDecoder\([^)]+\)\.Decode)\(`)

func AuditJSONUnmarshalCheck(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		// Match `_ = json.Unmarshal(...)` pattern (intentional ignore)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "_ = json.Unmarshal") || strings.HasPrefix(trimmed, "_ = json.NewDecoder") {
			out = append(out, Finding{
				Auditor:     "json_unmarshal_check_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "json.Unmarshal/Decode err ignored — silent parse failure, struct zero values",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "check err: `if err := json.Unmarshal(...); err != nil { return err }`",
			})
		}
	}
	return out
}

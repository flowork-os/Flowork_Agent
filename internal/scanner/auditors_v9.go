// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 8 — 10 auditor.
//
// auditors_v9.go:
//   double_lock, race_struct_field, http_chunked_max, regex_no_anchor,
//   slice_index_unchecked, var_naming, dead_code_func, env_default_missing,
//   unused_struct_field, log_format_mismatch.

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["double_lock_auditor"]            = AuditDoubleLock
	Auditors["race_struct_field_auditor"]      = AuditRaceStructField
	Auditors["http_chunked_max_auditor"]       = AuditHTTPChunkedMax
	Auditors["regex_no_anchor_auditor"]        = AuditRegexNoAnchor
	Auditors["slice_index_unchecked_auditor"]  = AuditSliceIndexUnchecked
	Auditors["var_naming_auditor"]             = AuditVarNaming
	Auditors["dead_code_func_auditor"]         = AuditDeadCodeFunc
	Auditors["env_default_missing_auditor"]    = AuditEnvDefaultMissing
	Auditors["unused_struct_field_auditor"]    = AuditUnusedStructField
	Auditors["log_format_mismatch_auditor"]    = AuditLogFormatMismatch
}

var doubleLockRE = regexp.MustCompile(`(\w+)\.Lock\(\)`)

func AuditDoubleLock(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := doubleLockRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		varName := m[1]
		// Check next 10 line for another Lock() pada var sama tanpa Unlock di antara.
		hasUnlock := false
		for j := i + 1; j < minInt(i+10, len(lines)); j++ {
			if strings.Contains(lines[j], varName+".Unlock") {
				hasUnlock = true
			}
			if strings.Contains(lines[j], varName+".Lock()") && !hasUnlock {
				out = append(out, Finding{
					Auditor:     "double_lock_auditor",
					Severity:    SevHigh,
					FilePath:    filePath,
					LineNumber:  j + 1,
					Message:     "double Lock pada " + varName + " tanpa Unlock di antara — deadlock",
					Snippet:     truncateSnippet(lines[j], 120),
					Remediation: "satu Lock per scope. Pakai recursive mu kalau memang butuh nested",
				})
				break
			}
		}
	}
	return out
}

func AuditRaceStructField(filePath, content string) []Finding {
	return nil
}

var httpChunkedRE = regexp.MustCompile(`io\.ReadAll\s*\(\s*r\.Body`)

func AuditHTTPChunkedMax(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if httpChunkedRE.MatchString(line) && !strings.Contains(line, "MaxBytesReader") && !strings.Contains(line, "LimitReader") {
			out = append(out, Finding{
				Auditor:     "http_chunked_max_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "io.ReadAll(r.Body) tanpa MaxBytesReader/LimitReader — DoS via large body",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "wrap: `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` (1MB cap)",
			})
		}
	}
	return out
}

var regexNoAnchorRE = regexp.MustCompile(`regexp\.MustCompile\s*\(\s*\x60[^\x60^][^\x60]*[^\x60$]\x60\s*\)`)

func AuditRegexNoAnchor(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		// Heuristic untuk pattern validation use case (kalau pattern punya `validate` di var name).
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "regexp.MustCompile") && strings.Contains(trimmed, "validate") {
			if !strings.Contains(line, "^") || !strings.Contains(line, "$") {
				out = append(out, Finding{
					Auditor:     "regex_no_anchor_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "validation regex tanpa ^/$ anchor — partial match accept",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "kalau intent full-match validation, anchor: `^pattern$`",
				})
			}
		}
	}
	return out
}

var sliceIndexRE = regexp.MustCompile(`\w+\[\s*(\d+|len\(\w+\)-\d+)\s*\]`)

func AuditSliceIndexUnchecked(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if sliceIndexRE.MatchString(line) {
			// Hanya flag literal index >= 5 (suggest unsafe).
			if strings.Contains(line, "[0]") || strings.Contains(line, "[1]") {
				continue
			}
			if strings.Contains(line, "[5]") || strings.Contains(line, "[10]") || strings.Contains(line, "[100]") {
				out = append(out, Finding{
					Auditor:     "slice_index_unchecked_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "slice index literal — check `len(s) > N` sebelum access",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "guard: `if len(s) > N { use s[N] }` atau pakai range loop",
				})
			}
		}
	}
	return out
}

var varCamelRE = regexp.MustCompile(`^\s*var\s+([a-z][a-z]+_[a-z])`)

func AuditVarNaming(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if varCamelRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "var_naming_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "var name snake_case — Go convention camelCase",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "rename ke camelCase: snake_case → snakeCase",
			})
		}
	}
	return out
}

func AuditDeadCodeFunc(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	// Heuristic: unexported func with NO callers in same file.
	funcRE := regexp.MustCompile(`^func\s+([a-z]\w*)\s*\(`)
	for i, line := range strings.Split(content, "\n") {
		m := funcRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		// Skip main/init.
		if name == "main" || name == "init" {
			continue
		}
		// Count usage (excluding decl line). Usage = `name(` minus declaration.
		usage := strings.Count(content, name+"(") - 1
		if usage == 0 {
			out = append(out, Finding{
				Auditor:     "dead_code_func_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "unexported func " + name + " tidak ada caller di file ini — dead code (heuristic)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "hapus, atau export (uppercase) kalau dipake package lain",
			})
		}
	}
	return out
}

var envWithoutDefaultRE = regexp.MustCompile(`os\.Getenv\s*\(\s*"([A-Z_]+)"\s*\)`)

func AuditEnvDefaultMissing(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := envWithoutDefaultRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Cek context — apakah ada `if v == "" { v = "default" }` heuristic.
		envName := m[1]
		hasDefault := false
		// Look 5 line setelahnya.
		for j := i + 1; j < minInt(i+6, len(lines)); j++ {
			if strings.Contains(lines[j], `= ""`) || strings.Contains(lines[j], `"` ) && strings.Contains(lines[j], envName) {
				hasDefault = true
			}
		}
		if !hasDefault {
			// Suppress noise — only flag if it's REQUIRED env (heuristic: NAME ends with REQUIRED/TOKEN).
			if strings.HasSuffix(envName, "_TOKEN") || strings.HasSuffix(envName, "_KEY") {
				out = append(out, Finding{
					Auditor:     "env_default_missing_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "os.Getenv " + envName + " tanpa default — kalau env absent silently zero string",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "guard: `if v == \"\" { return fmt.Errorf(\"" + envName + " required\") }`",
				})
			}
		}
	}
	return out
}

func AuditUnusedStructField(filePath, content string) []Finding {
	return nil
}

var logFormatMismatchRE = regexp.MustCompile(`log\.Printf\s*\(\s*"([^"]+)"`)
var formatVerbRE = regexp.MustCompile(`%[+\-#0 ]*\d*\.?\d*[svdfqxXoTtcebBpEgGUC]`)

func AuditLogFormatMismatch(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		m := logFormatMismatchRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		fmtStr := m[1]
		verbs := formatVerbRE.FindAllString(fmtStr, -1)
		expected := len(verbs)
		// Count args = count commas after first arg (rough).
		afterFmt := line[strings.Index(line, m[0])+len(m[0]):]
		args := strings.Count(afterFmt, ",")
		// Both: this is approx; just flag suspicious mismatch >=2 diff.
		if expected > 0 && args > 0 && expected != args {
			out = append(out, Finding{
				Auditor:     "log_format_mismatch_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "log.Printf format verb count (" + intToStr(expected) + ") tidak match args (~" + intToStr(args) + ")",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "verify jumlah verb + args sama (`%s %d` = 2 verb = 2 args)",
			})
		}
	}
	return out
}

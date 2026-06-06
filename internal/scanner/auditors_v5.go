// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 4 — 10 auditor lagi.
//
// auditors_v5.go:
//   tls_min_version, panic_recover_missing, http_redirect_open,
//   xml_external_entity, weak_random, world_writable_perm,
//   logger_concat, race_global_init, channel_no_close,
//   reflect_usage.

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["tls_min_version_auditor"]      = AuditTLSMinVersion
	Auditors["panic_recover_missing_auditor"] = AuditPanicRecoverMissing
	Auditors["http_redirect_open_auditor"]   = AuditHTTPRedirectOpen
	Auditors["xml_external_entity_auditor"]  = AuditXMLExternalEntity
	Auditors["weak_random_auditor"]          = AuditWeakRandom
	Auditors["world_writable_perm_auditor"]  = AuditWorldWritablePerm
	Auditors["logger_concat_auditor"]        = AuditLoggerConcat
	Auditors["race_global_init_auditor"]     = AuditRaceGlobalInit
	Auditors["channel_no_close_auditor"]     = AuditChannelNoClose
	Auditors["reflect_usage_auditor"]        = AuditReflectUsage
}

// =============================================================================
// 1. tls_min_version_auditor — tls.Config tanpa MinVersion
// =============================================================================

var tlsConfigRE = regexp.MustCompile(`&?tls\.Config\s*\{`)

func AuditTLSMinVersion(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !tlsConfigRE.MatchString(line) {
			continue
		}
		// Cek 8 line setelahnya untuk MinVersion.
		window := lines[i:minInt(i+10, len(lines))]
		hasMin := false
		for _, w := range window {
			if strings.Contains(w, "MinVersion") {
				hasMin = true
				break
			}
		}
		if !hasMin {
			out = append(out, Finding{
				Auditor:     "tls_min_version_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "tls.Config tanpa MinVersion — default allow TLS 1.0/1.1 deprecated",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "set `MinVersion: tls.VersionTLS12` (atau VersionTLS13 untuk modern)",
			})
		}
	}
	return out
}

// =============================================================================
// 2. panic_recover_missing_auditor — HTTP handler tanpa recover (panic = crash svr)
// =============================================================================

var httpHandlerRE = regexp.MustCompile(`func\s+\w*\s*\([^)]*\)\s*\(\s*w\s+http\.ResponseWriter\s*,\s*r\s+\*?http\.Request\s*\)`)

func AuditPanicRecoverMissing(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !httpHandlerRE.MatchString(line) {
			continue
		}
		// Window 20 line setelahnya — cari `defer ... recover`
		window := lines[i:minInt(i+20, len(lines))]
		hasRecover := false
		for _, w := range window {
			if strings.Contains(w, "recover()") {
				hasRecover = true
				break
			}
		}
		if !hasRecover {
			out = append(out, Finding{
				Auditor:     "panic_recover_missing_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "HTTP handler tanpa defer recover() — panic = server crash",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "wrap mux dengan middleware: `defer func() { if r := recover(); r != nil { ... } }()`",
			})
		}
	}
	return out
}

// =============================================================================
// 3. http_redirect_open_auditor — Client follows redirect tanpa whitelist
// =============================================================================

var redirectFollowRE = regexp.MustCompile(`(http\.Get|http\.Post|http\.Head|client\.Get|client\.Post)\s*\(`)
var customRedirectRE = regexp.MustCompile(`CheckRedirect`)

func AuditHTTPRedirectOpen(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	hasCustomRedirect := customRedirectRE.MatchString(content)
	if hasCustomRedirect {
		return nil // mitigasi explicit ada
	}
	for i, line := range strings.Split(content, "\n") {
		if redirectFollowRE.MatchString(line) {
			// Only flag kalau URL kelihatannya user-controlled (variable, bukan literal).
			if !strings.Contains(line, `"http`) {
				out = append(out, Finding{
					Auditor:     "http_redirect_open_auditor",
					Severity:    SevMedium,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "HTTP request follows redirect by default — server bisa redirect ke internal/cloud metadata IP",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "set `client.CheckRedirect = func(...) error { return http.ErrUseLastResponse }` atau whitelist",
				})
			}
		}
	}
	return out
}

// =============================================================================
// 4. xml_external_entity_auditor — encoding/xml decode tanpa strict
// =============================================================================

var xmlDecodeRE = regexp.MustCompile(`xml\.(NewDecoder|Unmarshal)\b`)

func AuditXMLExternalEntity(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if xmlDecodeRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "xml_external_entity_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "encoding/xml decode tanpa strict — XXE attack via external entity",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "set `d.Strict = true; d.Entity = nil` sebelum Decode, atau pakai library yg disable DOCTYPE",
			})
		}
	}
	return out
}

// =============================================================================
// 5. weak_random_auditor — math/rand untuk security context
// =============================================================================

var weakRandRE = regexp.MustCompile(`\bmath/rand\b|\brand\.(Int|Intn|Int63|Int31|Float64|Read)\b`)
var cryptoRandRE = regexp.MustCompile(`\bcrypto/rand\b`)

func AuditWeakRandom(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	hasCryptoRand := cryptoRandRE.MatchString(content)
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if !weakRandRE.MatchString(line) {
			continue
		}
		// Skip kalau math/rand digunakan di file yang JUGA import crypto/rand
		// (assume engineer aware konteks).
		if hasCryptoRand {
			continue
		}
		out = append(out, Finding{
			Auditor:     "weak_random_auditor",
			Severity:    SevMedium,
			FilePath:    filePath,
			LineNumber:  i + 1,
			Message:     "math/rand — non-cryptographic. JANGAN dipakai untuk token/secret generation",
			Snippet:     truncateSnippet(line, 120),
			Remediation: "untuk security: pakai crypto/rand. Untuk simulation/test: OK tapi seed eksplisit",
		})
	}
	return out
}

// =============================================================================
// 6. world_writable_perm_auditor — file mode 0777 / 0666
// =============================================================================

var worldWritableRE = regexp.MustCompile(`0o?7[67]7\b|0o?666\b`)

func AuditWorldWritablePerm(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		// Skip kalau dalam comment
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if worldWritableRE.MatchString(line) && (strings.Contains(line, "Chmod") || strings.Contains(line, "MkdirAll") || strings.Contains(line, "OpenFile") || strings.Contains(line, "WriteFile")) {
			out = append(out, Finding{
				Auditor:     "world_writable_perm_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "file/dir mode 0666/0777 — world-writable, security risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ketat: 0o755 untuk dir, 0o644 untuk file, 0o600 untuk secret",
			})
		}
	}
	return out
}

// =============================================================================
// 7. logger_concat_auditor — fmt.Sprintf di log.Println argument
// =============================================================================

var logConcatRE = regexp.MustCompile(`log\.(Print|Println|Fatal|Panic)\s*\(\s*fmt\.Sprintf`)

func AuditLoggerConcat(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if logConcatRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "logger_concat_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "log.Print/Println(fmt.Sprintf(...)) — redundant; pakai log.Printf directly",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti ke `log.Printf(\"format %s\", v)`",
			})
		}
	}
	return out
}

// =============================================================================
// 8. race_global_init_auditor — global var assigned via func call at decl
// =============================================================================

var globalFuncInitRE = regexp.MustCompile(`^var\s+\w+\s*=\s*\w+\(`)

func AuditRaceGlobalInit(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if globalFuncInitRE.MatchString(line) {
			// Skip patterns yang benign: regexp.MustCompile, errors.New, sync.OnceValue
			if strings.Contains(line, "regexp.MustCompile") ||
				strings.Contains(line, "errors.New") ||
				strings.Contains(line, "sync.") ||
				strings.Contains(line, "fmt.Sprintf") ||
				strings.Contains(trimmed, "= []") ||
				strings.Contains(trimmed, "= map[") {
				continue
			}
			out = append(out, Finding{
				Auditor:     "race_global_init_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "global var init via function call — runs sebelum main, error ngga visible",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "move ke func init() dengan err handling, atau lazy init via sync.Once",
			})
		}
	}
	return out
}

// =============================================================================
// 9. channel_no_close_auditor — make(chan) tanpa close di goroutine
// =============================================================================

var chanMakeRE = regexp.MustCompile(`make\(chan\s+\w+\s*,?\s*\d*\)`)

func AuditChannelNoClose(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	// Heuristic only — flag kalau file make chan banyak tapi close jarang.
	makeCount := 0
	closeCount := 0
	for _, line := range strings.Split(content, "\n") {
		if chanMakeRE.MatchString(line) {
			makeCount++
		}
		if strings.Contains(line, "close(") {
			closeCount++
		}
	}
	if makeCount > 2 && closeCount == 0 {
		return []Finding{{
			Auditor:     "channel_no_close_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "file pakai " + intToStr(makeCount) + " channel tanpa close() — receiver bisa block forever",
			Snippet:     "",
			Remediation: "producer harus close() channel saat done; receiver `for v := range ch` exit clean",
		}}
	}
	return nil
}

// =============================================================================
// 10. reflect_usage_auditor — reflect package usage
// =============================================================================

var reflectRE = regexp.MustCompile(`\breflect\.\w+\(`)

func AuditReflectUsage(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	count := 0
	firstLine := 0
	for i, line := range strings.Split(content, "\n") {
		if reflectRE.MatchString(line) {
			count++
			if firstLine == 0 {
				firstLine = i + 1
			}
		}
	}
	if count > 0 {
		return []Finding{{
			Auditor:     "reflect_usage_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  firstLine,
			Message:     "reflect package usage " + intToStr(count) + "x — runtime cost + type-safety bypass",
			Snippet:     "",
			Remediation: "kalau bisa, pakai code-generation atau generics (Go 1.18+) untuk type-safe alternative",
		}}
	}
	return nil
}

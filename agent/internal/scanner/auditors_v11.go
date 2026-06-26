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
	Auditors["tcp_keepalive_auditor"] = AuditTCPKeepalive
	Auditors["websocket_origin_auditor"] = AuditWebsocketOrigin
	Auditors["json_decode_unknownfields_auditor"] = AuditJSONDecodeUnknownFields
	Auditors["long_lived_token_auditor"] = AuditLongLivedToken
	Auditors["archive_path_traversal_auditor"] = AuditArchivePathTraversal
	Auditors["file_overwrite_auditor"] = AuditFileOverwrite
	Auditors["exit_in_lib_auditor"] = AuditExitInLib
	Auditors["missing_error_wrap_auditor"] = AuditMissingErrorWrap
	Auditors["middleware_no_recover_auditor"] = AuditMiddlewareNoRecover
	Auditors["http_no_user_agent_auditor"] = AuditHTTPNoUserAgent
	Auditors["time_truncate_round_auditor"] = AuditTimeTruncateRound
	Auditors["pprof_endpoint_auditor"] = AuditPprofEndpoint
	Auditors["sql_no_limit_auditor"] = AuditSQLNoLimit
}

var tcpDialRE = regexp.MustCompile(`net\.Dial\s*\(`)

func AuditTCPKeepalive(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if tcpDialRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "tcp_keepalive_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "net.Dial — set keepalive untuk detect dead connection",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai net.Dialer{Timeout, KeepAlive} dengan KeepAlive: 30*time.Second",
			})
		}
	}
	return out
}

var wsUpgradeRE = regexp.MustCompile(`websocket\.Upgrader\s*\{`)

func AuditWebsocketOrigin(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !wsUpgradeRE.MatchString(line) {
			continue
		}
		window := lines[i:minInt(i+12, len(lines))]
		hasOriginCheck := false
		for _, w := range window {
			if strings.Contains(w, "CheckOrigin") {
				hasOriginCheck = true
				break
			}
		}
		if !hasOriginCheck {
			out = append(out, Finding{
				Auditor:     "websocket_origin_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "websocket Upgrader tanpa CheckOrigin — cross-origin WS hijack",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "set CheckOrigin func dengan whitelist allowed origins",
			})
		}
	}
	return out
}

var jsonDecoderRE = regexp.MustCompile(`json\.NewDecoder\s*\(`)

func AuditJSONDecodeUnknownFields(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !jsonDecoderRE.MatchString(line) {
			continue
		}
		window := lines[i:minInt(i+5, len(lines))]
		hasStrict := false
		for _, w := range window {
			if strings.Contains(w, "DisallowUnknownFields") {
				hasStrict = true
				break
			}
		}
		if !hasStrict {
			out = append(out, Finding{
				Auditor:     "json_decode_unknownfields_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "json.Decoder tanpa DisallowUnknownFields — typo silent accept",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "panggil `dec.DisallowUnknownFields()` setelah NewDecoder",
			})
		}
	}
	return out
}

var longTokenRE = regexp.MustCompile(`(?i)token.*expiry.*(8760|17520|31536)\s*\*\s*time\.Hour|jwt.*ExpiresAt.*\d{10,}`)

func AuditLongLivedToken(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if longTokenRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "long_lived_token_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "long-lived token (>1 year) — refresh window kelamaan, magnify leak impact",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "token TTL 1-24h ideal, refresh token TTL max 30 day",
			})
		}
	}
	return out
}

var archiveExtractRE = regexp.MustCompile(`(zip|tar)\.NewReader|zip\.OpenReader`)

func AuditArchivePathTraversal(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !archiveExtractRE.MatchString(line) {
			continue
		}

		window := lines[i:minInt(i+30, len(lines))]
		hasGuard := false
		for _, w := range window {
			if strings.Contains(w, "filepath.Rel") || (strings.Contains(w, "HasPrefix") && strings.Contains(w, "..")) {
				hasGuard = true
				break
			}
		}
		if !hasGuard {
			out = append(out, Finding{
				Auditor:     "archive_path_traversal_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "archive extract tanpa path traversal guard — `../etc/passwd` di entry name",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "validate: `if rel, _ := filepath.Rel(root, dest); strings.HasPrefix(rel, \"..\") { reject }`",
			})
		}
	}
	return out
}

func AuditFileOverwrite(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "O_TRUNC") && strings.Contains(line, "OpenFile") {
			out = append(out, Finding{
				Auditor:     "file_overwrite_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "OpenFile O_TRUNC — destructive write, ensure caller mengexpect overwrite",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "consider O_EXCL untuk fail-safe, atau rename pakai .tmp + os.Rename atomic",
			})
		}
	}
	return out
}

var osExitRE = regexp.MustCompile(`os\.Exit\s*\(`)

func AuditExitInLib(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}

	if strings.Contains(content, "package main") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if osExitRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "exit_in_lib_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "os.Exit di package library — kill process, defer cleanup di-skip",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "return error ke caller. os.Exit ONLY di main package",
			})
		}
	}
	return out
}

var unwrappedErrRE = regexp.MustCompile(`fmt\.Errorf\s*\(\s*"[^"]*%v",\s*err\s*\)`)

func AuditMissingErrorWrap(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if unwrappedErrRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "missing_error_wrap_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "fmt.Errorf pakai %v bukan %w — errors.Is/As ngga bisa unwrap",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti %v → %w untuk preserve chain: `fmt.Errorf(\"ctx: %w\", err)`",
			})
		}
	}
	return out
}

func AuditMiddlewareNoRecover(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}

	if !strings.Contains(content, "mux.Handle") && !strings.Contains(content, "Use(") {
		return nil
	}
	if strings.Contains(content, "recover()") {
		return nil
	}
	return []Finding{{
		Auditor:     "middleware_no_recover_auditor",
		Severity:    SevMedium,
		FilePath:    filePath,
		LineNumber:  1,
		Message:     "HTTP server tanpa recover middleware (no recover() di file) — panic crash entire server",
		Snippet:     "",
		Remediation: "wrap mux dengan middleware recover: `defer func() { if r := recover(); r != nil { ... } }()`",
	}}
}

func AuditHTTPNoUserAgent(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}

	out := []Finding{}
	if strings.Contains(content, "http.NewRequest") && !strings.Contains(content, "User-Agent") {

		out = append(out, Finding{
			Auditor:     "http_no_user_agent_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "http.NewRequest tanpa User-Agent — server log = 'Go-http-client/1.1', identifies bot trivially",
			Snippet:     "",
			Remediation: "set custom UA: `req.Header.Set(\"User-Agent\", \"Flowork-Agent/1.0\")`",
		})
	}
	return out
}

var timeTruncateRoundRE = regexp.MustCompile(`time\.Time\{\s*\}`)

func AuditTimeTruncateRound(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if timeTruncateRoundRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "time_truncate_round_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "time.Time{} zero value — kalau dipake sebagai 'never', explicit lebih baik",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai `var t time.Time` atau check `!t.IsZero()` eksplisit",
			})
		}
	}
	return out
}

func AuditPprofEndpoint(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	if strings.Contains(content, "net/http/pprof") {
		out = append(out, Finding{
			Auditor:     "pprof_endpoint_auditor",
			Severity:    SevHigh,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "import net/http/pprof — expose /debug/pprof/* public, profiling endpoint leak",
			Snippet:     "",
			Remediation: "gate dengan internal mux only, atau remove dari production build",
		})
	}
	return out
}

var sqlNoLimitRE = regexp.MustCompile(`SELECT\s+\*\s+FROM\s+\w+(\s|$)`)

func AuditSQLNoLimit(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if sqlNoLimitRE.MatchString(line) && !strings.Contains(strings.ToUpper(line), "LIMIT") {
			out = append(out, Finding{
				Auditor:     "sql_no_limit_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "SELECT * FROM tanpa LIMIT — kalau table besar, OOM/timeout",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "always cap: `SELECT ... FROM x LIMIT N` atau pakai pagination cursor",
			})
		}
	}
	return out
}

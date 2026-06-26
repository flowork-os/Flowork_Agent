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
	Auditors["gosec_bind_all_auditor"] = AuditGosecBindAll
	Auditors["csrf_disable_auditor"] = AuditCSRFDisable
	Auditors["cookie_no_secure_auditor"] = AuditCookieNoSecure
	Auditors["jwt_none_alg_auditor"] = AuditJWTNoneAlg
	Auditors["open_redirect_auditor"] = AuditOpenRedirect
	Auditors["cors_wildcard_auditor"] = AuditCORSWildcard
	Auditors["header_x_forwarded_auditor"] = AuditHeaderXForwarded
	Auditors["password_hash_weak_auditor"] = AuditPasswordHashWeak
	Auditors["yaml_unsafe_auditor"] = AuditYAMLUnsafe
	Auditors["http_basic_auth_auditor"] = AuditHTTPBasicAuth
}

var bindAllRE = regexp.MustCompile(`(ListenAndServe|Listen)\s*\(\s*"0\.0\.0\.0:|:0"`)

func AuditGosecBindAll(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if bindAllRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "gosec_bind_all_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "server bind 0.0.0.0 — exposed di semua interface (cek intentional)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "kalau internal only, bind 127.0.0.1 saja. Public service OK, tapi cek firewall",
			})
		}
	}
	return out
}

var csrfDisableRE = regexp.MustCompile(`(?i)(skipcsrf|nocheckcsrf|csrf.*disable|disable.*csrf)`)

func AuditCSRFDisable(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if csrfDisableRE.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			out = append(out, Finding{
				Auditor:     "csrf_disable_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "CSRF guard di-disable — vulnerable to cross-site request forgery",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "enable CSRF token verification untuk semua state-changing endpoint",
			})
		}
	}
	return out
}

var cookieSetRE = regexp.MustCompile(`http\.Cookie\s*\{`)

func AuditCookieNoSecure(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !cookieSetRE.MatchString(line) {
			continue
		}

		window := lines[i:minInt(i+15, len(lines))]
		hasSecure := false
		hasHttpOnly := false
		for _, w := range window {
			if strings.Contains(w, "Secure:") && strings.Contains(w, "true") {
				hasSecure = true
			}
			if strings.Contains(w, "HttpOnly:") && strings.Contains(w, "true") {
				hasHttpOnly = true
			}
		}
		if !hasSecure || !hasHttpOnly {
			missing := []string{}
			if !hasSecure {
				missing = append(missing, "Secure")
			}
			if !hasHttpOnly {
				missing = append(missing, "HttpOnly")
			}
			out = append(out, Finding{
				Auditor:     "cookie_no_secure_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "http.Cookie tanpa " + strings.Join(missing, "+") + " — session hijack risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "set Secure:true (HTTPS only) + HttpOnly:true (no JS access) untuk session cookie",
			})
		}
	}
	return out
}

var jwtNoneRE = regexp.MustCompile(`"none"|jwt\.SigningMethodNone|SigningMethodNone`)

func AuditJWTNoneAlg(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {

		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if jwtNoneRE.MatchString(line) && (strings.Contains(strings.ToLower(line), "jwt") || strings.Contains(strings.ToLower(line), "token")) {
			out = append(out, Finding{
				Auditor:     "jwt_none_alg_auditor",
				Severity:    SevCritical,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "JWT alg=none accepted — auth bypass via crafted token",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "JANGAN pernah accept alg=none. Whitelist HS256/RS256/ES256 only",
			})
		}
	}
	return out
}

var redirectRE = regexp.MustCompile(`http\.Redirect\s*\([^,]+,\s*[^,]+,\s*(r\.URL\.Query|r\.FormValue|r\.URL\.Path)`)

func AuditOpenRedirect(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if redirectRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "open_redirect_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "http.Redirect target dari user input — phishing redirect bait",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "whitelist target URL atau pakai relative path only; validate dengan url.Parse + host check",
			})
		}
	}
	return out
}

var corsWildcardRE = regexp.MustCompile(`["']\*["']|Access-Control-Allow-Origin.*\*`)

func AuditCORSWildcard(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "Access-Control-Allow-Origin") && corsWildcardRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "cors_wildcard_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "CORS Allow-Origin: * — semua domain bisa fetch credentials",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "whitelist origin spesifik; kalau public API tanpa credentials, lebih OK tapi cek scope",
			})
		}
	}
	return out
}

var xForwardedRE = regexp.MustCompile(`r\.Header\.Get\s*\(\s*"X-Forwarded-(For|Proto|Host)"`)

func AuditHeaderXForwarded(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if xForwardedRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "header_x_forwarded_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "trust X-Forwarded-* tanpa verify reverse proxy whitelist — IP spoofing",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "verify hanya kalau request datang dari trusted reverse proxy (cek r.RemoteAddr)",
			})
		}
	}
	return out
}

var bcryptCostRE = regexp.MustCompile(`bcrypt\.GenerateFromPassword\s*\([^,]+,\s*([0-9]+)\s*\)`)

func AuditPasswordHashWeak(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		m := bcryptCostRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cost := 0
		for _, c := range m[1] {
			cost = cost*10 + int(c-'0')
		}
		if cost < 10 {
			out = append(out, Finding{
				Auditor:     "password_hash_weak_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "bcrypt cost=" + intToStr(cost) + " — too weak, brute-force feasible",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "minimum cost=10 (default bcrypt). Modern: cost=12+",
			})
		}
	}
	return out
}

var yamlUnsafeRE = regexp.MustCompile(`yaml\.(Unmarshal|UnmarshalStrict)\s*\(`)

func AuditYAMLUnsafe(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if yamlUnsafeRE.MatchString(line) && !strings.Contains(line, "UnmarshalStrict") {
			out = append(out, Finding{
				Auditor:     "yaml_unsafe_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "yaml.Unmarshal (non-strict) — accept unknown fields silently",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai yaml.UnmarshalStrict atau set Decoder.KnownFields(true)",
			})
		}
	}
	return out
}

var basicAuthRE = regexp.MustCompile(`SetBasicAuth\s*\(|"Authorization".*Basic`)

func AuditHTTPBasicAuth(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if basicAuthRE.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			out = append(out, Finding{
				Auditor:     "http_basic_auth_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "HTTP Basic Auth — base64 reversible, OK untuk internal, bad untuk public",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "untuk public API: pakai bearer token (OAuth2/JWT) + HTTPS",
			})
		}
	}
	return out
}

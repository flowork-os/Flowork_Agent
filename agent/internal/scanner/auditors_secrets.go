// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

var secretValuePatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"aws-access-key-id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"github-token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"github-pat", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{60,}\b`)},
	{"openai-key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`)},
	{"slack-token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)},
	{"stripe-key", regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{"google-api-key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"telegram-bot-token", regexp.MustCompile(`\b\d{8,10}:[A-Za-z0-9_-]{35}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"private-key-block", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`)},
}

var genericSecretRe = regexp.MustCompile(`(?i)\b(secret|token|passwd|password|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret|auth[_-]?token)\b\s*[:=]+\s*` + "`?" + `"([^"\s]{16,})"`)

var envLookupRe = regexp.MustCompile(`(?i)getenv|os\.environ|process\.env|config\.|flag\.|viper\.|secret_get|GetSecret|\$\{|\$\(`)
var placeholderRe = regexp.MustCompile(`(?i)example|placeholder|your[_-]?|xxxx|changeme|<[a-z]|redacted|dummy|sample|todo|fixme|test[_-]?key`)

var fakeSecretRe = regexp.MustCompile(`(?i)example|fake|realsecret|test[_-]?token|testtoken|placeholder|change[_-]?me|change[_-]?this|dummy|sample|redacted|paste[_-]?your|your[_-]?(key|token|secret)`)

func AuditHardcodedSecretValue(filePath, content string) []Finding {
	var out []Finding
	for i, line := range strings.Split(content, "\n") {
		ln := i + 1
		matched := false
		for _, p := range secretValuePatterns {
			if hit := p.re.FindString(line); hit != "" {

				if fakeSecretRe.MatchString(hit) {
					continue
				}
				out = append(out, Finding{
					Auditor:     "hardcoded_secret_value_auditor",
					Severity:    SevCritical,
					FilePath:    filePath,
					LineNumber:  ln,
					Message:     "hardcoded secret terdeteksi (" + p.name + ") — kredensial nyangkut di source code",
					Snippet:     snippetOf(line),
					Remediation: "pindahin ke env var / secret store; JANGAN commit. Kalau udah ke-commit, ROTATE kredensialnya sekarang.",
				})
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if m := genericSecretRe.FindStringSubmatch(line); m != nil {
			if envLookupRe.MatchString(line) || placeholderRe.MatchString(line) {
				continue
			}
			out = append(out, Finding{
				Auditor:     "hardcoded_secret_value_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  ln,
				Message:     "kemungkinan secret hardcoded (variabel sensitif = string literal)",
				Snippet:     snippetOf(line),
				Remediation: "pakai env var / secret store, jangan hardcode di source.",
			})
		}
	}
	return out
}

func snippetOf(line string) string {
	s := strings.TrimSpace(line)
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

func init() {
	Auditors["hardcoded_secret_value_auditor"] = AuditHardcodedSecretValue
}

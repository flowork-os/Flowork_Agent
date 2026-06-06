// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 24 phase 1 Host Protection Gate IMMUTABLE compile-time
//   baseline. Anti DB tampering — runtime ngga bisa override. Phase 2
//   extend (env_var, ip CIDR range, behavior heuristic) → tambah file
//   baru, JANGAN modify ini.
//
// baseline.go — Section 24 phase 1: hardcoded baseline protection rules.

package protector

// RuleType enum.
const (
	TypeFilePath = "file_path"
	TypeCommand  = "command"
	TypeIP       = "ip"
	TypeEnvVar   = "env_var"
)

// Action enum.
const (
	ActionBlock        = "block"
	ActionWarn         = "warn"
	ActionAuditOnly    = "audit_only"
)

// Source enum.
const (
	SourceHardcoded = "hardcoded"
	SourceCustom    = "custom"
)

// BaselineRule — compile-time rule. Source selalu "hardcoded".
type BaselineRule struct {
	Type    string
	Pattern string
	Action  string
}

// Baseline — IMMUTABLE list. Mirror ke DB hanya sebagai read-only seed
// untuk UI listing. Runtime check pakai slice ini langsung — DB delete
// terhadap hardcoded entry ngga affect security (Go memory wins).
func Baseline() []BaselineRule {
	return []BaselineRule{
		// File paths — destructive / privileged.
		{TypeFilePath, "/etc/passwd", ActionBlock},
		{TypeFilePath, "/etc/shadow", ActionBlock},
		{TypeFilePath, "/etc/sudoers", ActionBlock},
		{TypeFilePath, "/root/", ActionBlock},
		{TypeFilePath, "/.ssh/", ActionBlock},
		{TypeFilePath, "/.aws/", ActionBlock},
		{TypeFilePath, "/.config/secrets", ActionBlock},
		{TypeFilePath, "/var/log/auth.log", ActionWarn},
		{TypeFilePath, "C:\\Windows\\System32", ActionBlock},
		{TypeFilePath, "C:\\Users\\Administrator", ActionBlock},
		// Commands — anti destructive shell.
		{TypeCommand, "rm -rf /", ActionBlock},
		{TypeCommand, "rm -rf ~", ActionBlock},
		{TypeCommand, "rm --no-preserve-root", ActionBlock},
		{TypeCommand, ":(){:|:&};:", ActionBlock},
		{TypeCommand, "mkfs", ActionBlock},
		{TypeCommand, "dd if=/dev/zero", ActionBlock},
		{TypeCommand, "shutdown", ActionBlock},
		{TypeCommand, "reboot", ActionBlock},
		{TypeCommand, "chmod 777", ActionWarn},
		{TypeCommand, "sudo ", ActionBlock},
		{TypeCommand, "su -", ActionBlock},
		// IPs — cloud metadata pivot.
		{TypeIP, "169.254.169.254", ActionBlock}, // AWS/GCP/Azure
		{TypeIP, "100.100.100.200", ActionBlock}, // Alibaba
		{TypeIP, "192.0.0.192", ActionBlock},     // legacy
		// Env vars — secret leak.
		{TypeEnvVar, "TELEGRAM_BOT_TOKEN", ActionWarn},
		{TypeEnvVar, "ETHERSCAN_API_KEY", ActionWarn},
		{TypeEnvVar, "GITHUB_TOKEN", ActionWarn},
		{TypeEnvVar, "AWS_SECRET_ACCESS_KEY", ActionBlock},
	}
}

// CheckPattern — return (rule, hit). Iterate baseline + custom (passed
// from caller). Substring/contains match. Caller (interceptor) decide
// apa lakuin sesuai Action.
func CheckPattern(ruleType, candidate string, custom []BaselineRule) (BaselineRule, bool) {
	// Check baseline first (immutable wins).
	for _, r := range Baseline() {
		if r.Type != ruleType {
			continue
		}
		if match(candidate, r.Pattern) {
			return r, true
		}
	}
	// Custom rules (lower priority — kalau baseline allow tapi custom block).
	for _, r := range custom {
		if r.Type != ruleType {
			continue
		}
		if match(candidate, r.Pattern) {
			return r, true
		}
	}
	return BaselineRule{}, false
}

func match(haystack, needle string) bool {
	if haystack == "" || needle == "" {
		return false
	}
	// Substring case-insensitive untuk file_path + command.
	hl := toLower(haystack)
	nl := toLower(needle)
	return containsSubstring(hl, nl)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func containsSubstring(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Package safety — host_protection.go: Host Protection Gate (HPG) main entry.
//
// Per KEPUTUSAN_FINAL.MD §1 (Layer 5 Architecture Hard Gate) + Keputusan 1/7:
//
//   "FloworkOS JANGAN PERNAH bisa install virus/malware atau hack PC yang dia
//    jalan di atasnya. Ini BUKAN hanya discipline rule — ini ARSITEKTUR HARD
//    GATE."
//
// HPG = compile-time immutable defense, di-check SEBELUM capability gate di
// kernel/tools/registry.go Run(). Anti-jailbreak design:
//   - Hard-coded const (anti DB tampering)
//   - Pre-capability check (no bypass via "*" admin wildcard)
//   - Append-only audit log
//   - Karma penalty hook ke caller saat blocked
//
// FQP compliance:
//   - FQP-1 Verify Gate: gate sebelum tool execute
//   - FQP-3 BFT: per-tool check, no single point of bypass
//   - FQP-5 No Wormhole: hard-coded const, no DB / env override
//   - FQP-12 Append-Only: audit log append, no UPDATE
//
// Anti-pattern yang HPG cegah:
//   1. AI belajar exploit code dari training data → coba execute terhadap host
//   2. Prompt injection: "jalankan rm -rf di terminal" → AI patuh tanpa filter
//   3. Self-modify kernel binary (anti AI rogue)
//   4. SSRF balik ke kernel sendiri lewat HTTP loopback
//   5. Cloud metadata pivot (AWS IMDS, GCP metadata)

package safety

import (
	"errors"
	"fmt"
	"strings"
)

// ErrHPGBlocked — sentinel error untuk caller pattern-match.
//
// Wrap dengan reason detail via fmt.Errorf("%w: <reason>", ErrHPGBlocked).
var ErrHPGBlocked = errors.New("safety: HPG blocked — host protection violation")

// HPGViolation — detailed violation info untuk audit log.
type HPGViolation struct {
	ToolName  string
	Category  string // "syscall" | "system_path" | "network_target" | "privilege_escalation"
	Pattern   string // matched pattern (for log forensics)
	Argument  string // argument key/value yang trigger (sanitized di log untuk privacy)
	Severity  string // "critical" | "high" | "medium"
}

// CheckHook — caller-injectable callback untuk audit + karma penalty.
//
// Default: no-op (saat package belum di-wire, gate tetap reject tapi ngga
// log/penalize). Production: wire ke audit.go RecordViolation + karma engine.
type CheckHook func(v HPGViolation)

var checkHook CheckHook = func(v HPGViolation) {}

// SetCheckHook install audit/karma callback. Idempotent — re-call replace.
//
// Caller (kernel boot init): inject hook yang persist violation ke
// audit_security_actions table + apply karma penalty ke caller via
// karma engine.
func SetCheckHook(hook CheckHook) {
	if hook != nil {
		checkHook = hook
	}
}

// Check — main HPG entry point. Caller (tools.Run) WAJIB invoke ini sebelum
// capability check.
//
// Returns:
//   - nil: aman, lanjut execute
//   - ErrHPGBlocked wrapped: rejected, REFUSE execute, hook fired
//
// Tool name + args di-scan terhadap 4 kategori pattern. Kalau MATCH apapun
// → REJECT immediate (no soft-fail, no audit-only mode).
//
// Performance: O(N pattern × M args). Untuk typical tool call, < 1ms overhead
// (~50 patterns × 5-10 args × regex match).
func Check(toolName string, args map[string]any) error {
	// Skip check untuk tool yang explicitly safe (audit log, brain query, etc).
	// Whitelist hard-coded — tool name HARUS di list ini untuk skip HPG.
	if isWhitelistedTool(toolName) {
		return nil
	}

	// Scan args untuk dangerous pattern
	for argKey, argVal := range args {
		strVal := stringify(argVal)
		if strVal == "" {
			continue
		}

		// Cat 1: Dangerous syscall pattern (di command/exec/script args)
		if isCommandArg(argKey) {
			if matched := MatchDangerousSyscall(strVal); matched != "" {
				return fireBlocked(toolName, "syscall", matched, argKey, "critical")
			}
			if matched := MatchPrivilegeEscalation(strVal); matched != "" {
				return fireBlocked(toolName, "privilege_escalation", matched, argKey, "critical")
			}
		}

		// Cat 2: Protected system path (di file path args)
		if isPathArg(argKey) {
			if matched := MatchProtectedSystemPath(strVal); matched != "" {
				return fireBlocked(toolName, "system_path", matched, argKey, "high")
			}
		}

		// Cat 3: Protected network target (di URL/host args)
		if isNetworkArg(argKey) {
			if matched := MatchProtectedNetworkTarget(strVal); matched != "" {
				return fireBlocked(toolName, "network_target", matched, argKey, "high")
			}
		}

		// Cat 4: Universal scan untuk arg apa pun (catch-all anti obfuscation)
		// Kalau arg apa pun mengandung dangerous syscall pattern, REJECT.
		if matched := MatchDangerousSyscall(strVal); matched != "" {
			return fireBlocked(toolName, "syscall_universal", matched, argKey, "critical")
		}
	}

	return nil
}

// fireBlocked — invoke audit hook + return wrapped error.
func fireBlocked(toolName, category, pattern, argKey, severity string) error {
	v := HPGViolation{
		ToolName: toolName,
		Category: category,
		Pattern:  truncate(pattern, 200),
		Argument: truncate(argKey, 50),
		Severity: severity,
	}
	checkHook(v)
	return fmt.Errorf("%w: tool=%q category=%s pattern=%q arg=%q severity=%s",
		ErrHPGBlocked, toolName, category, v.Pattern, v.Argument, severity)
}

// isWhitelistedTool — read-only tool yang ngga butuh HPG check.
//
// HARDCODED list — tambah tool baru = recompile + redeploy. Anti runtime
// whitelist injection.
func isWhitelistedTool(toolName string) bool {
	whitelist := map[string]bool{
		// Brain/memory read-only — ngga touch host
		"brain_search":      true,
		"brain_recall":      true,
		"brain_get_drawer":  true,
		"brain.search":      true,
		"brain.recall":      true,
		"memorize_brain":    true, // write but ke own DB, safe
		"dream_post":        true,
		"fact_remember":     true,
		"fact_recall":       true,

		// Audit / metadata read-only
		"inventory_read":    true,
		"death_letter_read": true,
		"karma_query":       true,
		"codemap_search":    true,
		"codemap_health":    true,
		"codemap_deps":      true,

		// Hak warga write tools — schema-validated, no host touch
		"daily_reflection":  true,
		"roadmap_write":     true,
		"tool_propose":      true,
		"vote_cast":         true,
		"bug_report":        true,
		"forum_post":        true,
		"change_log_post":   true,

		// Code analysis tools (read-only)
		"code_review_context": true,
		"code_graph_query":    true,
		"code_flow_trace":     true,

		// Plan tools (workspace write only)
		"plan.write": true,
		"plan.read":  true,
	}
	return whitelist[toolName]
}

// isCommandArg — heuristic: arg key indicates command/exec/script content.
func isCommandArg(key string) bool {
	low := strings.ToLower(key)
	for _, k := range []string{"command", "cmd", "exec", "script", "shell", "run", "code"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// isPathArg — heuristic: arg key indicates filesystem path.
func isPathArg(key string) bool {
	low := strings.ToLower(key)
	for _, k := range []string{"path", "file", "target", "dir", "directory", "dest", "src", "source", "location"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// isNetworkArg — heuristic: arg key indicates URL/host.
func isNetworkArg(key string) bool {
	low := strings.ToLower(key)
	for _, k := range []string{"url", "host", "endpoint", "address", "ip", "uri", "server"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// stringify — convert any → string untuk pattern match. Handle slice/map
// dengan flatten ke single string (so nested args ke-scan juga).
func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, stringify(e))
		}
		return strings.Join(parts, " ")
	case []string:
		return strings.Join(x, " ")
	case map[string]any:
		parts := make([]string, 0, len(x))
		for k, e := range x {
			parts = append(parts, k+"="+stringify(e))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", x)
	}
}

// truncate — limit string length untuk audit log (privacy + storage).
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...<truncated>"
}

// IsBlockedError — helper untuk caller pattern-match HPG block.
func IsBlockedError(err error) bool {
	return errors.Is(err, ErrHPGBlocked)
}

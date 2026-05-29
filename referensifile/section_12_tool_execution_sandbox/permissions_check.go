package tools

import "strings"

// parsePermissionRule parses "Bash(npm:*)" → PermissionRule.
func parsePermissionRule(s, action string) PermissionRule {
	s = strings.TrimSpace(s)
	idx := strings.Index(s, "(")
	if idx < 0 {
		return PermissionRule{Tool: strings.ToLower(s), Action: action}
	}
	tool := strings.ToLower(s[:idx])
	pattern := strings.TrimRight(s[idx+1:], ")")
	return PermissionRule{Tool: tool, Pattern: pattern, Action: action}
}

// checkSettingsRules returns "allow"/"deny"/"ask"/"" (no rule matched).
//
// Priority order (Gemini audit fix Bug 3.1): deny ALWAYS beats allow.
// Previously the code iterated rules in load order (allow before deny)
// and returned on first match — so a broad `allow: ["bash"]` would swallow
// a more specific `deny: ["bash(rm -rf*)"]`. The new semantics:
//
//  1. If any deny rule matches → deny (immediately).
//  2. Else if any ask rule matches → ask.
//  3. Else if any allow rule matches → allow.
//  4. Otherwise "" (no explicit rule).
//
// Within the same action category, a more-specific pattern still wins
// over an empty pattern by nature of the loop checking Pattern != "" first.
func checkSettingsRules(inv *Invocation) string {
	settingsRulesMu.RLock()
	rules := settingsRules
	settingsRulesMu.RUnlock()

	matched := func(action string) bool {
		// First pass: patterned rules of this action
		for _, r := range rules {
			if r.Action != action {
				continue
			}
			if !strings.EqualFold(r.Tool, inv.ToolName) {
				continue
			}
			if r.Pattern != "" && rulePatternMatches(r.Pattern, inv) {
				return true
			}
		}
		// Second pass: bare (no-pattern) rules of this action — catch-all for tool.
		for _, r := range rules {
			if r.Action != action {
				continue
			}
			if !strings.EqualFold(r.Tool, inv.ToolName) {
				continue
			}
			if r.Pattern == "" {
				return true
			}
		}
		return false
	}

	if matched("deny") {
		return "deny"
	}
	if matched("ask") {
		return "ask"
	}
	if matched("allow") {
		return "allow"
	}
	return ""
}

// rulePatternMatches checks if the invocation argument matches the pattern.
// Pattern "npm:*" matches bash commands starting with "npm".
// Pattern "/etc/*" matches path arguments under /etc/.
func rulePatternMatches(pattern string, inv *Invocation) bool {
	if inv.ParsedArgs == nil {
		return false
	}
	// For bash, check "command"; for file tools check "path".
	var arg string
	if cmd, ok := inv.ParsedArgs["command"].(string); ok {
		arg = cmd
	} else if path, ok := inv.ParsedArgs["path"].(string); ok {
		arg = path
	} else {
		return false
	}

	// CODEX-BUG-11 fix: "Bash(npm:*)" used to match "npmx install" because
	// HasPrefix ignored what came *after* the prefix. Likewise
	// "Write(/tmp/*)" matched "/tmp2/foo.log". Now the character immediately
	// after the prefix must be a recognised boundary — whitespace, path
	// separator, end-of-string, or a shell continuation token — so a prefix
	// cannot masquerade as a sibling token.

	// Pattern "prefix:*" — arg must start with prefix AND end there or
	// continue into a word boundary (" ", "\t", or end).
	if idx := strings.Index(pattern, ":*"); idx >= 0 {
		prefix := pattern[:idx]
		trimmed := strings.TrimSpace(arg)
		if !strings.HasPrefix(trimmed, prefix) {
			return false
		}
		tail := trimmed[len(prefix):]
		if tail == "" {
			return true
		}
		switch tail[0] {
		case ' ', '\t', ';', '|', '&':
			return true
		}
		return false
	}
	// Pattern ending in "/*" — path prefix match constrained to a real
	// directory boundary so "/tmp/*" doesn't match "/tmp2/foo".
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if !strings.HasPrefix(arg, prefix) {
			return false
		}
		tail := arg[len(prefix):]
		if tail == "" {
			return true
		}
		return tail[0] == '/' || tail[0] == '\\'
	}
	// Exact match fallback.
	return arg == pattern
}

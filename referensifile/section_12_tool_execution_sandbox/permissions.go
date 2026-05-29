package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	braindb "github.com/teetah2402/flowork/brain/db"
)

// daemonDeniedError wrap denial dengan educational message kalau tool ditolak
// karena no-TTY daemon mode. rc164 fix: Merpati di Telegram cuma liat
// "user denied 'bash'" tanpa context — sekarang dapat panduan: lapor ke Ayah
// supaya set FLOW_DAEMON_POLICY=allow.
//
// Kalau workspace ga ke-detect (FLOWORK_WORKSPACE empty) → fallback generic.
func daemonDeniedError(toolName string) error {
	ws := strings.TrimSpace(os.Getenv("FLOWORK_WORKSPACE"))
	if ws == "" {
		return fmt.Errorf("user denied %q (daemon no-TTY; minta Ayah set FLOW_DAEMON_POLICY=allow di .env)", toolName)
	}
	msg := braindb.GetEducationalError(ws, "ERR_PERMISSION_DENIED_DAEMON", toolName)
	return fmt.Errorf("%s", msg)
}

// PermissionMode — controls how potentially-dangerous tool calls are gated.
type PermissionMode string

const (
	// PermissionDefault — prompt user for write/exec ops; allow reads.
	PermissionDefault PermissionMode = "default"
	// PermissionAcceptEdits — auto-allow file edits; prompt for bash/exec.
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	// PermissionBypass — allow everything (dangerous).
	PermissionBypass PermissionMode = "bypassPermissions"
	// PermissionPlan — read-only mode (no writes, no exec).
	PermissionPlan PermissionMode = "plan"
)

var currentMode PermissionMode = PermissionDefault
var modeMu sync.RWMutex

func SetPermissionMode(m PermissionMode) {
	modeMu.Lock()
	currentMode = m
	modeMu.Unlock()
}

func CurrentPermissionMode() PermissionMode {
	modeMu.RLock()
	defer modeMu.RUnlock()
	return currentMode
}

// PermissionInterceptor — blocks tool execution that violates current mode.
// Insert as the FIRST interceptor in registry.
type PermissionInterceptor struct{}

var writeTools = map[string]bool{
	"write":        true,
	"edit":         true,
	"multiedit":    true,
	"notebookedit": true,
}

var execTools = map[string]bool{
	"bash":     true,
	"mcp_call": true, // may have side effects
}

func (p PermissionInterceptor) Before(ctx context.Context, invocation *Invocation) error {
	// Check explicit rules from settings.json first (highest priority).
	if action := checkSettingsRules(invocation); action != "" {
		switch action {
		case "allow":
			return nil
		case "deny":
			return fmt.Errorf("denied by permission rule: %q", invocation.ToolName)
		case "ask":
			if !promptApprove(invocation) {
				return fmt.Errorf("user denied %q (ask rule)", invocation.ToolName)
			}
			return nil
		default:
			// no-op — exhaustive switch guard
		}
	}

	mode := CurrentPermissionMode()
	switch mode {
	case PermissionBypass:
		return nil
	case PermissionPlan:
		if writeTools[invocation.ToolName] || execTools[invocation.ToolName] {
			return fmt.Errorf("plan mode: tool %q is forbidden (read-only mode)", invocation.ToolName)
		}
		return nil
	case PermissionAcceptEdits:
		// allow writes, gate exec
		if execTools[invocation.ToolName] {
			if !promptApprove(invocation) {
				if !isInteractiveStdin() {
					return daemonDeniedError(invocation.ToolName)
				}
				return fmt.Errorf("user denied execution of %q", invocation.ToolName)
			}
		}
		return nil
	default:
		// PermissionDefault — prompt for writes + exec
		if writeTools[invocation.ToolName] || execTools[invocation.ToolName] {
			if !promptApprove(invocation) {
				if !isInteractiveStdin() {
					return daemonDeniedError(invocation.ToolName)
				}
				return fmt.Errorf("user denied %q", invocation.ToolName)
			}
		}
		return nil
	}
}

func (p PermissionInterceptor) After(ctx context.Context, invocation Invocation, result *Result, err error) {
}

// promptApprove — interactive y/N prompt via stderr/stdin.
// Auto-allows when FLOW_BYPASS_PROMPT=1 (test mode) or tool already in allowlist.
//
// Gemini audit fix (Bug 3.2): when running as daemon (flowork-chat/mesh/
// telegram with no attached TTY), stdin is closed — Fscanln returns
// immediately with EOF, old code returned false and silently blocked every
// write/exec. Now: if non-TTY, we honor FLOW_DAEMON_POLICY env var
// (default "deny") instead of relying on a doomed Scanln.
func promptApprove(inv *Invocation) bool {
	if os.Getenv("FLOW_BYPASS_PROMPT") == "1" {
		return true
	}
	if sessionAllowed(inv) {
		return true
	}
	if !isInteractiveStdin() {
		switch strings.ToLower(os.Getenv("FLOW_DAEMON_POLICY")) {
		case "allow":
			return true
		case "":
			// No explicit policy: fail closed but log the reason so daemons
			// don't mysteriously stall.
			fmt.Fprintf(os.Stderr, "flowork: tool %q denied (no TTY, set FLOW_DAEMON_POLICY=allow to whitelist)\n", inv.ToolName)
		}
		return false
	}
	preview := ""
	if inv.ParsedArgs != nil {
		if cmd, ok := inv.ParsedArgs["command"].(string); ok {
			preview = " » " + cmd
		} else if path, ok := inv.ParsedArgs["path"].(string); ok {
			preview = " » " + path
		}
	}
	fmt.Fprintf(os.Stderr, "\n⚠️  Allow %s%s ? [y/N/a=always-this-command (hash-lock)] ", inv.ToolName, preview)
	var reply string
	_, _ = fmt.Fscanln(os.Stdin, &reply)
	reply = strings.ToLower(strings.TrimSpace(reply))
	switch reply {
	case "y", "yes":
		return true
	case "a", "always":
		_ = sessionAllow(inv.ToolName, hashInvocation(inv))
		return true
	default:
		return false
	}
}

// ─── Permission rule syntax ─────────────────────────────────────
// Supports: "Bash(npm:*)", "Edit", "Write(/etc/*)", "Bash(rm -rf:*)"
// Loaded from settings.json → permissions.allow / .deny / .ask
// (rule check + pattern match: see permissions_check.go)
// (session allowlist + caching: see permissions_session.go)

// PermissionRule is a parsed rule from settings.json.
type PermissionRule struct {
	Tool    string // lowercase tool name, e.g. "bash"
	Pattern string // optional argument prefix/glob, e.g. "npm:*"
	Action  string // "allow" | "deny" | "ask"
}

var (
	settingsRules   []PermissionRule
	settingsRulesMu sync.RWMutex
)

// LoadPermissionRules parses allow/deny/ask arrays from settings.json at all 3 levels:
//
//	.flowork/settings.local.json > .flowork/settings.json > ~/.flowork/settings.json
func LoadPermissionRules(workspace string) {
	type ruleSet struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
		Ask   []string `json:"ask"`
	}
	type settingsFile struct {
		Permissions ruleSet `json:"permissions" validate:"required"`
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(workspace, ".flowork", "settings.local.json"), // highest priority
		filepath.Join(workspace, ".flowork", "settings.json"),
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".flowork", "settings.json"))
	}

	seen := make(map[string]bool) // tool+pattern dedup (first wins)
	var rules []PermissionRule

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			// Missing settings files are expected (each level is optional).
			// Unexpected IO failure (permission denied, symlink loop, ...)
			// is worth surfacing so the user isn't left wondering why their
			// rules aren't applied.
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "permissions: cannot read %s: %v (rules from this file NOT loaded)\n", path, err)
			}
			continue
		}
		var sf settingsFile
		if err := json.Unmarshal(data, &sf); err != nil {
			// Kategori 4 (silent failure): a typo in settings.json used to
			// drop every allow/deny/ask rule from this layer without a
			// peep, so a user who wrote `deny: ["Bash(rm:*)"]` could end
			// up running with permissive defaults. Log loudly so a
			// malformed file is visible before the next tool call.
			fmt.Fprintf(os.Stderr, "permissions: %s is not valid JSON: %v (rules from this file NOT loaded)\n", path, err)
			continue
		}
		for _, s := range sf.Permissions.Allow {
			r := parsePermissionRule(s, "allow")
			key := r.Tool + "|" + r.Pattern
			if !seen[key] {
				seen[key] = true
				rules = append(rules, r)
			}
		}
		for _, s := range sf.Permissions.Deny {
			r := parsePermissionRule(s, "deny")
			key := r.Tool + "|" + r.Pattern + "|deny"
			if !seen[key] {
				seen[key] = true
				rules = append(rules, r)
			}
		}
		for _, s := range sf.Permissions.Ask {
			r := parsePermissionRule(s, "ask")
			key := r.Tool + "|" + r.Pattern + "|ask"
			if !seen[key] {
				seen[key] = true
				rules = append(rules, r)
			}
		}
	}

	settingsRulesMu.Lock()
	settingsRules = rules
	settingsRulesMu.Unlock()
}

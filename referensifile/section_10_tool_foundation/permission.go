// Package tools — granular per-arg permission gate (Tier 1.5 Foundation).
//
// Pattern: extension interface optional. Tool yang implement PermissionAware
// dapat veto per-call basis (e.g. bash tool veto kalau args["command"] match
// destructive pattern, allow kalau "ls"). Default tools tanpa interface ini
// = behavior backward-compat sesuai capabilityMap (flat all-or-nothing).
//
// Replaces flat FLOW_DAEMON_POLICY env-var pattern (binary on/off) dengan
// runtime per-arg evaluation. Mirror Anthropic Claude Code "ask permission"
// model tapi adapted untuk warga AI (BUKAN human user) — kernel sebagai
// gatekeeper, warga sebagai actor, capability map + per-arg = 2-layer defense.
//
// Why optional (not mandated): refactor 134 tools = breaking change.
// Adapter pattern — old tools tetap jalan via capabilityMap, new tools opt-in
// granular by implementing PermissionAware. Migration path: tool-by-tool
// upgrade ke PermissionAware kalau butuh per-arg gate (bash, exec, browser_eval,
// dll). Tool yang ngga butuh (read, list_files) skip — overhead-free.
//
// Caller pattern (registry dispatch):
//
//	rawTool, _ := tools.Get(name)
//	if perm, ok := rawTool.(PermissionAware); ok {
//	    result := perm.NeedsPermission(args)
//	    if result.Behavior == PermDeny {
//	        return nil, fmt.Errorf("denied: %s", result.Reason)
//	    }
//	    if result.Behavior == PermAsk {
//	        // future: surface ke gate handler (forum/Ayah-via-telegram).
//	        // Phase 1: log warning + allow (audit-only mode).
//	        log.Printf("[permission] tool=%s asks: %s", name, result.Reason)
//	    }
//	}

package tools

// PermissionBehavior — outcome behavior dari NeedsPermission check.
type PermissionBehavior int

const (
	// PermAllow — proceed without gate.
	PermAllow PermissionBehavior = iota
	// PermAsk — surface ke external gate (forum decision / Ayah / Teguh proxy).
	// Phase 1 implementation: log + allow (audit-only mode). Phase 2 extend
	// dengan actual gate handler.
	PermAsk
	// PermDeny — reject call. Tool BUKAN unavailable, args spesifik this call
	// yang denied. Warga bisa retry dengan args berbeda.
	PermDeny
)

// PermissionResult — return value dari PermissionAware.NeedsPermission.
//
// SuggestPatch optional: tool kasih hint args alternative yang safe. Mis.
// bash deny "rm -rf /" suggest patch {"command": "rm -rf /tmp/specific-path"}.
// Helps warga self-correct tanpa retry blind.
type PermissionResult struct {
	Behavior     PermissionBehavior
	Reason       string
	SuggestPatch map[string]any
}

// Allow — helper construct allow result. Reason optional (audit log).
func Allow(reason string) PermissionResult {
	return PermissionResult{Behavior: PermAllow, Reason: reason}
}

// Ask — helper construct ask result. Reason wajib (gate handler context).
func Ask(reason string) PermissionResult {
	return PermissionResult{Behavior: PermAsk, Reason: reason}
}

// Deny — helper construct deny result. Reason wajib + optional suggest patch.
func Deny(reason string, suggest map[string]any) PermissionResult {
	return PermissionResult{Behavior: PermDeny, Reason: reason, SuggestPatch: suggest}
}

// PermissionAware — extension interface optional. Tool yang implement dapat
// veto per-call dengan args context awareness.
//
// Implementation pattern (lihat kernel/tools/execution/bash_permission.go nanti
// post Step 6 wiring):
//
//	func (t *bashTool) NeedsPermission(args map[string]any) PermissionResult {
//	    cmd, _ := args["command"].(string)
//	    if matchesDestructive(cmd) {
//	        return Deny("destructive pattern detected: rm -rf | format | dd",
//	                    map[string]any{"command": "<safe-alternative>"})
//	    }
//	    if matchesNetworkExfil(cmd) {
//	        return Ask("network exfil pattern: curl + secret env-var")
//	    }
//	    return Allow("")
//	}
//
// Tool tanpa implement interface ini = default Allow (backward compat).
type PermissionAware interface {
	NeedsPermission(args map[string]any) PermissionResult
}

// EvalPermission — helper untuk caller (registry dispatch). Type-assert tool
// ke PermissionAware, eval kalau implement, otherwise return default Allow.
//
// Pure function — no side effects, no logging. Caller decide apa yang dilakuin
// dengan result (log audit, surface to gate handler, abort dispatch, dll).
func EvalPermission(tool any, args map[string]any) PermissionResult {
	if perm, ok := tool.(PermissionAware); ok {
		return perm.NeedsPermission(args)
	}
	return Allow("no PermissionAware interface; default allow")
}

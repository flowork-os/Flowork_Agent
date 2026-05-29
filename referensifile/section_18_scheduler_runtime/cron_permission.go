// Package cron — kernel/tools/cron/cron_permission.go
//
// Tier 1.5 PermissionAware untuk addTool, removeTool, triggerTool.
//
// Why: cron adalah persistent state mutation. Add cron = bikin scheduled
// trigger yang jalan unattended. Remove cron = bisa kena infrastructure cron
// (daemon heartbeat, brain maintenance, dll). Trigger = manual fire bisa expose
// destructive payload.
//
// Strategy:
// - addTool: ASK kalau expression terlalu agresif (every minute), atau action
//   memanggil destructive tool. ALLOW reasonable schedules.
// - removeTool: ASK kalau ID match pattern infrastructure cron.
// - triggerTool: ASK kalau ID match infrastructure cron (manual fire harus
//   audit-able).

package cron

import (
	"regexp"
	"strings"

	"github.com/flowork/kernel/kernel/tools"
)

// Infrastructure cron ID prefix yang penting (heartbeat, maintenance, audit).
var infraCronPrefixes = []string{
	"sys.",
	"daemon.",
	"heartbeat.",
	"brain.maintenance",
	"karma.audit",
	"watchdog.",
	"bft.",
}

// Cron expressions terlalu agresif — every second/minute level.
var aggressiveExpressions = []*regexp.Regexp{
	regexp.MustCompile(`^\*\s+\*\s+\*\s+\*\s+\*$`),  // * * * * * (every minute)
	regexp.MustCompile(`^\*/[12]\s+\*\s+\*\s+\*\s+\*$`), // */1 or */2 * * * * (every 1-2 minutes)
	regexp.MustCompile(`^@(every|secondly)\s+1?s$`),    // every 1s
}

// Destructive tool names yang kalau di-schedule = persistent risk.
var destructiveActionTools = map[string]bool{
	"execution.bash":   true,
	"execution.sandbox": true,
	"bash":             true, // alias
	"shell":            true,
}

func matchesInfraCron(id string) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	for _, prefix := range infraCronPrefixes {
		if strings.HasPrefix(idLower, prefix) {
			return true
		}
	}
	return false
}

// NeedsPermission untuk addTool.
func (t *addTool) NeedsPermission(args map[string]any) tools.PermissionResult {
	expr, _ := args["expression"].(string)
	expr = strings.TrimSpace(expr)

	// Layer 1: too aggressive — ASK (audit potential resource abuse)
	for _, p := range aggressiveExpressions {
		if p.MatchString(expr) {
			return tools.Ask("cron expression too aggressive: " + expr + " (every minute or faster)")
		}
	}

	// Layer 2: destructive action — ASK (scheduled bash = persistent risk)
	if action, ok := args["action"].(map[string]any); ok {
		if actionType, _ := action["type"].(string); actionType == "tool" {
			if toolName, _ := action["tool"].(string); destructiveActionTools[strings.ToLower(toolName)] {
				return tools.Ask("scheduling destructive tool: " + toolName + " — review payload for persistent risk")
			}
		}
	}

	return tools.Allow("")
}

// NeedsPermission untuk removeTool.
func (t *removeTool) NeedsPermission(args map[string]any) tools.PermissionResult {
	id, _ := args["id"].(string)
	if matchesInfraCron(id) {
		return tools.Ask("removing infrastructure cron: " + id + " (system heartbeat/maintenance/audit)")
	}
	return tools.Allow("")
}

// NeedsPermission untuk triggerTool.
func (t *triggerTool) NeedsPermission(args map[string]any) tools.PermissionResult {
	id, _ := args["id"].(string)
	if matchesInfraCron(id) {
		return tools.Ask("manually triggering infrastructure cron: " + id + " — bypass schedule audit")
	}
	return tools.Allow("")
}

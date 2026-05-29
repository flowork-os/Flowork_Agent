// Package subagent — kernel-side slash command dispatcher.
//
// Mr.Dev mandate 2026-05-21: 12 slash command per arch diagram
// (/skills /tasks /agents /plan /compact /memory /doctor /config
//  /mcp /worktree /cost /resume).
//
// Phase 1 implementation: parse literal slash → map ke action acknowledge.
// Phase 2 (TODO): actual dispatch ke real action handler per slash.
//
// Per [[feedback-tool-naming-lockin-mutlak]] 2026-05-12: tool name
// `SlashCommand` PascalCase (Claude-Code convention) tetap ALIAS via
// aliases.go ke `slash_command` lowercase canonical.

package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowork/kernel/kernel/tools"
)

const ToolSlashName = "slash_command"

// slashRoutes — map slash literal ke action handler description.
// Phase 1 ACK-stub only: return mapping. Phase 2 wire real handler.
var slashRoutes = map[string]string{
	"/skills":   "List + manage skill registry. Use `skill_search` atau `tool_search query=skill`.",
	"/tasks":    "Show task board. Use `task_list` for items.",
	"/agents":   "List active subagent / spawn new. Use `agent_task_list` atau `delegate_task subagent_type=X`.",
	"/plan":     "Enter plan mode (no writes/execution). Use `enter_plan_mode` tool.",
	"/compact":  "Compact conversation history. Use `snip` tool.",
	"/memory":   "Show/edit memory. Use `memory_get/set/delete` tools.",
	"/doctor":   "Health check daemons. Use `bash command='curl http://localhost:3105/healthz'`.",
	"/config":   "Show settings DB config. Use `config` tool.",
	"/mcp":      "MCP server status. Use `mcp_list_resources` tool.",
	"/worktree": "Git worktree management. Use `enter_worktree`/`exit_worktree` tools.",
	"/cost":     "Show LLM cost summary. Use `finance.summary_read` tool.",
	"/resume":   "Resume conversation from previous session. (no-op kalau session aktif)",
}

func init() { tools.Register(&slashTool{}) }

type slashTool struct{}

func (t *slashTool) Name() string { return ToolSlashName }

func (t *slashTool) Description() string {
	return "Dispatch slash command literal (e.g. /plan, /tasks, /memory). Args: {input: '/plan args optional'}. Returns: {slash, action_hint, args, routed_tool_hint}."
}

func (t *slashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "Full slash command literal: /plan <args> atau /tasks atau /memory dll.",
			},
		},
		"required": []string{"input"},
	}
}

func (t *slashTool) Run(ctx context.Context, args map[string]any) (any, error) {
	input, _ := args["input"].(string)
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("slash_command: input required")
	}
	if !strings.HasPrefix(input, "/") {
		return nil, fmt.Errorf("slash_command: input harus mulai '/' (e.g. /plan)")
	}

	// Parse: /slash [args]
	parts := strings.SplitN(input, " ", 2)
	slash := parts[0]
	argText := ""
	if len(parts) == 2 {
		argText = strings.TrimSpace(parts[1])
	}

	hint, ok := slashRoutes[slash]
	if !ok {
		// Unknown slash — return all valid options
		valid := make([]string, 0, len(slashRoutes))
		for k := range slashRoutes {
			valid = append(valid, k)
		}
		return map[string]any{
			"slash":          slash,
			"recognized":     false,
			"error":          "unknown slash command",
			"valid_commands": valid,
		}, nil
	}

	return map[string]any{
		"slash":            slash,
		"recognized":       true,
		"args":             argText,
		"action_hint":      hint,
		"note":             "Phase 1 dispatcher: return action hint. Mr.Flow follow hint dispatch real tool.",
	}, nil
}

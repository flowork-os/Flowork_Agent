// Package tools — slash_commands.go: Phase 5.3 Slash Commands dispatcher.
//
// Adopt Claude Code /commit /review /memory /tasks /skills /plan /compact
// pattern. Adapted for Flowork Telegram interface — user send /<command>
// di Telegram, kernel route ke handler yang invoke appropriate tool.
//
// PHASE 5.3 STATUS: Registry + 12 default commands. Telegram bot integration
// pending kernel/warga/telegram dispatch hook (Phase 5.x lain).

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// SlashCommand — handler signature.
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(ctx context.Context, args string) (string, error)
}

var (
	slashRegistry = map[string]*SlashCommand{}
)

// RegisterSlashCommand — register command.
func RegisterSlashCommand(cmd *SlashCommand) {
	slashRegistry[cmd.Name] = cmd
}

// SlashCommandTool — dispatch /<command> input.
type SlashCommandTool struct{}

type slashArgs struct {
	Input string `json:"input" validate:"required"` // full "/command args"
}

func NewSlashCommandTool() *SlashCommandTool { return &SlashCommandTool{} }

func (t *SlashCommandTool) Definition() provider.ToolDefinition {
	names := []string{}
	for n := range slashRegistry {
		names = append(names, n)
	}
	desc := "Dispatch slash command /<name> <args>. Available: " + strings.Join(names, ", ")
	return provider.ToolDefinition{
		Name:        "SlashCommand",
		Description: desc,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string", "description": "/command args"},
			},
			"required": []string{"input"},
		},
	}
}

func (t *SlashCommandTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args slashArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("SlashCommand: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("SlashCommand: validation: %w", err)
	}
	input := strings.TrimSpace(args.Input)
	if !strings.HasPrefix(input, "/") {
		return Result{}, fmt.Errorf("SlashCommand: input must start with '/'")
	}
	parts := strings.SplitN(input[1:], " ", 2)
	cmdName := parts[0]
	cmdArgs := ""
	if len(parts) > 1 {
		cmdArgs = parts[1]
	}
	cmd, ok := slashRegistry[cmdName]
	if !ok {
		var known []string
		for n := range slashRegistry {
			known = append(known, "/"+n)
		}
		return Result{}, fmt.Errorf("unknown command /%s (available: %s)", cmdName, strings.Join(known, " "))
	}
	out, err := cmd.Handler(ctx, cmdArgs)
	if err != nil {
		return Result{}, fmt.Errorf("/%s failed: %w", cmdName, err)
	}
	return Result{
		Output: out,
		Metadata: map[string]any{
			"command": cmdName,
			"args":    cmdArgs,
		},
	}, nil
}

// InitDefaultSlashCommands — register 12 default slash commands.
func InitDefaultSlashCommands() {
	RegisterSlashCommand(&SlashCommand{
		Name:        "skills",
		Description: "List installed skills",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "# Skills installed\n\n(Loaded via kernel/skills/loader.go at startup. See ~/.flowork/skills/ + workspaces/<agent>/skills/ + bundled/skills/)", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "tasks",
		Description: "List active tasks (alias TaskList tool)",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "# Tasks\n\nUse TaskList tool dengan optional status filter (pending/running/completed/failed).", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "agents",
		Description: "List 6 built-in subagent types",
		Handler: func(ctx context.Context, args string) (string, error) {
			var out strings.Builder
			out.WriteString("# Built-in subagent types\n\n")
			for k, v := range SubagentTypes {
				out.WriteString(fmt.Sprintf("- **%s**: %s\n", k, v))
			}
			return out.String(), nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "plan",
		Description: "Enter plan mode untuk high-risk task",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Enter plan mode via EnterPlanMode tool dengan reason: " + args, nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "compact",
		Description: "Trigger context compression (Phase 3.2)",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Context compression: use skill 'compact' atau invoke compact_context tool (Phase 3.2).", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "memory",
		Description: "Show FLOWORK.md memory directory",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Memory directory: ~/.flowork/memory/FLOWORK.md (Phase 3.3 implementation pending).", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "doctor",
		Description: "Environment diagnostics",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Doctor: cek kernel /healthz + brain DB + GUI + llama-server. (Stub Phase 5.x.)", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "config",
		Description: "Show/set config",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Config: ENV var + settings DB. Toggle feature flags via SetEnabled() Phase 5.2.", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "mcp",
		Description: "MCP server management",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "MCP: client di kernel/tools/mcp.go ada. Server stub Phase 4.1.", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "worktree",
		Description: "Git worktree mgmt (alias EnterWorktree/ExitWorktree)",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Worktree: EnterWorktree create, ExitWorktree cleanup. Phase 1.4 done.", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "cost",
		Description: "Token usage cost tracker",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Cost: see state/analytics/calls.jsonl + budget snapshot. (Local Qwen = $0).", nil
		},
	})
	RegisterSlashCommand(&SlashCommand{
		Name:        "resume",
		Description: "Resume previous session",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "Resume: session ID dari state/sessions/. (Phase 3.x integration with Task system pending.)", nil
		},
	})
}

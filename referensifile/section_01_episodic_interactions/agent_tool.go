// Package tools — agent_tool.go: AgentTool implementation Phase 0.3.
//
// Per Ayah arahan 2026-05-17: adopt Claude Code AgentTool pattern.
// Spawn subagent dengan tool subset + scoped context + isolation option.
//
// Schema input (parity Claude Code AgentTool):
//
//	{
//	  "description": "short 3-5 word task",
//	  "prompt": "actual task detail",
//	  "subagent_type": "general-purpose | hacker | coder | researcher | planner | verifier",
//	  "model": "optional model override (sonnet/opus/haiku)",
//	  "mode": "default | plan | bypassPermissions | auto",
//	  "isolation": "worktree | remote (optional)",
//	  "cwd": "optional working directory",
//	  "name": "optional addressable name for SendMessage",
//	  "team_name": "optional team context",
//	  "run_in_background": bool
//	}
//
// PHASE 0.3 STATUS: SKELETON — schema + registration + stub spawn.
// Full spawn logic requires:
//   - Phase 1.1: 6 built-in subagent type persona templates
//   - Phase 1.4: Worktree manager (git worktree per subagent)
//   - Phase 1.5: Permission tiered modes (default/plan/bypass/auto)
//   - Phase 2.x: Skill loading per subagent
//
// Current behavior: validate input, log spawn intent, return structured
// stub showing what would happen. Future PR fills actual spawn execution.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// SubagentType — known built-in subagent types (Phase 0.3 skeleton list).
// Full persona templates di brain DB di Phase 1.1.
var SubagentTypes = map[string]string{
	"general-purpose": "Catch-all default subagent dengan full toolset",
	"hacker":          "Security/pentest/exploit/OSINT/RE — level dewa judgment-driven",
	"coder":           "Write/edit/multiedit/bash dengan workspace bypass authorization",
	"researcher":      "Read-only Grep/WebFetch/WebSearch untuk research + analysis",
	"planner":         "Plan mode only (read + plan, no execute) untuk high-risk task",
	"verifier":        "Build/vet/test runner untuk verification workflow",
}

// PermissionMode — 4 tiered permission modes (Phase 1.5 will enforce).
var PermissionModes = map[string]string{
	"default":           "Prompt per destructive operation",
	"plan":              "Show plan + user confirm sebelum execute",
	"bypassPermissions": "Auto-approve semua (use with care)",
	"auto":              "ML classifier decide per case",
}

// AgentTool — spawn subagent dengan tool subset + scoped context.
type AgentTool struct {
	// subagentRegistry pointer reserved untuk Phase 1.1 (persona templates)
}

type agentToolArgs struct {
	Description     string `json:"description" validate:"required"`
	Prompt          string `json:"prompt" validate:"required"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Isolation       string `json:"isolation,omitempty"`
	Cwd             string `json:"cwd,omitempty"`
	Name            string `json:"name,omitempty"`
	TeamName        string `json:"team_name,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

func NewAgentTool() *AgentTool {
	return &AgentTool{}
}

func (t *AgentTool) Definition() provider.ToolDefinition {
	// Build subagent_type enum description dari map
	var types []string
	for k := range SubagentTypes {
		types = append(types, k)
	}
	desc := "Spawn subagent for task delegation. Adopt Claude Code AgentTool pattern + Hermes Agent subagent. Subagent types: " + strings.Join(types, ", ") + ". Use untuk complex/specialized task delegation, parallel work, atau Plan mode high-risk task."
	return provider.ToolDefinition{
		Name:        "AgentTool",
		Description: desc,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Short 3-5 word task description",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Detailed task prompt for subagent",
				},
				"subagent_type": map[string]any{
					"type":        "string",
					"description": "Subagent type: " + strings.Join(types, " / "),
					"enum":        types,
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Optional model override",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Permission mode: default / plan / bypassPermissions / auto",
					"enum":        []string{"default", "plan", "bypassPermissions", "auto"},
				},
				"isolation": map[string]any{
					"type":        "string",
					"description": "Optional isolation mode 'worktree' (git worktree for parallel-safe edit)",
					"enum":        []string{"worktree"},
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Optional working directory override",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Optional addressable name for SendMessage routing",
				},
				"team_name": map[string]any{
					"type":        "string",
					"description": "Optional team context",
				},
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": "Run subagent in background, return notification on complete",
				},
			},
			"required": []string{"description", "prompt"},
		},
	}
}

func (t *AgentTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args agentToolArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("AgentTool: decode arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("AgentTool: validation: %w", err)
	}

	// Default subagent_type kalau ngga di-set
	if args.SubagentType == "" {
		args.SubagentType = "general-purpose"
	}
	if args.Mode == "" {
		args.Mode = "default"
	}

	// Validate subagent_type
	if _, ok := SubagentTypes[args.SubagentType]; !ok {
		var known []string
		for k := range SubagentTypes {
			known = append(known, k)
		}
		return Result{}, fmt.Errorf("AgentTool: unknown subagent_type %q (known: %s)", args.SubagentType, strings.Join(known, ", "))
	}

	// Validate mode
	if _, ok := PermissionModes[args.Mode]; !ok {
		return Result{}, fmt.Errorf("AgentTool: unknown mode %q", args.Mode)
	}

	// Log spawn intent
	log.Printf("[AgentTool] spawn intent: type=%s mode=%s isolation=%s name=%s bg=%v desc=%q",
		args.SubagentType, args.Mode, args.Isolation, args.Name, args.RunInBackground, args.Description)

	// === PHASE 0.3 SKELETON RESPONSE ===
	// Full spawn logic pending Phase 1.x:
	//   - Phase 1.1: persona template dari brain DB per subagent_type
	//   - Phase 1.4: worktree create kalau isolation='worktree'
	//   - Phase 1.5: permission mode enforcement
	//   - Phase 2.x: skill loading per subagent
	output := fmt.Sprintf(`# AgentTool Spawn (Phase 0.3 skeleton)

## Spawn intent received
- **description**: %s
- **subagent_type**: %s (%s)
- **mode**: %s (%s)
- **isolation**: %s
- **name**: %s
- **run_in_background**: %v

## Prompt
%s

## Status
**SKELETON** — schema validated + registered. Full spawn execution pending:
- Phase 1.1: Persona template loader per subagent_type
- Phase 1.4: Worktree manager (kalau isolation=worktree)
- Phase 1.5: Permission mode enforcement
- Phase 2.x: Skill loading per subagent

Sementara, untuk task ini, handle langsung dengan tools yang ada (jangan delegate via AgentTool sampai Phase 1.x landed).
`,
		args.Description,
		args.SubagentType, SubagentTypes[args.SubagentType],
		args.Mode, PermissionModes[args.Mode],
		args.Isolation,
		args.Name,
		args.RunInBackground,
		args.Prompt,
	)

	return Result{
		Output: output,
		Metadata: map[string]any{
			"subagent_type":     args.SubagentType,
			"mode":              args.Mode,
			"isolation":         args.Isolation,
			"name":              args.Name,
			"run_in_background": args.RunInBackground,
			"phase":             "0.3-skeleton",
			"pending_phase":     []string{"1.1", "1.4", "1.5", "2.x"},
		},
	}, nil
}

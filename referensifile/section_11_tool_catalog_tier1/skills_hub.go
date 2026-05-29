// Package tools — skills_hub.go: Phase 2.4 Skills Hub (agentskills.io).
//
// Adopt Hermes Agent + Claude Code skill sharing pattern. Registry remote
// untuk install skill dari community.
//
// PHASE 2.4 STATUS: SKELETON. Real implementation requires:
//   - HTTP client agentskills.io
//   - Skill validation (frontmatter + body schema check)
//   - Sandbox install (dry-run before commit)
//
// Sekarang: stub yang document pattern + return mock list.

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/teetah2402/flowork/internal/provider"
)

// SkillsHubTool — search + install skill dari remote hub.
type SkillsHubTool struct{}

type skillsHubArgs struct {
	Action     string `json:"action" validate:"required"` // search / install / list
	Query      string `json:"query,omitempty"`
	SkillName  string `json:"skill_name,omitempty"`
}

func NewSkillsHubTool() *SkillsHubTool { return &SkillsHubTool{} }

func (t *SkillsHubTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "SkillsHub",
		Description: "Browse + install community skill dari agentskills.io registry. " +
			"Action: search (find by keyword), install (fetch + save ke ~/.flowork/skills/hub/), list (installed).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":     map[string]any{"type": "string", "enum": []string{"search", "install", "list"}},
				"query":      map[string]any{"type": "string", "description": "Search keyword (for action=search)"},
				"skill_name": map[string]any{"type": "string", "description": "Skill name (for action=install)"},
			},
			"required": []string{"action"},
		},
	}
}

func (t *SkillsHubTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args skillsHubArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("SkillsHub: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("SkillsHub: validation: %w", err)
	}

	switch args.Action {
	case "list":
		return Result{
			Output: "# Skills Hub installed (Phase 2.4 SKELETON)\n\nNo skills installed via hub yet. Use action=search to browse.",
		}, nil
	case "search":
		return Result{
			Output: fmt.Sprintf("# Skills Hub search '%s' (Phase 2.4 SKELETON)\n\nPlaceholder. Real impl pending agentskills.io HTTP client.", args.Query),
		}, nil
	case "install":
		return Result{
			Output: fmt.Sprintf("# Skills Hub install '%s' (Phase 2.4 SKELETON)\n\nPlaceholder. Real impl: fetch agentskills.io/skills/%s + validate + save ~/.flowork/skills/hub/", args.SkillName, args.SkillName),
		}, nil
	default:
		return Result{}, fmt.Errorf("SkillsHub: unknown action %q", args.Action)
	}
}

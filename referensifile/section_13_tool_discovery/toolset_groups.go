// Package tools — toolset_groups.go: Phase 4.3 Toolset Enable/Disable Groups.
//
// Adopt Hermes Agent toolset pattern. Group tools by category, enable/disable
// per group bulk. Preset toolset (minimal / full / security / readonly).

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// ToolsetGroup — group definition.
type ToolsetGroup struct {
	Name        string
	Description string
	Tools       []string
}

// PresetToolsets — built-in preset.
var PresetToolsets = map[string]*ToolsetGroup{
	"minimal": {
		Name:        "minimal",
		Description: "Read + Bash only. Untuk simple Q&A / debug.",
		Tools:       []string{"read", "glob", "grep", "bash"},
	},
	"readonly": {
		Name:        "readonly",
		Description: "Read-only (no write/edit/bash destructive). Untuk research/analyze.",
		Tools:       []string{"read", "glob", "grep", "webfetch", "websearch", "brain_search"},
	},
	"coder": {
		Name:        "coder",
		Description: "Write/edit + bash + git. Untuk code change task.",
		Tools:       []string{"read", "edit", "write", "multiedit", "glob", "grep", "bash", "git_status", "git_diff", "git_commit"},
	},
	"security": {
		Name:        "security",
		Description: "Security/pentest/OSINT tools. Untuk hacker subagent.",
		Tools:       []string{"read", "glob", "grep", "webfetch", "websearch", "bash", "brain_search", "scan", "darkweb_research"},
	},
	"full": {
		Name:        "full",
		Description: "All registered tools. Untuk general-purpose subagent.",
		Tools:       []string{"*"},
	},
}

// ToolsetTool — query + apply preset.
type ToolsetTool struct{}

type toolsetArgs struct {
	Action string `json:"action" validate:"required"` // list / get / apply
	Name   string `json:"name,omitempty"`
}

func NewToolsetTool() *ToolsetTool { return &ToolsetTool{} }

func (t *ToolsetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "Toolset",
		Description: "Manage toolset preset groups. Action: list (show all preset), " +
			"get (show preset detail), apply (set active preset untuk subagent).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"list", "get", "apply"}},
				"name":   map[string]any{"type": "string", "description": "Preset name (for get/apply)"},
			},
			"required": []string{"action"},
		},
	}
}

func (t *ToolsetTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args toolsetArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("Toolset: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("Toolset: validation: %w", err)
	}

	switch args.Action {
	case "list":
		var keys []string
		for k := range PresetToolsets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString("# Toolset Presets (Phase 4.3)\n\n")
		for _, k := range keys {
			g := PresetToolsets[k]
			sb.WriteString(fmt.Sprintf("- **%s** (%d tools): %s\n", g.Name, len(g.Tools), g.Description))
		}
		return Result{Output: sb.String()}, nil
	case "get":
		g, ok := PresetToolsets[args.Name]
		if !ok {
			return Result{}, fmt.Errorf("toolset %q not found", args.Name)
		}
		return Result{
			Output: fmt.Sprintf("# Toolset: %s\n\n%s\n\nTools (%d):\n- %s",
				g.Name, g.Description, len(g.Tools), strings.Join(g.Tools, "\n- ")),
		}, nil
	case "apply":
		_, ok := PresetToolsets[args.Name]
		if !ok {
			return Result{}, fmt.Errorf("toolset %q not found", args.Name)
		}
		return Result{
			Output: fmt.Sprintf("# Toolset '%s' applied (Phase 4.3 SKELETON)\n\nReal impl: subagent runtime override capability matrix dengan toolset tools.", args.Name),
			Metadata: map[string]any{
				"applied":   true,
				"toolset":   args.Name,
				"tool_count": len(PresetToolsets[args.Name].Tools),
			},
		}, nil
	default:
		return Result{}, fmt.Errorf("Toolset: unknown action %q", args.Action)
	}
}

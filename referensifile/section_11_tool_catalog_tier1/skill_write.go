package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// skill_write.go — tool baru Sprint 3.5g 2026-05-03 fix orphan cap
// `skill_write` (toggled ON di GUI tapi belum ada implementation).
//
// Tulis SKILL.md ke ~/.flowork/skills/<name>/SKILL.md (global user skill)
// atau <workspace>/.flowork/skills/<name>/SKILL.md (workspace-private).
// Format mirror SkillRegistry.LoadUserSkills (skill_markdown.go) — YAML
// frontmatter `name`, `description`, `trigger` lalu body markdown.
//
// FQP-7 plug-and-play: tool ini bikin skill jadi data-driven, AI bisa
// self-bootstrap workflow baru tanpa restart binary (LoadUserSkills
// scan ulang next session — atau hot-reload kalau registry support).

type SkillWriteTool struct {
	workspace string
}

type skillWriteArgs struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description" validate:"required"`
	Content     string `json:"content" validate:"required"` // body markdown (instructions)
	Trigger     string `json:"trigger,omitempty"`
	Scope       string `json:"scope,omitempty"` // "user" (default) | "workspace"
}

func NewSkillWriteTool(workspace string) *SkillWriteTool {
	return &SkillWriteTool{workspace: workspace}
}

func (t *SkillWriteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "skill_write",
		Description: "Write a new SKILL.md file (skill bootstrap). Creates " +
			"~/.flowork/skills/<name>/SKILL.md (scope=user, default) or " +
			"<workspace>/.flowork/skills/<name>/SKILL.md (scope=workspace). " +
			"Skill akan ke-pickup oleh SkillRegistry.LoadUserSkills next boot " +
			"(atau hot-reload kalau aktif). Idempotent — overwrite kalau nama sama.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (lowercase, hyphen-separated). Used as folder name and YAML `name`.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Short description of when to use the skill (1-2 sentences).",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Markdown body — instructions/prompt the skill provides when invoked.",
				},
				"trigger": map[string]any{
					"type":        "string",
					"description": "Optional keywords/pattern that signal skill is relevant.",
				},
				"scope": map[string]any{
					"type":        "string",
					"description": "user|workspace (default: user)",
					"enum":        []string{"user", "workspace"},
				},
			},
			"required": []string{"name", "description", "content"},
		},
	}
}

func (t *SkillWriteTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args skillWriteArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{ToolName: "skill_write"}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{ToolName: "skill_write"}, fmt.Errorf("validation failed: %w", err)
	}

	name := strings.ToLower(strings.TrimSpace(args.Name))
	if name == "" {
		return Result{ToolName: "skill_write", OK: false, Output: "name is required"}, nil
	}
	// Sanitize name — folder-safe, no traversal.
	if strings.ContainsAny(name, `/\:*?"<>|`) || strings.Contains(name, "..") {
		return Result{
			ToolName: "skill_write",
			OK:       false,
			Output:   "name contains invalid path characters",
		}, nil
	}

	scope := strings.ToLower(strings.TrimSpace(args.Scope))
	if scope == "" {
		scope = "user"
	}

	var rootDir string
	switch scope {
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{
				ToolName: "skill_write",
				OK:       false,
				Output:   fmt.Sprintf("resolve home dir: %v", err),
			}, nil
		}
		rootDir = filepath.Join(home, ".flowork", "skills")
	case "workspace":
		if t.workspace == "" {
			return Result{
				ToolName: "skill_write",
				OK:       false,
				Output:   "workspace scope requires workspace root (none configured)",
			}, nil
		}
		rootDir = filepath.Join(t.workspace, ".flowork", "skills")
	default:
		return Result{
			ToolName: "skill_write",
			OK:       false,
			Output:   fmt.Sprintf("invalid scope %q (allowed: user|workspace)", scope),
		}, nil
	}

	skillDir := filepath.Join(rootDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Result{
			ToolName: "skill_write",
			OK:       false,
			Output:   fmt.Sprintf("mkdir %s: %v", skillDir, err),
		}, nil
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	body := buildSkillMarkdown(name, args.Description, args.Trigger, args.Content)
	if err := os.WriteFile(skillPath, []byte(body), 0o644); err != nil {
		return Result{
			ToolName: "skill_write",
			OK:       false,
			Output:   fmt.Sprintf("write %s: %v", skillPath, err),
		}, nil
	}

	return Result{
		ToolName: "skill_write",
		OK:       true,
		Output:   fmt.Sprintf("skill written: %s\nReload picks it up next boot (LoadUserSkills scan).", skillPath),
		Metadata: map[string]any{
			"path":  skillPath,
			"name":  name,
			"scope": scope,
		},
	}, nil
}

func buildSkillMarkdown(name, desc, trigger, content string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", name))
	b.WriteString(fmt.Sprintf("description: %s\n", strings.ReplaceAll(desc, "\n", " ")))
	if strings.TrimSpace(trigger) != "" {
		b.WriteString(fmt.Sprintf("trigger: %s\n", strings.ReplaceAll(trigger, "\n", " ")))
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

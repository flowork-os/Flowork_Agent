package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// git.go — generic `git` tool yang dispatch ke action status/log/diff/branch
// (read-only) atau ke specialized tools git_checkpoint / git_verify /
// git_rollback (write side, sudah ada). Sprint 3.5g 2026-05-03 fix orphan cap
// `git` di role_capabilities table tanpa duplikasi logic.
//
// Read-only dispatch jalan langsung di sini supaya tool one-shot
// (status / diff / log) gak butuh chain bash + parse output. Write-side
// proxy ke tool yang sudah teregister (caller LLM bisa direct call).

type GitTool struct {
	workspace string
}

type gitArgs struct {
	Action string `json:"action" validate:"required"` // status|log|diff|branch|checkpoint|verify|rollback
	Args   string `json:"args,omitempty"`             // raw extra args (mis. "--oneline -10")
}

func NewGitTool(workspace string) *GitTool { return &GitTool{workspace: workspace} }

func (t *GitTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "git",
		Description: "Generic git operation. Read-only actions run directly: " +
			"`status`, `log`, `diff`, `branch`. Write actions delegate ke specialized tool: " +
			"`checkpoint` -> git_checkpoint, `verify` -> git_verify, `rollback` -> git_rollback. " +
			"Pakai specialized tool langsung kalau butuh args spesifik (mis. message untuk checkpoint).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "One of: status, log, diff, branch, checkpoint, verify, rollback",
					"enum":        []string{"status", "log", "diff", "branch", "checkpoint", "verify", "rollback"},
				},
				"args": map[string]any{
					"type":        "string",
					"description": "Optional extra args, e.g. '--oneline -10' for log. Ignored for write actions.",
				},
			},
			"required": []string{"action"},
		},
	}
}

func (t *GitTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args gitArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{ToolName: "git"}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{ToolName: "git"}, fmt.Errorf("validation failed: %w", err)
	}

	action := strings.ToLower(strings.TrimSpace(args.Action))
	switch action {
	case "status", "log", "diff", "branch":
		return t.runReadOnly(ctx, action, args.Args)
	case "checkpoint", "verify", "rollback":
		return Result{
			ToolName: "git",
			OK:       false,
			Output: fmt.Sprintf("write action %q delegasi ke tool khusus: panggil `git_%s` "+
				"langsung dengan args yang sesuai (lihat tool definition).", action, action),
			Metadata: map[string]any{"redirect_to": "git_" + action},
		}, nil
	default:
		return Result{
			ToolName: "git",
			OK:       false,
			Output:   fmt.Sprintf("unknown action %q (allowed: status|log|diff|branch|checkpoint|verify|rollback)", action),
		}, nil
	}
}

func (t *GitTool) runReadOnly(ctx context.Context, action, extra string) (Result, error) {
	gitArgs := []string{"git", action}
	if strings.TrimSpace(extra) != "" {
		// Whitelist split — git args ngga punya quoting kompleks untuk
		// read-only ops yang lazim (--oneline, -10, --stat, dll).
		for _, p := range strings.Fields(extra) {
			if strings.HasPrefix(p, "--exec") || strings.HasPrefix(p, "--upload-pack") {
				return Result{
					ToolName: "git",
					OK:       false,
					Output:   fmt.Sprintf("rejected dangerous flag: %s", p),
				}, nil
			}
			gitArgs = append(gitArgs, p)
		}
	}
	out, err := runGit(ctx, t.workspace, gitArgs...)
	if err != nil {
		return Result{
			ToolName: "git",
			OK:       false,
			Output:   fmt.Sprintf("git %s failed: %v\n%s", action, err, out),
		}, nil
	}
	return Result{
		ToolName: "git",
		OK:       true,
		Output:   out,
		Metadata: map[string]any{"action": action},
	}, nil
}

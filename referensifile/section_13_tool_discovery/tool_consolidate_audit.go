// Package tools — tool_consolidate_audit.go: Phase 4.4 Tool Consolidate Audit.
//
// Per Ayah arahan: 136 tools registered di kernel terlalu banyak (banyak
// overlap + granular variant). Target ~50 high-quality consolidated.
//
// Strategy:
//   1. CONSOLIDATE granular variant → 1 tool dengan mode/sub-action
//   2. DROP deprecated (social media non-Telegram per Ayah Telegram-only)
//   3. PROMOTE frequently-used ke "productive set" (top-20)
//   4. DEFER rare ke "category drill" tier-2
//
// Tool ini = audit + recommendation report. Caller decide deletion.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// ConsolidationCandidate — tool group yang bisa di-merge.
type ConsolidationCandidate struct {
	Group       string   // logical group name (e.g., "git")
	Tools       []string // current tools that overlap
	Proposal    string   // proposed consolidated form
	Rationale   string   // why merge
}

// ConsolidationProposals — known groups yang harus di-consolidate.
var ConsolidationProposals = []ConsolidationCandidate{
	{
		Group:     "git",
		Tools:     []string{"git_status", "git_diff", "git_log", "git_show", "git_blame", "git_branch", "git_commit"},
		Proposal:  "1 git tool dengan mode={status,diff,log,show,blame,branch,commit}",
		Rationale: "7 git tool → 1 dengan sub-action. Cleaner LLM choice + smaller registry.",
	},
	{
		Group:     "read variants",
		Tools:     []string{"read", "read_lines", "file_get_lines"},
		Proposal:  "Keep 'read' dengan optional line_range param (line_from, line_to)",
		Rationale: "3 variant essentially same. Consolidate ke 1 tool dengan optional args.",
	},
	{
		Group:     "browser",
		Tools:     []string{"browser_navigate", "browser_click", "browser_type", "browser_render", "browser_login", "browser_screenshot"},
		Proposal:  "1 browser tool dengan action={navigate,click,type,render,login,screenshot}",
		Rationale: "6 browser tool → 1 dengan action enum. Mirror puppeteer/playwright API.",
	},
	{
		Group:     "memorize variants",
		Tools:     []string{"memorize_brain", "memory_set", "brain_post_drawer"},
		Proposal:  "Keep memorize_brain sebagai canonical. Deprecate 2 lain.",
		Rationale: "3 cara save memori = warga bingung pilih. Canonical 1.",
	},
	{
		Group:     "list helpers",
		Tools:     []string{"list_my_tools", "list_workspace_tools", "list_skills"},
		Proposal:  "1 list tool dengan type={tools,workspace_tools,skills}",
		Rationale: "Listing pattern sama, parameter beda.",
	},
	{
		Group:     "social media deprecated",
		Tools:     []string{"twitter_post", "facebook_post_status", "instagram_post", "linkedin_post", "discord_send", "reddit_post"},
		Proposal:  "DELETE all (Ayah arahan 'Telegram-only' 2026-05-12)",
		Rationale: "Daemon social media sudah di-delete Phase 1 cleanup. Tool registration leftover.",
	},
	{
		Group:     "telegram variants",
		Tools:     []string{"telegram_send", "telegram_blast", "telegram_alert"},
		Proposal:  "1 telegram_send dengan mode={normal,blast,alert} + recipient list",
		Rationale: "3 telegram tool similar, consolidate.",
	},
	{
		Group:     "alert auto-spam",
		Tools:     []string{"alert_owner", "alert_emergency"},
		Proposal:  "Rate-limit + threshold check sebelum fire. Plus content validation (anti-halu).",
		Rationale: "Per Ayah lapor 2026-05-17 PM: alert spam halu via Merpati. Need gate.",
	},
}

// ToolConsolidateAuditTool — show consolidation report.
type ToolConsolidateAuditTool struct {
	registry *Registry
}

type consolidateAuditArgs struct {
	Action string `json:"action" validate:"required"` // count / proposals / detail
	Group  string `json:"group,omitempty"`
}

func NewToolConsolidateAuditTool(reg *Registry) *ToolConsolidateAuditTool {
	return &ToolConsolidateAuditTool{registry: reg}
}

func (t *ToolConsolidateAuditTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "ToolConsolidateAudit",
		Description: "Tool consolidation audit report (Phase 4.4). Action: count (current count), " +
			"proposals (list merge candidates), detail (specific group).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"count", "proposals", "detail"}},
				"group":  map[string]any{"type": "string"},
			},
			"required": []string{"action"},
		},
	}
}

func (t *ToolConsolidateAuditTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args consolidateAuditArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("ToolConsolidateAudit: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("ToolConsolidateAudit: validation: %w", err)
	}

	switch args.Action {
	case "count":
		count := 0
		var names []string
		if t.registry != nil {
			for n := range t.registry.tools {
				count++
				names = append(names, n)
			}
		}
		sort.Strings(names)
		return Result{
			Output: fmt.Sprintf("# Tool Registry Count\n\nTotal registered: %d\nTarget post-consolidation: ~50\nReduction needed: %d tools\n\nRegistered (alphabetical):\n- %s",
				count, count-50, strings.Join(names, "\n- ")),
			Metadata: map[string]any{
				"count":           count,
				"target":          50,
				"reduction_needed": count - 50,
			},
		}, nil

	case "proposals":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Consolidation Proposals (%d groups)\n\n", len(ConsolidationProposals)))
		totalReduction := 0
		for _, c := range ConsolidationProposals {
			reduction := len(c.Tools) - 1
			if strings.HasPrefix(c.Proposal, "DELETE") {
				reduction = len(c.Tools)
			}
			sb.WriteString(fmt.Sprintf("## %s (-%d tools)\n", c.Group, reduction))
			sb.WriteString(fmt.Sprintf("- Current: %s\n", strings.Join(c.Tools, ", ")))
			sb.WriteString(fmt.Sprintf("- Proposal: %s\n", c.Proposal))
			sb.WriteString(fmt.Sprintf("- Rationale: %s\n\n", c.Rationale))
			totalReduction += reduction
		}
		sb.WriteString(fmt.Sprintf("---\n\nTotal reduction projected: **-%d tools**\n", totalReduction))
		sb.WriteString(fmt.Sprintf("From 136 → ~%d post-consolidation.\n", 136-totalReduction))
		return Result{
			Output: sb.String(),
			Metadata: map[string]any{
				"proposals":        len(ConsolidationProposals),
				"total_reduction":  totalReduction,
				"projected_final":  136 - totalReduction,
			},
		}, nil

	case "detail":
		for _, c := range ConsolidationProposals {
			if c.Group == args.Group {
				return Result{
					Output: fmt.Sprintf("# Group: %s\n\n**Current tools (%d):** %s\n\n**Proposal:** %s\n\n**Rationale:** %s",
						c.Group, len(c.Tools), strings.Join(c.Tools, ", "), c.Proposal, c.Rationale),
					Metadata: map[string]any{
						"group":      c.Group,
						"tool_count": len(c.Tools),
					},
				}, nil
			}
		}
		return Result{}, fmt.Errorf("group %q not found in proposals", args.Group)

	default:
		return Result{}, fmt.Errorf("unknown action %q", args.Action)
	}
}

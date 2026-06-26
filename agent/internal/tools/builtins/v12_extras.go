// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&workspaceUpsertTool{})
	tools.Register(&eduErrorUpsertTool{})
	tools.Register(&workspaceMetaCountTool{})
	tools.Register(&auditCountTool{})
	tools.Register(&decisionCountTool{})
	tools.Register(&mistakePromoteEligibleTool{})
	tools.Register(&protectorRuleAddTool{})
	tools.Register(&slashAliasResolveTool{})
}

type workspaceUpsertTool struct{}

func (workspaceUpsertTool) Name() string       { return "workspace_upsert" }
func (workspaceUpsertTool) Capability() string { return "state:write" }
func (workspaceUpsertTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Upsert workspace_meta entry. category+path UNIQUE — re-insert update. Buat catalog resource yg agent kerjain.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "document|code|log|media|...", Required: true},
			{Name: "path", Type: tools.ParamString, Description: "Relative path", Required: true},
			{Name: "description", Type: tools.ParamString, Description: "Optional description", Required: false},
			{Name: "shareable", Type: tools.ParamBool, Description: "Visible cross-agent (default true)", Required: false},
		},
		Returns: "{ok, id}",
	}
}

func (workspaceUpsertTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	path, _ := args["path"].(string)
	description, _ := args["description"].(string)
	shareable := true
	if v, ok := args["shareable"].(bool); ok {
		shareable = v
	}
	if category == "" || path == "" {
		return tools.Result{}, fmt.Errorf("category + path required")
	}
	id, err := store.RegisterMeta(category, path, description, "", 0, shareable)
	if err != nil {
		return tools.Result{}, fmt.Errorf("register workspace meta: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id},
	}, nil
}

type eduErrorUpsertTool struct{}

func (eduErrorUpsertTool) Name() string       { return "edu_error_upsert" }
func (eduErrorUpsertTool) Capability() string { return "state:write" }
func (eduErrorUpsertTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Upsert educational error entry. Code PRIMARY KEY. Buat tambah/update edukatif error per Section 9.",
		Params: []tools.Param{
			{Name: "code", Type: tools.ParamString, Description: "Error code (mis. ERR_TOOL_NOT_ALLOWED)", Required: true},
			{Name: "category", Type: tools.ParamString, Description: "Category", Required: true},
			{Name: "title", Type: tools.ParamString, Description: "Short title", Required: true},
			{Name: "explanation", Type: tools.ParamString, Description: "User-facing explanation", Required: true},
			{Name: "remediation", Type: tools.ParamString, Description: "How to fix", Required: true},
		},
		Returns: "{ok}",
	}
}

func (eduErrorUpsertTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	code, _ := args["code"].(string)
	category, _ := args["category"].(string)
	title, _ := args["title"].(string)
	explanation, _ := args["explanation"].(string)
	remediation, _ := args["remediation"].(string)
	if code == "" || title == "" || explanation == "" {
		return tools.Result{}, fmt.Errorf("code + title + explanation required")
	}
	e := agentdb.EduError{
		Code:        code,
		Category:    category,
		Title:       title,
		Explanation: explanation,
		Remediation: remediation,
	}
	if err := store.UpsertEduError(e); err != nil {
		return tools.Result{}, fmt.Errorf("upsert edu error: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "code": code},
	}, nil
}

type workspaceMetaCountTool struct{}

func (workspaceMetaCountTool) Name() string       { return "workspace_meta_count" }
func (workspaceMetaCountTool) Capability() string { return "state:read" }
func (workspaceMetaCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count workspace_meta entries by category. Quick overview tanpa list dump.",
		Params:      []tools.Param{},
		Returns:     "{total, by_category}",
	}
}

func (workspaceMetaCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListMeta("", 1000)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list meta: %w", err)
	}
	byCategory := map[string]int{}
	for _, m := range items {
		byCategory[m.Category]++
	}
	return tools.Result{
		Output: map[string]any{"total": len(items), "by_category": byCategory},
	}, nil
}

type auditCountTool struct{}

func (auditCountTool) Name() string       { return "audit_count" }
func (auditCountTool) Capability() string { return "state:read" }
func (auditCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count audit entries by event_type. Anti over-prompt: counts only, no list dump.",
		Params: []tools.Param{
			{Name: "since_iso", Type: tools.ParamString, Description: "From timestamp ISO (mis. 2026-05-29T00:00:00Z), kosong=all-time", Required: false},
		},
		Returns: "{counts_by_event_type}",
	}
}

func (auditCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	since, _ := args["since_iso"].(string)
	items, err := store.ListAudit("", since, "", 1000)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list audit: %w", err)
	}
	counts := map[string]int{}
	for _, e := range items {
		counts[e.EventType]++
	}
	return tools.Result{
		Output: map[string]any{"counts": counts, "total": len(items)},
	}, nil
}

type decisionCountTool struct{}

func (decisionCountTool) Name() string       { return "decision_count" }
func (decisionCountTool) Capability() string { return "state:read" }
func (decisionCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count decisions by decision_type + outcome. Quick analytics.",
		Params:      []tools.Param{},
		Returns:     "{by_type, by_outcome}",
	}
}

func (decisionCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListDecisions("", 1000)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list decisions: %w", err)
	}
	byType := map[string]int{}
	byOutcome := map[string]int{}
	for _, d := range items {
		byType[d.DecisionType]++
		byOutcome[d.Outcome]++
	}
	return tools.Result{
		Output: map[string]any{"by_type": byType, "by_outcome": byOutcome, "total": len(items)},
	}, nil
}

type mistakePromoteEligibleTool struct{}

func (mistakePromoteEligibleTool) Name() string       { return "mistake_promote_eligible" }
func (mistakePromoteEligibleTool) Capability() string { return "state:read" }
func (mistakePromoteEligibleTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List mistakes yang eligible promote ke Router antibody (Section 7) — tier=raw + hit_count >= N.",
		Params: []tools.Param{
			{Name: "min_hit_count", Type: tools.ParamInt, Description: "Min hit (default 3)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 20)", Required: false},
		},
		Returns: "{count, eligible[]}",
	}
}

func (mistakePromoteEligibleTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	minHit := int64(3)
	if n, ok := args["min_hit_count"].(float64); ok && n > 0 {
		minHit = int64(n)
	}
	limit := 20
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
	}
	items, err := store.ListMistakesEligibleForPromote(minHit, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list eligible: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "eligible": items},
	}, nil
}

type protectorRuleAddTool struct{}

func (protectorRuleAddTool) Name() string       { return "protector_rule_add" }
func (protectorRuleAddTool) Capability() string { return "state:write" }
func (protectorRuleAddTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Add protector rule — pattern matching kit. Pattern target message/tool call yang harus di-block. Reason+severity untuk audit trail.",
		Params: []tools.Param{
			{Name: "name", Type: tools.ParamString, Description: "Rule name (snake_case)", Required: true},
			{Name: "pattern", Type: tools.ParamString, Description: "Match pattern (substring atau regex)", Required: true},
			{Name: "action", Type: tools.ParamString, Description: "block|allow|warn", Required: true},
			{Name: "reason", Type: tools.ParamString, Description: "Why this rule", Required: false},
		},
		Returns: "{ok, id}",
	}
}

func (protectorRuleAddTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	name, _ := args["name"].(string)
	pattern, _ := args["pattern"].(string)
	action, _ := args["action"].(string)
	reason, _ := args["reason"].(string)
	if name == "" || pattern == "" || action == "" {
		return tools.Result{}, fmt.Errorf("name + pattern + action required")
	}

	source := reason
	if source == "" {
		source = "tool:" + name
	}
	rule := agentdb.ProtectorRule{
		RuleType: name,
		Pattern:  pattern,
		Action:   action,
		Source:   source,
		Enabled:  true,
	}
	id, err := store.AddProtectorRule(rule)
	if err != nil {
		return tools.Result{}, fmt.Errorf("add rule: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id},
	}, nil
}

type slashAliasResolveTool struct{}

func (slashAliasResolveTool) Name() string       { return "slash_alias_resolve" }
func (slashAliasResolveTool) Capability() string { return "state:read" }
func (slashAliasResolveTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Resolve slash alias → canonical command name. Lookup-style.",
		Params: []tools.Param{
			{Name: "alias", Type: tools.ParamString, Description: "Alias name (mis. 'v' → 'version')", Required: true},
		},
		Returns: "{found, alias, target}",
	}
}

func (slashAliasResolveTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	_, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	alias, _ := args["alias"].(string)
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return tools.Result{}, fmt.Errorf("alias required")
	}

	return tools.Result{
		Output: map[string]any{"found": false, "alias": alias, "note": "alias table API placeholder — future enhancement"},
	}, nil
}

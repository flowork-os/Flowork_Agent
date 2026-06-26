// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&slashHistoryTool{})
	tools.Register(&eduErrorLookupTool{})
	tools.Register(&eduErrorListTool{})
	tools.Register(&auditSearchTool{})
	tools.Register(&protectorAuditQueryTool{})
	tools.Register(&toolSubscribedListTool{})
}

type slashHistoryTool struct{}

func (slashHistoryTool) Name() string       { return "slash_history" }
func (slashHistoryTool) Capability() string { return "state:read" }
func (slashHistoryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List slash command history. Filter by command name. Default limit 30 (max 200).",
		Params: []tools.Param{
			{Name: "command", Type: tools.ParamString, Description: "Filter by command (mis. 'version', 'help')", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (slashHistoryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	command, _ := args["command"].(string)
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListSlashInvocations(command, "", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list slash: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type eduErrorLookupTool struct{}

func (eduErrorLookupTool) Name() string       { return "edu_error_lookup" }
func (eduErrorLookupTool) Capability() string { return "state:read" }
func (eduErrorLookupTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Lookup educational error by code (Section 9). Return explanation + remediation untuk hadling error tertentu.",
		Params: []tools.Param{
			{Name: "code", Type: tools.ParamString, Description: "Error code (mis. ERR_TOOL_NOT_ALLOWED)", Required: true},
		},
		Returns: "{found, code, title, explanation, remediation}",
	}
}

func (eduErrorLookupTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	code, _ := args["code"].(string)
	code = strings.TrimSpace(code)
	if code == "" {
		return tools.Result{}, fmt.Errorf("code required")
	}
	e, err := store.LookupEduError(code)
	if err != nil {
		return tools.Result{}, fmt.Errorf("lookup edu error: %w", err)
	}
	if e.Code == "" {
		return tools.Result{
			Output: map[string]any{"found": false, "code": code},
			Note:   "no entry for that code",
		}, nil
	}
	return tools.Result{
		Output: map[string]any{
			"found":       true,
			"code":        e.Code,
			"title":       e.Title,
			"explanation": e.Explanation,
			"remediation": e.Remediation,
		},
	}, nil
}

type eduErrorListTool struct{}

func (eduErrorListTool) Name() string       { return "edu_error_list" }
func (eduErrorListTool) Capability() string { return "state:read" }
func (eduErrorListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List educational errors catalog (Section 9). Filter by category. Default limit 50 (max 500).",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "Filter by category (network|tool|protected|...)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 50, max 500)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (eduErrorListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 500 {
			limit = 500
		}
	}
	items, err := store.ListEduErrors(category, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list edu errors: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type auditSearchTool struct{}

func (auditSearchTool) Name() string       { return "audit_search" }
func (auditSearchTool) Capability() string { return "state:read" }
func (auditSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search audit log by event_type. Default limit 30 (max 200).",
		Params: []tools.Param{
			{Name: "event_type", Type: tools.ParamString, Description: "Filter (mis. login_success, rate_limit_hit)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (auditSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	eventType, _ := args["event_type"].(string)
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListAudit(eventType, "", "", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list audit: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type protectorAuditQueryTool struct{}

func (protectorAuditQueryTool) Name() string       { return "protector_audit_query" }
func (protectorAuditQueryTool) Capability() string { return "state:read" }
func (protectorAuditQueryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Query protector_audit log — protector rule trigger history (allow/block decisions). Default limit 30.",
		Params: []tools.Param{
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (protectorAuditQueryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListProtectorAudit("", "", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list protector audit: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type toolSubscribedListTool struct{}

func (toolSubscribedListTool) Name() string       { return "tool_subscribed_list" }
func (toolSubscribedListTool) Capability() string { return "state:read" }
func (toolSubscribedListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List tools yang agent ini subscribe — source=manual|default|recommended. Self-introspection: 'gw punya tool apa aktif?'.",
		Params:      []tools.Param{},
		Returns:     "{count, items[]}",
	}
}

func (toolSubscribedListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListSubscriptions()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list subscriptions: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

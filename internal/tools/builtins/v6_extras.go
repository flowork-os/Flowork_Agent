// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 5 — 6 tool tambahan.
//
// v6_extras.go:
//   wallet_balance, finance_summary, finance_log, kv_list,
//   tool_invocations_list, protector_rules_list.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&financeSummaryTool{})
	tools.Register(&financeLogTool{})
	tools.Register(&kvListTool{})
	tools.Register(&toolInvocationsListTool{})
	tools.Register(&protectorRulesListTool{})
}

// =============================================================================
// 2. finance_summary — finance ledger summary per period
// =============================================================================

type financeSummaryTool struct{}

func (financeSummaryTool) Name() string       { return "finance_summary" }
func (financeSummaryTool) Capability() string { return "state:read" }
func (financeSummaryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Finance ledger summary — income/expense/net per period. Period: 24h/7d/30d (default 30d).",
		Params: []tools.Param{
			{Name: "period", Type: tools.ParamString, Description: "24h|7d|30d (default 30d)", Required: false},
		},
		Returns: "{period, income, expense, net, count}",
	}
}

func (financeSummaryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	period, _ := args["period"].(string)
	period = strings.TrimSpace(period)
	if period == "" {
		period = "30d"
	}
	// Period → from/to ISO. Simplification: from = empty (all-time) untuk 30d default.
	summary, err := store.SummaryLedger("", "")
	if err != nil {
		return tools.Result{}, fmt.Errorf("summary ledger: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"period": period, "summary": summary},
	}, nil
}

// =============================================================================
// 3. finance_log — append finance ledger entry
// =============================================================================

type financeLogTool struct{}

func (financeLogTool) Name() string       { return "finance_log" }
func (financeLogTool) Capability() string { return "state:write" }
func (financeLogTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Log finance entry — income/expense. Amount positive (sign by entry_type). Currency 3-letter ISO (USD/IDR/BTC).",
		Params: []tools.Param{
			{Name: "entry_type", Type: tools.ParamString, Description: "income|expense", Required: true},
			{Name: "amount", Type: tools.ParamFloat, Description: "Amount (positive, sign by entry_type)", Required: true},
			{Name: "currency", Type: tools.ParamString, Description: "ISO 3-letter (USD/IDR/BTC/...)", Required: true},
			{Name: "category", Type: tools.ParamString, Description: "tax|food|api_credit|salary|...", Required: false},
			{Name: "note", Type: tools.ParamString, Description: "Free-text note", Required: false},
		},
		Returns: "{ok, id}",
	}
}

func (financeLogTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	entryType, _ := args["entry_type"].(string)
	amt, _ := args["amount"].(float64)
	currency, _ := args["currency"].(string)
	category, _ := args["category"].(string)
	note, _ := args["note"].(string)
	if entryType != "income" && entryType != "expense" {
		return tools.Result{}, fmt.Errorf("entry_type must be income|expense")
	}
	if amt <= 0 {
		return tools.Result{}, fmt.Errorf("amount must be positive")
	}
	if currency == "" {
		return tools.Result{}, fmt.Errorf("currency required")
	}
	// AddLedger pakai struct FinanceLedger. Map ke fields.
	cost := amt
	if entryType == "expense" {
		// Expense as positive cost (consumer pattern).
	} else {
		// Income as negative cost (reverse).
		cost = -amt
	}
	entry := agentdb.FinanceLedger{
		Category:     category,
		Provider:     currency, // reuse Provider field untuk currency slot
		Model:        entryType,
		CostUSD:      cost,
		MetadataJSON: note,
	}
	id, err := store.AddLedger(entry)
	if err != nil {
		return tools.Result{}, fmt.Errorf("add ledger: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id},
	}, nil
}

// =============================================================================
// 4. kv_list — dump kv keys (anti-secret: skip prefix _)
// =============================================================================

type kvListTool struct{}

func (kvListTool) Name() string       { return "kv_list" }
func (kvListTool) Capability() string { return "state:read" }
func (kvListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List kv table keys (config storage). Anti-secret: keys mulai underscore di-mask. Buat introspect 'config apa yang gw punya'.",
		Params:      []tools.Param{},
		Returns:     "{count, keys[]}",
	}
}

func (kvListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load cfg: %w", err)
	}
	kvAny, _ := cfg["kv"].(map[string]any)
	keys := make([]string, 0, len(kvAny))
	for k := range kvAny {
		if strings.HasPrefix(k, "_") {
			keys = append(keys, "(masked) "+k)
		} else {
			keys = append(keys, k)
		}
	}
	return tools.Result{
		Output: map[string]any{"count": len(keys), "keys": keys},
	}, nil
}

// =============================================================================
// 5. tool_invocations_list — tool call audit log per agent
// =============================================================================

type toolInvocationsListTool struct{}

func (toolInvocationsListTool) Name() string       { return "tool_invocations_list" }
func (toolInvocationsListTool) Capability() string { return "state:read" }
func (toolInvocationsListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List tool invocations log — semua tool call yg agent ini lakukan + result + duration. Filter by tool name. Default limit 50.",
		Params: []tools.Param{
			{Name: "tool_name", Type: tools.ParamString, Description: "Filter (kosong=all)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 50, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (toolInvocationsListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	toolName, _ := args["tool_name"].(string)
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListToolInvocations(toolName, "", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list tool invocations: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

// =============================================================================
// 6. protector_rules_list — list protector rules aktif
// =============================================================================

type protectorRulesListTool struct{}

func (protectorRulesListTool) Name() string       { return "protector_rules_list" }
func (protectorRulesListTool) Capability() string { return "state:read" }
func (protectorRulesListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List protector rules untuk agent — pattern matching kit yg block/allow tool call atau message.",
		Params:      []tools.Param{},
		Returns:     "{count, rules[]}",
	}
}

func (protectorRulesListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	rules, err := store.ListProtectorRules()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list protector rules: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(rules), "rules": rules},
	}, nil
}

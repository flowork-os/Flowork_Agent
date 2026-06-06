// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/checking-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 7 — 6 tool tambahan.
//
// v8_extras.go:
//   self_prompt_render, self_prompt_set, codemap_search_advanced,
//   wallet_alert_list, wallet_alerts_fired_list, ledger_list.

package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&selfPromptRenderTool{})
	tools.Register(&selfPromptSetTool{})
	tools.Register(&codemapSearchAdvancedTool{})
	tools.Register(&ledgerListTool{})
}

// =============================================================================
// 1. self_prompt_render — render self-prompt slots full
// =============================================================================

type selfPromptRenderTool struct{}

func (selfPromptRenderTool) Name() string       { return "self_prompt_render" }
func (selfPromptRenderTool) Capability() string { return "state:read" }
func (selfPromptRenderTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Render full self-prompt slots (system/persona/guideline/task — Section 35). Return current configured slot bodies.",
		Params:      []tools.Param{},
		Returns:     "{slots[]}",
	}
}

func (selfPromptRenderTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	slots, err := store.ListSelfPromptSlots()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list self prompt: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(slots), "slots": slots},
	}, nil
}

// =============================================================================
// 2. self_prompt_set — set/update self-prompt slot
// =============================================================================

type selfPromptSetTool struct{}

func (selfPromptSetTool) Name() string       { return "self_prompt_set" }
func (selfPromptSetTool) Capability() string { return "state:write" }
func (selfPromptSetTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Set/update self-prompt slot. Slot 'system|persona|guideline|task'. Body markdown OK, max 8KB.",
		Params: []tools.Param{
			{Name: "slot", Type: tools.ParamString, Description: "system|persona|guideline|task", Required: true},
			{Name: "body", Type: tools.ParamString, Description: "Slot content (max 8KB)", Required: true},
		},
		Returns: "{ok}",
	}
}

func (selfPromptSetTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	slot, _ := args["slot"].(string)
	body, _ := args["body"].(string)
	slot = strings.TrimSpace(slot)
	if slot == "" {
		return tools.Result{}, fmt.Errorf("slot required")
	}
	valid := map[string]bool{"system": true, "persona": true, "guideline": true, "task": true}
	if !valid[slot] {
		return tools.Result{}, fmt.Errorf("slot must be system|persona|guideline|task")
	}
	if body == "" {
		return tools.Result{}, fmt.Errorf("body required")
	}
	id, err := store.SetSelfPrompt(slot, body, "", 1)
	if err != nil {
		return tools.Result{}, fmt.Errorf("set self prompt: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "slot": slot, "id": id, "body_len": len(body)},
	}, nil
}

// =============================================================================
// 3. codemap_search_advanced — search dengan node_type + layer filter
// =============================================================================

type codemapSearchAdvancedTool struct{}

func (codemapSearchAdvancedTool) Name() string       { return "codemap_search_advanced" }
func (codemapSearchAdvancedTool) Capability() string { return "state:read" }
func (codemapSearchAdvancedTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search codemap nodes by name + filter node_type (func|type|var|const) + layer (handler|service|store). Cap 20 hits.",
		Params: []tools.Param{
			{Name: "search", Type: tools.ParamString, Description: "Substring match on name", Required: false},
			{Name: "node_type", Type: tools.ParamString, Description: "func|type|var|const", Required: false},
			{Name: "layer", Type: tools.ParamString, Description: "Logical layer tag", Required: false},
		},
		Returns: "{count, nodes[]}",
	}
}

func (codemapSearchAdvancedTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	search, _ := args["search"].(string)
	nodeType, _ := args["node_type"].(string)
	layer, _ := args["layer"].(string)
	nodes, err := store.ListCodemapNodes(nodeType, layer, search, 20)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list codemap: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(nodes), "nodes": nodes},
	}, nil
}

// =============================================================================
// 6. ledger_list — list finance ledger entries with filter
// =============================================================================

type ledgerListTool struct{}

func (ledgerListTool) Name() string       { return "ledger_list" }
func (ledgerListTool) Capability() string { return "state:read" }
func (ledgerListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List finance ledger entries — income+expense rows. Filter by category. Default limit 50.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "Filter (kosong=all)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 50, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (ledgerListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListLedger(category, "", "", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list ledger: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

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
	tools.Register(&protectorRuleDeleteTool{})
	tools.Register(&deathLetterSealTool{})
	tools.Register(&financeBudgetSetTool{})
	tools.Register(&skillAddTool{})
	tools.Register(&skillRemoveTool{})
	tools.Register(&secretSetTool{})
	tools.Register(&secretGetKeysTool{})
}

type protectorRuleDeleteTool struct{}

func (protectorRuleDeleteTool) Name() string       { return "protector_rule_delete" }
func (protectorRuleDeleteTool) Capability() string { return "state:write" }
func (protectorRuleDeleteTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Delete protector rule by ID.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamInt, Description: "Rule ID", Required: true},
		},
		Returns: "{ok}",
	}
}

func (protectorRuleDeleteTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	idf, _ := args["id"].(float64)
	if idf <= 0 {
		return tools.Result{}, fmt.Errorf("id required")
	}
	if err := store.DeleteProtectorRule(int64(idf)); err != nil {
		return tools.Result{}, fmt.Errorf("delete: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": int64(idf)},
	}, nil
}

type deathLetterSealTool struct{}

func (deathLetterSealTool) Name() string       { return "death_letter_seal" }
func (deathLetterSealTool) Capability() string { return "state:write" }
func (deathLetterSealTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Seal death letter by id — set sealed_at, body immutable after. Final commit wasiat.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamInt, Description: "Letter ID", Required: true},
		},
		Returns: "{ok}",
	}
}

func (deathLetterSealTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	idf, _ := args["id"].(float64)
	if idf <= 0 {
		return tools.Result{}, fmt.Errorf("id required")
	}
	if err := store.SealLetter(int64(idf)); err != nil {
		return tools.Result{}, fmt.Errorf("seal: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": int64(idf)},
	}, nil
}

type financeBudgetSetTool struct{}

func (financeBudgetSetTool) Name() string       { return "finance_budget_set" }
func (financeBudgetSetTool) Capability() string { return "state:write" }
func (financeBudgetSetTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Set budget per metric_key. warning_at_pct = warning threshold (0-1).",
		Params: []tools.Param{
			{Name: "metric_key", Type: tools.ParamString, Description: "Metric key (mis. 'daily_cost_usd')", Required: true},
			{Name: "budget_value", Type: tools.ParamFloat, Description: "Hard limit", Required: true},
			{Name: "warning_at_pct", Type: tools.ParamFloat, Description: "Warning pct 0-1 (default 0.8)", Required: false},
		},
		Returns: "{ok}",
	}
}

func (financeBudgetSetTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	metricKey, _ := args["metric_key"].(string)
	budgetValue, _ := args["budget_value"].(float64)
	warnPct, _ := args["warning_at_pct"].(float64)
	if warnPct <= 0 {
		warnPct = 0.8
	}
	if metricKey == "" || budgetValue <= 0 {
		return tools.Result{}, fmt.Errorf("metric_key + budget_value required")
	}
	budget := agentdb.FinanceBudget{
		MetricKey:    metricKey,
		BudgetValue:  budgetValue,
		WarningAtPct: warnPct,
		Enabled:      true,
	}
	if err := store.SetBudget(budget); err != nil {
		return tools.Result{}, fmt.Errorf("set budget: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "metric_key": metricKey},
	}, nil
}

type skillAddTool struct{}

func (skillAddTool) Name() string       { return "skill_add" }
func (skillAddTool) Capability() string { return "state:write" }
func (skillAddTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Add agent skill (id + trigger + instructions). Anti over-prompt: max 3 auto-inject; sisanya via skill_search on-demand.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "Skill ID (snake_case)", Required: true},
			{Name: "trigger", Type: tools.ParamString, Description: "Trigger pattern (mis. '/version', '#crypto')", Required: false},
			{Name: "instructions", Type: tools.ParamString, Description: "Skill instruction text", Required: true},
		},
		Returns: "{ok}",
	}
}

func (skillAddTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	id, _ := args["id"].(string)
	trigger, _ := args["trigger"].(string)
	instructions, _ := args["instructions"].(string)
	if id == "" || instructions == "" {
		return tools.Result{}, fmt.Errorf("id + instructions required")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	skillsAny, _ := cfg["skills"].([]any)
	filtered := make([]any, 0, len(skillsAny))
	for _, s := range skillsAny {
		m, _ := s.(map[string]any)
		if m == nil {
			continue
		}
		if existID, _ := m["id"].(string); existID == id {
			continue
		}
		filtered = append(filtered, s)
	}
	filtered = append(filtered, map[string]any{"id": id, "trigger": trigger, "instructions": instructions})
	cfg["skills"] = filtered
	if err := store.Save(cfg); err != nil {
		return tools.Result{}, fmt.Errorf("save: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id, "total_skills": len(filtered)},
	}, nil
}

type skillRemoveTool struct{}

func (skillRemoveTool) Name() string       { return "skill_remove" }
func (skillRemoveTool) Capability() string { return "state:write" }
func (skillRemoveTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Remove agent skill by ID.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "Skill ID", Required: true},
		},
		Returns: "{ok, removed}",
	}
}

func (skillRemoveTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	id, _ := args["id"].(string)
	if id == "" {
		return tools.Result{}, fmt.Errorf("id required")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	skillsAny, _ := cfg["skills"].([]any)
	removed := false
	filtered := make([]any, 0, len(skillsAny))
	for _, s := range skillsAny {
		m, _ := s.(map[string]any)
		if m == nil {
			continue
		}
		if existID, _ := m["id"].(string); existID == id {
			removed = true
			continue
		}
		filtered = append(filtered, s)
	}
	cfg["skills"] = filtered
	if err := store.Save(cfg); err != nil {
		return tools.Result{}, fmt.Errorf("save: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "removed": removed},
	}, nil
}

type secretSetTool struct{}

func (secretSetTool) Name() string       { return "secret_set" }
func (secretSetTool) Capability() string { return "state:write" }
func (secretSetTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Set secret (env credential). Stored di secrets table. Re-set = update. Key uppercase convention (mis. TELEGRAM_BOT_TOKEN).",
		Params: []tools.Param{
			{Name: "key", Type: tools.ParamString, Description: "Secret key (UPPER_SNAKE_CASE)", Required: true},
			{Name: "value", Type: tools.ParamString, Description: "Secret value", Required: true},
		},
		Returns: "{ok}",
	}
}

func (secretSetTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	key = strings.TrimSpace(key)
	if key == "" || value == "" {
		return tools.Result{}, fmt.Errorf("key + value required")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	secrets, _ := cfg["secrets"].(map[string]any)
	if secrets == nil {
		secrets = map[string]any{}
	}
	secrets[key] = value
	cfg["secrets"] = secrets
	if err := store.Save(cfg); err != nil {
		return tools.Result{}, fmt.Errorf("save: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "key": key},
	}, nil
}

type secretGetKeysTool struct{}

func (secretGetKeysTool) Name() string       { return "secret_get_keys" }
func (secretGetKeysTool) Capability() string { return "state:read" }
func (secretGetKeysTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List secret keys (names only, NO values). Anti leak: introspection 'gw punya credentials apa' tanpa expose value.",
		Params:      []tools.Param{},
		Returns:     "{keys[]}",
	}
}

func (secretGetKeysTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	secrets, err := store.Secrets()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load secrets: %w", err)
	}
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	return tools.Result{
		Output: map[string]any{"count": len(keys), "keys": keys},
	}, nil
}

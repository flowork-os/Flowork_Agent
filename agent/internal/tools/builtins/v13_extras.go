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
	tools.Register(&schedulerScheduleAddTool{})
	tools.Register(&schedulerScheduleRemoveTool{})
	tools.Register(&mistakePromoteMarkTool{})
	tools.Register(&protectorRuleToggleTool{})
	tools.Register(&eduErrorCountTool{})
	tools.Register(&mistakesCountTool{})
	tools.Register(&interactionCountTool{})
}

type schedulerScheduleAddTool struct{}

func (schedulerScheduleAddTool) Name() string       { return "scheduler_schedule_add" }
func (schedulerScheduleAddTool) Capability() string { return "state:write" }
func (schedulerScheduleAddTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Add new schedule entry — agent ngecut cron task. ID PRIMARY (upsert). Cron 5-field POSIX.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "Schedule ID (kebab-case)", Required: true},
			{Name: "cron", Type: tools.ParamString, Description: "5-field cron (min hr dom mon dow)", Required: true},
			{Name: "task", Type: tools.ParamString, Description: "Task text (lead '/' = slash command)", Required: true},
		},
		Returns: "{ok}",
	}
}

func (schedulerScheduleAddTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	id, _ := args["id"].(string)
	cron, _ := args["cron"].(string)
	task, _ := args["task"].(string)
	id = strings.TrimSpace(id)
	cron = strings.TrimSpace(cron)
	task = strings.TrimSpace(task)
	if id == "" || cron == "" || task == "" {
		return tools.Result{}, fmt.Errorf("id + cron + task required")
	}

	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	scheduleAny, _ := cfg["schedule"].([]any)

	filtered := make([]any, 0, len(scheduleAny))
	for _, s := range scheduleAny {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if existID, _ := m["id"].(string); existID == id {
			continue
		}
		filtered = append(filtered, s)
	}
	filtered = append(filtered, map[string]any{"id": id, "cron": cron, "task": task})
	cfg["schedule"] = filtered
	if err := store.Save(cfg); err != nil {
		return tools.Result{}, fmt.Errorf("save: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id, "total_schedules": len(filtered)},
	}, nil
}

type schedulerScheduleRemoveTool struct{}

func (schedulerScheduleRemoveTool) Name() string       { return "scheduler_schedule_remove" }
func (schedulerScheduleRemoveTool) Capability() string { return "state:write" }
func (schedulerScheduleRemoveTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Remove schedule entry by ID.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamString, Description: "Schedule ID to remove", Required: true},
		},
		Returns: "{ok, removed}",
	}
}

func (schedulerScheduleRemoveTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	id, _ := args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return tools.Result{}, fmt.Errorf("id required")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	scheduleAny, _ := cfg["schedule"].([]any)
	removed := false
	filtered := make([]any, 0, len(scheduleAny))
	for _, s := range scheduleAny {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if existID, _ := m["id"].(string); existID == id {
			removed = true
			continue
		}
		filtered = append(filtered, s)
	}
	cfg["schedule"] = filtered
	if err := store.Save(cfg); err != nil {
		return tools.Result{}, fmt.Errorf("save: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "removed": removed, "total_schedules": len(filtered)},
	}, nil
}

type mistakePromoteMarkTool struct{}

func (mistakePromoteMarkTool) Name() string       { return "mistake_promote_mark" }
func (mistakePromoteMarkTool) Capability() string { return "state:write" }
func (mistakePromoteMarkTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Mark mistake (by id) sebagai promoted ke Router antibody. Set tier='promoted' + promoted_at + promoted_to_id link.",
		Params: []tools.Param{
			{Name: "mistake_id", Type: tools.ParamInt, Description: "Mistake row ID", Required: true},
			{Name: "promoted_to_id", Type: tools.ParamString, Description: "Router antibody/drawer ID after promote", Required: true},
		},
		Returns: "{ok}",
	}
}

func (mistakePromoteMarkTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	idf, _ := args["mistake_id"].(float64)
	promotedTo, _ := args["promoted_to_id"].(string)
	if idf <= 0 || promotedTo == "" {
		return tools.Result{}, fmt.Errorf("mistake_id + promoted_to_id required")
	}

	var promotedToID int64
	for _, c := range promotedTo {
		if c >= '0' && c <= '9' {
			promotedToID = promotedToID*10 + int64(c-'0')
		}
	}
	if err := store.SetMistakePromoted(int64(idf), promotedToID); err != nil {
		return tools.Result{}, fmt.Errorf("set promoted: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "mistake_id": int64(idf)},
	}, nil
}

type protectorRuleToggleTool struct{}

func (protectorRuleToggleTool) Name() string       { return "protector_rule_toggle" }
func (protectorRuleToggleTool) Capability() string { return "state:write" }
func (protectorRuleToggleTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Toggle protector rule enabled/disabled by ID.",
		Params: []tools.Param{
			{Name: "id", Type: tools.ParamInt, Description: "Rule ID", Required: true},
			{Name: "enabled", Type: tools.ParamBool, Description: "true=enable, false=disable", Required: true},
		},
		Returns: "{ok}",
	}
}

func (protectorRuleToggleTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	idf, _ := args["id"].(float64)
	enabled, ok2 := args["enabled"].(bool)
	if idf <= 0 || !ok2 {
		return tools.Result{}, fmt.Errorf("id + enabled bool required")
	}
	if err := store.ToggleProtectorRule(int64(idf), enabled); err != nil {
		return tools.Result{}, fmt.Errorf("toggle rule: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": int64(idf), "enabled": enabled},
	}, nil
}

type eduErrorCountTool struct{}

func (eduErrorCountTool) Name() string       { return "edu_error_count" }
func (eduErrorCountTool) Capability() string { return "state:read" }
func (eduErrorCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count educational errors. Quick overview Section 9 catalog state.",
		Params:      []tools.Param{},
		Returns:     "{total}",
	}
}

func (eduErrorCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	total, err := store.CountEduErrors()
	if err != nil {
		return tools.Result{}, fmt.Errorf("count: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"total": total},
	}, nil
}

type mistakesCountTool struct{}

func (mistakesCountTool) Name() string       { return "mistakes_count" }
func (mistakesCountTool) Capability() string { return "state:read" }
func (mistakesCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count mistakes_local entries by tier (raw/promoted/reviewed).",
		Params:      []tools.Param{},
		Returns:     "{total, by_tier}",
	}
}

func (mistakesCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListMistakes("", 1000)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list: %w", err)
	}
	byTier := map[string]int{}
	for _, m := range items {
		byTier[m.Tier]++
	}
	return tools.Result{
		Output: map[string]any{"total": len(items), "by_tier": byTier},
	}, nil
}

type interactionCountTool struct{}

func (interactionCountTool) Name() string       { return "interaction_count" }
func (interactionCountTool) Capability() string { return "state:read" }
func (interactionCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Count total interactions logged. Quick KPI.",
		Params:      []tools.Param{},
		Returns:     "{total}",
	}
}

func (interactionCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	total, err := store.CountInteractions()
	if err != nil {
		return tools.Result{}, fmt.Errorf("count: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"total": total},
	}, nil
}

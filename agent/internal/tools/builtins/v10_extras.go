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
	tools.Register(&sneakernetExportQueryTool{})
	tools.Register(&slashAliasListTool{})
	tools.Register(&toolSubscriptionsCountTool{})
	tools.Register(&scheduleRunsQueryTool{})
	tools.Register(&scannerQuickScanTool{})
	tools.Register(&schedulerNextTool{})
}

type sneakernetExportQueryTool struct{}

func (sneakernetExportQueryTool) Name() string       { return "sneakernet_export_query" }
func (sneakernetExportQueryTool) Capability() string { return "state:read" }
func (sneakernetExportQueryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Snapshot sneakernet capability — manifest agent + total tools subscribed + schedule count. Info untuk export decision.",
		Params:      []tools.Param{},
		Returns:     "{manifest_info, tools_count, schedule_count}",
	}
}

func (sneakernetExportQueryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	tools_, _ := cfg["tools"].([]any)
	schedule, _ := cfg["schedule"].([]any)
	subs, _ := store.ListSubscriptions()
	return tools.Result{
		Output: map[string]any{
			"tools_in_manifest": len(tools_),
			"schedule_count":    len(schedule),
			"active_subs":       len(subs),
		},
	}, nil
}

type slashAliasListTool struct{}

func (slashAliasListTool) Name() string       { return "slash_alias_list" }
func (slashAliasListTool) Capability() string { return "state:read" }
func (slashAliasListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List slash command aliases — alias → target_name mapping.",
		Params:      []tools.Param{},
		Returns:     "{count, aliases[]}",
	}
}

func (slashAliasListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	_, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}

	return tools.Result{
		Output: map[string]any{"count": 0, "aliases": []any{}, "note": "alias API placeholder — future enhancement"},
	}, nil
}

type toolSubscriptionsCountTool struct{}

func (toolSubscriptionsCountTool) Name() string       { return "tool_subscriptions_count" }
func (toolSubscriptionsCountTool) Capability() string { return "state:read" }
func (toolSubscriptionsCountTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Quick count tool subscriptions + breakdown by source (manual/default/recommended).",
		Params:      []tools.Param{},
		Returns:     "{total, by_source}",
	}
}

func (toolSubscriptionsCountTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	subs, err := store.ListSubscriptions()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list subs: %w", err)
	}
	bySource := map[string]int{}
	for _, s := range subs {
		bySource[s.Source]++
	}
	return tools.Result{
		Output: map[string]any{"total": len(subs), "by_source": bySource},
	}, nil
}

type scheduleRunsQueryTool struct{}

func (scheduleRunsQueryTool) Name() string       { return "schedule_runs_query" }
func (scheduleRunsQueryTool) Capability() string { return "state:read" }
func (scheduleRunsQueryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List scheduler runs history — fired_at, outcome, duration_ms, task. Default limit 50 (max 200).",
		Params: []tools.Param{
			{Name: "limit", Type: tools.ParamInt, Description: "Max", Required: false},
		},
		Returns: "{count, runs[]}",
	}
}

func (scheduleRunsQueryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListSchedulerRuns("", limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list scheduler runs: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "runs": items},
	}, nil
}

type scannerQuickScanTool struct{}

func (scannerQuickScanTool) Name() string       { return "scanner_quick_scan" }
func (scannerQuickScanTool) Capability() string { return "state:read" }
func (scannerQuickScanTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Quick scan — count finding by severity dari scanner_findings table (last 30 hari heuristic).",
		Params:      []tools.Param{},
		Returns:     "{critical, high, medium, low, info}",
	}
}

func (scannerQuickScanTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	runs, err := store.ListScannerRuns(50)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list runs: %w", err)
	}
	totals := map[string]int{}
	for _, r := range runs {
		findings, _ := store.ListScannerFindings(r.ID)
		for _, f := range findings {
			totals[strings.ToLower(f.Severity)]++
		}
	}
	return tools.Result{
		Output: totals,
	}, nil
}

type schedulerNextTool struct{}

func (schedulerNextTool) Name() string       { return "scheduler_next" }
func (schedulerNextTool) Capability() string { return "state:read" }
func (schedulerNextTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Get next_run_at untuk semua schedule. Caller bisa lihat kapan task next fire.",
		Params:      []tools.Param{},
		Returns:     "{schedules[]}",
	}
}

func (schedulerNextTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListSchedulesForRunner()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list schedules: %w", err)
	}
	out := make([]map[string]any, 0, len(items))
	for _, s := range items {
		out = append(out, map[string]any{
			"id":          s.ID,
			"cron":        s.Cron,
			"task":        s.Task,
			"next_run_at": s.NextRunAt,
			"last_run_at": s.LastRunAt,
		})
	}
	return tools.Result{
		Output: map[string]any{"count": len(out), "schedules": out},
	}, nil
}

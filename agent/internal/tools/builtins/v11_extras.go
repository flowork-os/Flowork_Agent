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
	tools.Register(&statSummaryTool{})
	tools.Register(&capabilitiesListTool{})
	tools.Register(&watchdogAlertsListTool{})
	tools.Register(&zombieFindingsListTool{})
	tools.Register(&personaGetTool{})
	tools.Register(&decisionSearchTool{})
}

type statSummaryTool struct{}

func (statSummaryTool) Name() string       { return "stat_summary" }
func (statSummaryTool) Capability() string { return "state:read" }
func (statSummaryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Quick statistics overview: total counts per table. Buat 'how am I doing?' self-introspection.",
		Params:      []tools.Param{},
		Returns:     "{interactions, death_letters, scanner_runs, schedules, subscriptions}",
	}
}

func (statSummaryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	interactions, _ := store.CountInteractions()
	letters, _ := store.CountLetters(false)
	runs, _ := store.ListScannerRuns(1000)
	schedules, _ := store.ListSchedulesForRunner()
	subs, _ := store.ListSubscriptions()
	return tools.Result{
		Output: map[string]any{
			"interactions":  interactions,
			"death_letters": letters,
			"scanner_runs":  len(runs),
			"schedules":     len(schedules),
			"subscriptions": len(subs),
		},
	}, nil
}

type capabilitiesListTool struct{}

func (capabilitiesListTool) Name() string       { return "capabilities_list" }
func (capabilitiesListTool) Capability() string { return "state:read" }
func (capabilitiesListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List unique capabilities yang aktif (state:read/write, net:fetch:*, exec:*, dst). Self-intro: 'gw bisa apa?'.",
		Params:      []tools.Param{},
		Returns:     "{capabilities[]}",
	}
}

func (capabilitiesListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	summaries := tools.ListSummaries()
	caps := map[string]int{}
	for _, s := range summaries {
		if s.Capability != "" {
			caps[s.Capability]++
		}
	}
	out := make([]string, 0, len(caps))
	for k := range caps {
		out = append(out, k)
	}
	return tools.Result{
		Output: map[string]any{"count": len(out), "capabilities": out},
	}, nil
}

type watchdogAlertsListTool struct{}

func (watchdogAlertsListTool) Name() string       { return "watchdog_alerts_list" }
func (watchdogAlertsListTool) Capability() string { return "state:read" }
func (watchdogAlertsListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List watchdog alerts triggered — protector_burst, scanner_critical_burst, tool_call_storm rules.",
		Params: []tools.Param{
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 100)", Required: false},
		},
		Returns: "{count, alerts[]}",
	}
}

func (watchdogAlertsListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 100 {
			limit = 100
		}
	}
	items, err := store.ListWatchdogAlerts(limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list watchdog alerts: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "alerts": items},
	}, nil
}

type zombieFindingsListTool struct{}

func (zombieFindingsListTool) Name() string       { return "zombie_findings_list" }
func (zombieFindingsListTool) Capability() string { return "state:read" }
func (zombieFindingsListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List zombie code findings (Section 29) — file/symbol orphan tanpa caller. Filter by confidence.",
		Params: []tools.Param{
			{Name: "confidence", Type: tools.ParamString, Description: "high|medium|low (kosong=all)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 100)", Required: false},
		},
		Returns: "{count, findings[]}",
	}
}

func (zombieFindingsListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	confidence, _ := args["confidence"].(string)
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 100 {
			limit = 100
		}
	}
	items, err := store.ListZombieFindings(limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list zombie findings: %w", err)
	}

	if confidence != "" {
		filtered := items[:0]
		for _, f := range items {
			if f.Confidence == confidence {
				filtered = append(filtered, f)
			}
		}
		items = filtered
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "findings": items},
	}, nil
}

type personaGetTool struct{}

func (personaGetTool) Name() string       { return "persona_get" }
func (personaGetTool) Capability() string { return "state:read" }
func (personaGetTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Return current persona prompt (kv.prompt). Self-introspection: 'apa persona gw?'",
		Params:      []tools.Param{},
		Returns:     "{prompt, length}",
	}
}

func (personaGetTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	cfg, err := store.Load()
	if err != nil {
		return tools.Result{}, fmt.Errorf("load: %w", err)
	}
	prompt, _ := cfg["prompt"].(string)
	return tools.Result{
		Output: map[string]any{"prompt": prompt, "length": len(prompt)},
	}, nil
}

type decisionSearchTool struct{}

func (decisionSearchTool) Name() string       { return "decision_search" }
func (decisionSearchTool) Capability() string { return "state:read" }
func (decisionSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search decisions by decision_type substring. Recall keputusan historis.",
		Params: []tools.Param{
			{Name: "decision_type", Type: tools.ParamString, Description: "Type slug substring (mis. 'escalate' atau 'model')", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 30, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (decisionSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	dtype, _ := args["decision_type"].(string)
	limit := 30
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListDecisions("", limit*3)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list decisions: %w", err)
	}
	if dtype != "" {
		filtered := items[:0]
		dtypeLower := strings.ToLower(dtype)
		for _, d := range items {
			if strings.Contains(strings.ToLower(d.DecisionType), dtypeLower) {
				filtered = append(filtered, d)
			}
		}
		items = filtered
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

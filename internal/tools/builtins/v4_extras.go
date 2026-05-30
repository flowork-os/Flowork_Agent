// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Port batch 3 — 6 tool tambahan. Auto-register via init.
//
// v4_extras.go — 6 tool:
//   tool_audit_log, scheduler_list, mistake_search, death_letter_read,
//   workspace_lookup, system_health.

package builtins

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&toolAuditLogTool{})
	tools.Register(&schedulerListTool{})
	tools.Register(&mistakeSearchTool{})
	tools.Register(&deathLetterReadTool{})
	tools.Register(&workspaceLookupTool{})
	tools.Register(&systemHealthTool{})
}

// =============================================================================
// 1. tool_audit_log — query tool_audit table (Section 26)
// =============================================================================

type toolAuditLogTool struct{}

func (toolAuditLogTool) Name() string       { return "tool_audit_log" }
func (toolAuditLogTool) Capability() string { return "state:read" }
func (toolAuditLogTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Query tool_audit table — sandbox tool calls history (allowed/denied/pending). Filter by tool name or decision. Default limit 50 (max 200).",
		Params: []tools.Param{
			{Name: "tool_name", Type: tools.ParamString, Description: "Filter by tool name (kosong=all)", Required: false},
			{Name: "decision", Type: tools.ParamString, Description: "allowed|denied_interceptor|pending_approve (kosong=all)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max entries (default 50, max 200)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (toolAuditLogTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	toolName, _ := args["tool_name"].(string)
	decision, _ := args["decision"].(string)
	limit := 50
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 200 {
			limit = 200
		}
	}
	items, err := store.ListToolAudit(decision, toolName, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list tool audit: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

// =============================================================================
// 2. scheduler_list — list scheduled tasks (Section 18)
// =============================================================================

type schedulerListTool struct{}

func (schedulerListTool) Name() string       { return "scheduler_list" }
func (schedulerListTool) Capability() string { return "state:read" }
func (schedulerListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List schedules untuk agent ini — cron pattern + task + last_run + next_run. Useful untuk introspection: 'kapan gw jadwal next /version?'",
		Params:      []tools.Param{},
		Returns:     "{count, items[]}",
	}
}

func (schedulerListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	items, err := store.ListSchedulesForRunner()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list schedules: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

// =============================================================================
// 3. mistake_search — substring search di mistakes_local
// =============================================================================

type mistakeSearchTool struct{}

func (mistakeSearchTool) Name() string       { return "mistake_search" }
func (mistakeSearchTool) Capability() string { return "state:read" }
func (mistakeSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search mistakes_local by category atau substring di title. Anti over-prompt: tool ini on-demand, BUKAN auto-inject. Default limit 20 (max 100).",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "Filter by category (logic/halu/performance/...) atau kosong", Required: false},
			{Name: "title_substr", Type: tools.ParamString, Description: "Substring in title (case-sensitive) atau kosong", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 20, max 100)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (mistakeSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	substr, _ := args["title_substr"].(string)
	limit := 20
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 100 {
			limit = 100
		}
	}
	items, err := store.ListMistakes(category, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list mistakes: %w", err)
	}
	if substr != "" {
		filtered := make([]agentdb.Mistake, 0, len(items))
		for _, m := range items {
			if strings.Contains(m.Title, substr) {
				filtered = append(filtered, m)
			}
		}
		items = filtered
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

// =============================================================================
// 4. death_letter_read — baca wasiat pendahulu (Section 4 ADR-010)
// =============================================================================

type deathLetterReadTool struct{}

func (deathLetterReadTool) Name() string       { return "death_letter_read" }
func (deathLetterReadTool) Capability() string { return "state:read" }
func (deathLetterReadTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Baca wasiat warga sebelumnya (Predecessor Honor Protocol ADR-010). Warga baru WAJIB panggil saat boot ke workspace yg sama — biar inherit pembelajaran. Default sealed-only, limit 10.",
		Params: []tools.Param{
			{Name: "recipient", Type: tools.ParamString, Description: "Filter by recipient (default 'all')", Required: false},
			{Name: "sealed_only", Type: tools.ParamBool, Description: "Only sealed (final) letters (default true)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max (default 10, max 50)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (deathLetterReadTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	recipient, _ := args["recipient"].(string)
	if recipient == "" {
		recipient = "all"
	}
	sealedOnly := true
	if v, ok := args["sealed_only"].(bool); ok {
		sealedOnly = v
	}
	limit := 10
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 50 {
			limit = 50
		}
	}
	items, err := store.ReadLetters(recipient, sealedOnly, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("read letters: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

// =============================================================================
// 5. workspace_lookup — single workspace_meta entry by (category, path)
// =============================================================================

type workspaceLookupTool struct{}

func (workspaceLookupTool) Name() string       { return "workspace_lookup" }
func (workspaceLookupTool) Capability() string { return "state:read" }
func (workspaceLookupTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Lookup single workspace_meta entry by (category, path). Return zero-value kalau ngga ada.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "Resource category", Required: true},
			{Name: "path", Type: tools.ParamString, Description: "Relative path dari workspace root", Required: true},
		},
		Returns: "{found, item}",
	}
}

func (workspaceLookupTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	path, _ := args["path"].(string)
	if category == "" || path == "" {
		return tools.Result{}, fmt.Errorf("category + path required")
	}
	item, err := store.LookupMeta(category, path)
	if err != nil {
		return tools.Result{}, fmt.Errorf("lookup meta: %w", err)
	}
	if item.Path == "" {
		return tools.Result{
			Output: map[string]any{"found": false},
			Note:   "no matching entry",
		}, nil
	}
	return tools.Result{
		Output: map[string]any{"found": true, "item": item},
	}, nil
}

// =============================================================================
// 6. system_health — kernel runtime status snapshot
// =============================================================================

type systemHealthTool struct{}

func (systemHealthTool) Name() string       { return "system_health" }
func (systemHealthTool) Capability() string { return "time:read" }
func (systemHealthTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Snapshot system health: GOOS, GOARCH, Go version, goroutine count, mem alloc, CPU count, current time UTC. Buat self-introspection (mis. 'lo running di OS apa?').",
		Params:      []tools.Param{},
		Returns:     "{goos, goarch, go_version, num_goroutine, num_cpu, mem_alloc_mb, time_utc}",
	}
}

func (systemHealthTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return tools.Result{
		Output: map[string]any{
			"goos":          runtime.GOOS,
			"goarch":        runtime.GOARCH,
			"go_version":    runtime.Version(),
			"num_goroutine": runtime.NumGoroutine(),
			"num_cpu":       runtime.NumCPU(),
			"mem_alloc_mb":  float64(ms.Alloc) / (1024 * 1024),
			"time_utc":      time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

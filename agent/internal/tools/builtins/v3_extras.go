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
	tools.Register(&mistakeLogTool{})
	tools.Register(&interactionRecallTool{})
	tools.Register(&decisionLogTool{})
	tools.Register(&auditEventTool{})
	tools.Register(&workspaceListTool{})
	tools.Register(&karmaQueryTool{})
}

type mistakeLogTool struct{}

func (mistakeLogTool) Name() string       { return "mistake_log" }
func (mistakeLogTool) Capability() string { return "state:write" }
func (mistakeLogTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Log mistake/lesson dari halu/error agent ke mistakes_local table (Section 2). Idempotent via UNIQUE(category,title) — kalau title sama, hit_count auto-increment. Tier default 'raw' — phase 7 promote ke router brain antibody.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "logic|performance|security|halu|anti_pattern|workflow", Required: true},
			{Name: "title", Type: tools.ParamString, Description: "Short identifier (max 256 char)", Required: true},
			{Name: "content", Type: tools.ParamString, Description: "Full description (max 4KB)", Required: true},
			{Name: "context_origin", Type: tools.ParamString, Description: "Where detected (file:line atau session id)", Required: false},
		},
		Returns: "{ok, id, was_new}",
	}
}

func (mistakeLogTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	category, _ := args["category"].(string)
	title, _ := args["title"].(string)
	content, _ := args["content"].(string)
	origin, _ := args["context_origin"].(string)
	if category == "" || title == "" || content == "" {
		return tools.Result{}, fmt.Errorf("category + title + content required")
	}
	id, isNew, err := store.AddMistake(category, title, content, origin)
	if err != nil {
		return tools.Result{}, fmt.Errorf("add mistake: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id, "was_new": isNew},
		Note:   fmt.Sprintf("Mistake %s (id=%d, new=%v)", title, id, isNew),
	}, nil
}

type interactionRecallTool struct{}

func (interactionRecallTool) Name() string       { return "interaction_recall" }
func (interactionRecallTool) Capability() string { return "state:read" }
func (interactionRecallTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Query chat history dari interactions table. Anti over-prompt: tool ini on-demand only — TIDAK auto-inject ke system prompt. Default limit 10 (max 100).",
		Params: []tools.Param{
			{Name: "channel", Type: tools.ParamString, Description: "Filter by channel (telegram|router|... atau kosong=all)", Required: false},
			{Name: "actor", Type: tools.ParamString, Description: "Filter by actor/chat_id (kosong=all)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max entries (default 10, max 100)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (interactionRecallTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	channel, _ := args["channel"].(string)
	actor, _ := args["actor"].(string)
	limit := 10
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
		if limit > 100 {
			limit = 100
		}
	}
	items, err := store.ListInteractions(channel, actor, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list interactions: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type decisionLogTool struct{}

func (decisionLogTool) Name() string       { return "decision_log" }
func (decisionLogTool) Capability() string { return "state:write" }
func (decisionLogTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Log keputusan non-trivial ke decisions table. Audit trail untuk model fallback, drop chat, escalate, tool pick, dst. decision_type slug (snake_case), rationale natural language, outcome success|fail|pending.",
		Params: []tools.Param{
			{Name: "decision_type", Type: tools.ParamString, Description: "Slug (mis. model_fallback, drop_chat, escalate)", Required: true},
			{Name: "rationale", Type: tools.ParamString, Description: "Why this decision (1-3 kalimat)", Required: true},
			{Name: "outcome", Type: tools.ParamString, Description: "success|fail|pending", Required: false},
		},
		Returns: "{ok, id}",
	}
}

func (decisionLogTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	dtype, _ := args["decision_type"].(string)
	rationale, _ := args["rationale"].(string)
	outcome, _ := args["outcome"].(string)
	if dtype == "" || rationale == "" {
		return tools.Result{}, fmt.Errorf("decision_type + rationale required")
	}
	if outcome == "" {
		outcome = "pending"
	}
	id, err := store.LogDecision(dtype, rationale, outcome, nil, 0)
	if err != nil {
		return tools.Result{}, fmt.Errorf("log decision: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id},
	}, nil
}

type auditEventTool struct{}

func (auditEventTool) Name() string       { return "audit_event" }
func (auditEventTool) Capability() string { return "state:write" }
func (auditEventTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Append-only audit event log. Berbeda dari decision_log: audit untuk EXTERNAL events (login, security action, protector trigger). Severity info|warning|error|critical.",
		Params: []tools.Param{
			{Name: "event_type", Type: tools.ParamString, Description: "Slug event (mis. login_success, rate_limit_hit)", Required: true},
			{Name: "severity", Type: tools.ParamString, Description: "info|warning|error|critical (default info)", Required: false},
			{Name: "actor", Type: tools.ParamString, Description: "Who triggered (caller id/name)", Required: false},
			{Name: "detail_json", Type: tools.ParamString, Description: "JSON detail string (max 8KB)", Required: false},
		},
		Returns: "{ok, id}",
	}
}

func (auditEventTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	etype, _ := args["event_type"].(string)
	sev, _ := args["severity"].(string)
	actor, _ := args["actor"].(string)
	detail, _ := args["detail_json"].(string)
	etype = strings.TrimSpace(etype)
	if etype == "" {
		return tools.Result{}, fmt.Errorf("event_type required")
	}
	if sev == "" {
		sev = "info"
	}
	entry := agentdb.AuditEntry{
		EventType:  etype,
		Severity:   sev,
		Actor:      actor,
		DetailJSON: detail,
	}
	id, err := store.AppendAudit(entry)
	if err != nil {
		return tools.Result{}, fmt.Errorf("append audit: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"ok": true, "id": id},
	}, nil
}

type workspaceListTool struct{}

func (workspaceListTool) Name() string       { return "workspace_list" }
func (workspaceListTool) Capability() string { return "state:read" }
func (workspaceListTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "List resource entries di workspace_meta — catalog file/dir per agent. Default limit 50 (max 500). Filter by category optional.",
		Params: []tools.Param{
			{Name: "category", Type: tools.ParamString, Description: "Filter (mis. document, code, log)", Required: false},
			{Name: "limit", Type: tools.ParamInt, Description: "Max entries (default 50, max 500)", Required: false},
		},
		Returns: "{count, items[]}",
	}
}

func (workspaceListTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
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
	items, err := store.ListMeta(category, limit)
	if err != nil {
		return tools.Result{}, fmt.Errorf("list workspace_meta: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"count": len(items), "items": items},
	}, nil
}

type karmaQueryTool struct{}

func (karmaQueryTool) Name() string       { return "karma_query" }
func (karmaQueryTool) Capability() string { return "state:read" }
func (karmaQueryTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Query karma metric self — read counter or average. Pakai key kalau tau (mis. 'success_count', 'avg_response_ms'), atau biarkan kosong untuk dump semua.",
		Params: []tools.Param{
			{Name: "key", Type: tools.ParamString, Description: "Metric key (kosong = list all)", Required: false},
		},
		Returns: "{items: [{metric_key, metric_value, metric_count, updated_at}]}",
	}
}

func (karmaQueryTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	key, _ := args["key"].(string)
	key = strings.TrimSpace(key)
	if key != "" {
		k, err := store.GetKarma(key)
		if err != nil {
			return tools.Result{}, fmt.Errorf("get karma %q: %w", key, err)
		}
		return tools.Result{
			Output: map[string]any{
				"items": []agentdb.Karma{k},
				"count": 1,
			},
		}, nil
	}
	items, err := store.ListKarma()
	if err != nil {
		return tools.Result{}, fmt.Errorf("list karma: %w", err)
	}
	return tools.Result{
		Output: map[string]any{"items": items, "count": len(items)},
	}, nil
}

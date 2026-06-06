// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 11 P1 skill tools — `skill` (run by name, fetch body
//   dari Router) + `skill_search` (substring search Router catalog).
//   Reuse routerclient (Section 7 phase 2 locked). Phase 2 `skill_write`
//   (push baru ke Router) → tambah file baru, JANGAN modify ini.
//
// skill.go — Section 11 P1: skill + skill_search.
//
// SKILL EXECUTION MODEL:
//   Phase 1 = retrieve. Tool return skill body markdown sebagai output —
//   caller (LLM persona Mr.Flow) consume isi sebagai system-prompt-style
//   instruction. Tool NGGA execute kode/script — itu phase 2 territory
//   (kalau body punya `run: bash` frontmatter, tool harus dispatch).
//
// SECURITY:
//   Capability `rpc:router:skill`. Router URL per-agent dari kv (via
//   buildRouterClient pattern di routerclient.NewFromAgentURL).

package builtins

import (
	"context"
	"fmt"
	"strings"
	"time"

	"flowork-gui/internal/routerclient"
	"flowork-gui/internal/tools"
)

// =============================================================================
// skill — fetch Router skill body by name
// =============================================================================

type skillTool struct{}

func (skillTool) Name() string       { return "skill" }
func (skillTool) Capability() string { return "rpc:router:skill" }
func (skillTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Retrieve full skill (name+description+body markdown) dari Router brain catalog. Caller treat body sebagai system-prompt-style instruction. Timeout 10s. Body cap 256KB.",
		Params: []tools.Param{
			{Name: "name", Type: tools.ParamString, Description: "skill name (case-sensitive)", Required: true},
		},
		Returns: "{name, description, body}",
	}
}

func (skillTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return tools.Result{}, fmt.Errorf("name required")
	}
	client := routerClientFromCtx()
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var doc routerclient.SkillDoc
	rerr := routerclient.WithRetry(runCtx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			doc, ierr = client.GetSkill(ctx, name)
			return ierr
		})
	if rerr != nil {
		return tools.Result{}, fmt.Errorf("get skill: %w", rerr)
	}
	return tools.Result{Output: map[string]any{
		"name":        doc.Name,
		"description": doc.Description,
		"body":        doc.Body,
	}}, nil
}

// =============================================================================
// skill_search — list skill summary by substring
// =============================================================================

type skillSearchTool struct{}

func (skillSearchTool) Name() string       { return "skill_search" }
func (skillSearchTool) Capability() string { return "rpc:router:skill" }
func (skillSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search skill catalog di Router brain. Search optional (kosong=top of catalog). Returns summary (name + description) cap 10 per call (Router anti over-prompt).",
		Params: []tools.Param{
			{Name: "search", Type: tools.ParamString, Description: "optional search query"},
			{Name: "limit", Type: tools.ParamInt, Description: "default 10, max 10"},
		},
		Returns: "{items: [{name, description}], count, total}",
	}
}

func (skillSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	search, _ := args["search"].(string)
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if vs, ok := args["limit"].(int); ok && vs > 0 {
		limit = vs
	}
	if limit > 10 {
		limit = 10
	}
	client := routerClientFromCtx()
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var resp routerclient.SkillListResp
	rerr := routerclient.WithRetry(runCtx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			resp, ierr = client.ListSkills(ctx, search, limit)
			return ierr
		})
	if rerr != nil {
		return tools.Result{}, fmt.Errorf("list skills: %w", rerr)
	}
	items := make([]map[string]any, 0, len(resp.Items))
	for _, s := range resp.Items {
		items = append(items, map[string]any{
			"name":        s.Name,
			"description": s.Description,
		})
	}
	return tools.Result{Output: map[string]any{
		"items": items,
		"count": resp.Count,
		"total": resp.Total,
	}}, nil
}

// routerClientFromCtx — return Client bound ke default router. Phase 1
// constraint: tool ngga punya ctx-access ke per-agent kv.router_url
// (broker design). Pakai DefaultRouterURL + host whitelist gate. Phase 2
// → expose per-agent URL via additional ctx key.
func routerClientFromCtx() *routerclient.Client {
	// NewFromAgentURL("") → fallback ke DefaultRouterURL.
	return routerclient.NewFromAgentURL("")
}

// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 11 phase 1e (brain_search) DONE. API stable: brain_search
//   tool via routerclient.SearchBrain. Router URL resolve dari agent kv
//   config (mirror RunPromoteForAgent). k default 5, max 10 anti
//   over-prompt. Phase 1f+ brain tools (brain_recall, brain_get_drawer)
//   → tambah implementation di same file (Register di Init), JANGAN
//   modify existing function di sini.
//
// brain.go — Section 11 phase 1e: brain_search tool.
//
// Tool: brain_search — query Router brain via routerclient.SearchBrain.
// Return top-K hits dengan content + score + drawer_id.
//
// CAPABILITY: rpc:router:brain
//
// CONFIG:
//   Router URL ambil dari agent kv config (`router_url`) atau default.
//   Mirror pattern di kernelhost.RunPromoteForAgent.
//
// ⚠️ Anti over-prompt: k default 5, max 10 (overrideable via args).
// JANGAN auto-inject ke chat — caller eksplisit panggil tool dan filter
// hits relevant.

package builtins

import (
	"context"
	"fmt"

	"flowork-gui/internal/routerclient"
	"flowork-gui/internal/tools"
)

const (
	defaultBrainSearchK = 5
	maxBrainSearchK     = 10
)

// =============================================================================
// brain_search — query Router brain
// =============================================================================

type brainSearchTool struct{}

func (brainSearchTool) Name() string       { return "brain_search" }
func (brainSearchTool) Capability() string { return "rpc:router:brain" }
func (brainSearchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Search Router brain drawers via BM25/FTS rank. Return top-K hits dengan content + score + drawer_id.",
		Params: []tools.Param{
			{Name: "query", Type: tools.ParamString, Description: "search query (natural language atau keyword)", Required: true},
			{Name: "k", Type: tools.ParamInt, Description: "max hits (default 5, max 10)", Required: false, Default: defaultBrainSearchK},
		},
		Returns: "{query, hits: [{wing, room, content, score, drawer_id}], count}",
	}
}

func (brainSearchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}

	query, _ := args["query"].(string)
	if query == "" {
		return tools.Result{}, fmt.Errorf("query required")
	}

	// k normalize. JSON number masuk sebagai float64 di Go.
	k := defaultBrainSearchK
	switch v := args["k"].(type) {
	case float64:
		k = int(v)
	case int:
		k = v
	}
	if k <= 0 {
		k = defaultBrainSearchK
	}
	if k > maxBrainSearchK {
		k = maxBrainSearchK
	}

	// Resolve router URL dari agent kv (mirror RunPromoteForAgent pattern).
	routerURL := routerclient.DefaultRouterURL
	if cfg, lerr := store.Load(); lerr == nil {
		if u, ok := cfg["router_url"].(string); ok && u != "" {
			routerURL = u
		}
	}

	client := routerclient.New(routerURL)
	resp, err := client.SearchBrain(ctx, query, k)
	if err != nil {
		return tools.Result{}, fmt.Errorf("search brain: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"query": resp.Query,
		"hits":  resp.Hits,
		"count": resp.Count,
	}}, nil
}

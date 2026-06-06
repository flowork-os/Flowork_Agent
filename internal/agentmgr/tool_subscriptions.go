// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 13 phase 2 handlers — catalog/my/subscribe/unsubscribe/
//   suggest. Suggest = Router proxy + local fallback. Phase 3 (UI integration,
//   group preset, popular-share metric) → tambah file baru, JANGAN modify.
//
// tool_subscriptions.go — Section 13 phase 2 HTTP endpoints.
//
// GET  /api/agents/tools/catalog?id=&search=
// GET  /api/agents/tools/my?id=
// POST /api/agents/tools/subscribe?id=&tool=&source=
// POST /api/agents/tools/unsubscribe?id=&tool=
// POST /api/agents/tools/suggest?id= body {query, limit?}

package agentmgr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/tools"
)

// ToolCatalogHandler — GET /api/agents/tools/catalog?id=&search=.
//
// Return semua tool yang registered di registry, tag `subscribed=true|false`
// per agent. search optional substring filter (name/capability/description).
func ToolCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))

	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	subs, err := store.SubscribedSet()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

	summaries := tools.ListSummaries()
	out := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		if search != "" {
			if !strings.Contains(strings.ToLower(s.Name), search) &&
				!strings.Contains(strings.ToLower(s.Capability), search) &&
				!strings.Contains(strings.ToLower(s.Description), search) {
				continue
			}
		}
		out = append(out, map[string]any{
			"name":        s.Name,
			"capability":  s.Capability,
			"description": s.Description,
			"subscribed":  subs[s.Name],
		})
	}
	httpx.WriteJSON(w, map[string]any{
		"items":          out,
		"count":          len(out),
		"total":          len(summaries),
		"subscribed_set": len(subs),
	})
}

// ToolMyHandler — GET /api/agents/tools/my?id=. Return list yang warga aktif
// (intersect registry × subscriptions). Fields lengkap (name+cap+desc).
func ToolMyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	subs, err := store.ListSubscriptions()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	// Intersect dengan registry — yang udah ngga ada, mark inactive.
	summaries := tools.ListSummaries()
	regBy := make(map[string]tools.ToolSummary, len(summaries))
	for _, s := range summaries {
		regBy[s.Name] = s
	}
	out := make([]map[string]any, 0, len(subs))
	for _, ts := range subs {
		reg, ok := regBy[ts.ToolName]
		entry := map[string]any{
			"tool_name":     ts.ToolName,
			"subscribed_at": ts.SubscribedAt,
			"source":        ts.Source,
			"config":        ts.Config,
			"active":        ok,
		}
		if ok {
			entry["capability"] = reg.Capability
			entry["description"] = reg.Description
		}
		out = append(out, entry)
	}
	httpx.WriteJSON(w, map[string]any{
		"items": out,
		"count": len(out),
	})
}

// ToolSubscribeHandler — POST /api/agents/tools/subscribe?id=&tool=&source=
func ToolSubscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	toolName := strings.TrimSpace(r.URL.Query().Get("tool"))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if agentID == "" || toolName == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id + tool required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if err := store.SubscribeTool(toolName, source, "{}"); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "tool": toolName, "source": source})
}

// ToolUnsubscribeHandler — POST /api/agents/tools/unsubscribe?id=&tool=
func ToolUnsubscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	toolName := strings.TrimSpace(r.URL.Query().Get("tool"))
	if agentID == "" || toolName == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id + tool required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if err := store.UnsubscribeTool(toolName); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "tool": toolName})
}

// ToolSuggestHandler — POST /api/agents/tools/suggest?id= body {query, limit?}
//
// Strategy:
//   1. Try Router /api/brain/tools/suggest (kalau exist) — proxy via
//      routerclient. Future Section 6 Router tool_learner endpoint.
//   2. Fallback: local heuristic — scan registry, score by substring match
//      di name (×3) / capability (×2) / description (×1). Return top-K.
//
// Phase 2 only local heuristic (Router endpoint belum ada).
func ToolSuggestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	var body struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	q := strings.ToLower(strings.TrimSpace(body.Query))
	if q == "" {
		httpx.WriteJSON(w, map[string]any{"error": "query required"})
		return
	}
	limit := body.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	// Phase 2: Router tool_learner ngga ada — langsung local heuristic.
	suggestions := localSuggest(q, limit)

	// Phase 2 hint: kalau Router endpoint exist, panggil via routerclient
	// dengan timeout pendek (2s) — merge result. Saat ini stub return false.
	routerHit := false
	if ok := tryRouterSuggest(r.Context(), agentID, q, limit, &suggestions); ok {
		routerHit = true
	}

	// Per-agent MCP opt-out: hide tools from MCP connectors this agent unchecked.
	if hidden := hiddenMCPToolNames(agentID); len(hidden) > 0 {
		kept := suggestions[:0]
		for _, s := range suggestions {
			if !hidden[s.Name] {
				kept = append(kept, s)
			}
		}
		suggestions = kept
	}

	httpx.WriteJSON(w, map[string]any{
		"query":      body.Query,
		"items":      suggestions,
		"count":      len(suggestions),
		"router_hit": routerHit,
		"source":     map[string]any{"local": true, "router": routerHit},
	})
}

// localSuggest — substring scoring di registry. Return top-K.
type suggestEntry struct {
	Name        string  `json:"name"`
	Capability  string  `json:"capability"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
}

func localSuggest(q string, k int) []suggestEntry {
	all := tools.ListSummaries()
	scored := make([]suggestEntry, 0, len(all))
	for _, s := range all {
		var score float64
		var reasons []string
		nl := strings.ToLower(s.Name)
		cl := strings.ToLower(s.Capability)
		dl := strings.ToLower(s.Description)
		if strings.Contains(nl, q) {
			score += 3.0
			reasons = append(reasons, "name match")
		}
		if strings.Contains(cl, q) {
			score += 2.0
			reasons = append(reasons, "capability match")
		}
		if strings.Contains(dl, q) {
			score += 1.0
			reasons = append(reasons, "description match")
		}
		if score == 0 {
			continue
		}
		scored = append(scored, suggestEntry{
			Name:        s.Name,
			Capability:  s.Capability,
			Description: s.Description,
			Score:       score,
			Reason:      strings.Join(reasons, ", "),
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > k {
		scored = scored[:k]
	}
	return scored
}

// tryRouterSuggest — placeholder buat phase 3. Saat ini Router tool_learner
// endpoint belum ada — return false. Kalau Router future expose
// /api/brain/tools/suggest, helper ini panggil + merge ke suggestions slice.
func tryRouterSuggest(ctx context.Context, agentID, query string, limit int, out *[]suggestEntry) bool {
	_ = ctx
	_ = agentID
	_ = query
	_ = limit
	_ = out
	// Phase 2 stub. Phase 3 implementation:
	//   client, err := buildRouterClient(agentID)  // routerclient.NewFromAgentURL
	//   if err != nil { return false }
	//   resp, err := client.SuggestTools(ctx, query, limit)  // new method
	//   if err != nil || len(resp.Items) == 0 { return false }
	//   merge resp ke *out (dedupe by name + boost score)
	//   return true
	return false
}

// openAgentStore — shared helper. Opens *Store for agent id with default
// timeout. Caller MUST defer .Close().
func openAgentStore(agentID string) (*agentdb.Store, error) {
	// Choke-point isolation: an agent id must be well-formed before it is turned
	// into a filesystem path. Without this, a handler taking ?id= verbatim lets a
	// "../"-style id resolve a SQLite DB outside the agents folder (path traversal).
	// reID is the same shape every agent endpoint already enforces.
	if !reID.MatchString(agentID) {
		return nil, errors.New("invalid agent id")
	}
	dbPath := agentdb.Resolve(agentID, agentFolder(agentID))
	store, err := agentdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	// Sanity: set short busy_timeout via DB if needed; existing Open already
	// uses busy_timeout=5s pragma. Just return.
	_ = time.Second // keep import even if helper trivial
	_ = strconv.Itoa
	return store, nil
}

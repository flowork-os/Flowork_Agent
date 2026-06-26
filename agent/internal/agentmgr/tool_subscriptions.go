// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

	suggestions := localSuggest(q, limit)

	routerHit := false
	if ok := tryRouterSuggest(r.Context(), agentID, q, limit, &suggestions); ok {
		routerHit = true
	}

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

func tryRouterSuggest(ctx context.Context, agentID, query string, limit int, out *[]suggestEntry) bool {
	_ = ctx
	_ = agentID
	_ = query
	_ = limit
	_ = out

	return false
}

func openAgentStore(agentID string) (*agentdb.Store, error) {

	if !reID.MatchString(agentID) {
		return nil, errors.New("invalid agent id")
	}
	dbPath := agentdb.Resolve(agentID, agentFolder(agentID))
	store, err := agentdb.Open(dbPath)
	if err != nil {
		return nil, err
	}

	_ = time.Second
	_ = strconv.Itoa
	return store, nil
}

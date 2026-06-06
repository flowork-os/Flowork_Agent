// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 7 phase 2 endpoint proxy Agent → Router skill catalog.
//   API stable: GET /api/agents/router-skills/list?id=<agent>&search=
//                  &limit=, GET /api/agents/router-skills/get?id=<agent>
//                  &name=. Phase 3 (import to local skills, cache) →
//                  tambah handler baru, JANGAN modify ini.
//
// router_skills.go — Section 7 phase 2: proxy Agent UI → Router skill
// catalog. UI panggil endpoint ini (bukan langsung ke Router) — capability
// whitelist + retry policy ke-handle di routerclient.
//
// Per-agent: router_url di kv. Resolve via agentdb open.

package agentmgr

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/routerclient"
)

// RouterSkillsListHandler — GET /api/agents/router-skills/list?id=<agent>
// &search=&limit=
//
// Proxy ke Router /api/brain/skills/list via routerclient.Client (resolves
// per-agent router_url + capability whitelist + retry).
func RouterSkillsListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	limit := 10
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	client, err := buildRouterClient(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var resp routerclient.SkillListResp
	rerr := routerclient.WithRetry(ctx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			resp, ierr = client.ListSkills(ctx, search, limit)
			return ierr
		})
	if rerr != nil {
		httpx.WriteJSON(w, map[string]any{"error": "router unreachable: " + rerr.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"items": resp.Items,
		"count": resp.Count,
		"total": resp.Total,
	})
}

// RouterSkillsGetHandler — GET /api/agents/router-skills/get?id=<agent>&name=
// Proxy ke Router /api/brain/skills/get untuk fetch full skill detail.
func RouterSkillsGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if agentID == "" || name == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id + name required"})
		return
	}

	client, err := buildRouterClient(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var doc routerclient.SkillDoc
	rerr := routerclient.WithRetry(ctx, routerclient.DefaultRetry(),
		func(ctx context.Context) error {
			var ierr error
			doc, ierr = client.GetSkill(ctx, name)
			return ierr
		})
	if rerr != nil {
		httpx.WriteJSON(w, map[string]any{"error": "router error: " + rerr.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"name":        doc.Name,
		"description": doc.Description,
		"body":        doc.Body,
	})
}

// buildRouterClient — open agent state.db, lookup kv.router_url via Load(),
// return configured Client. Defaults to routerclient.DefaultRouterURL kalau
// kosong/missing.
func buildRouterClient(agentID string) (*routerclient.Client, error) {
	// Same choke-point isolation as openAgentStore: reject a malformed id before it
	// becomes a filesystem path, so ?id=../other cannot escape the agents folder.
	if !reID.MatchString(agentID) {
		return nil, fmt.Errorf("invalid agent id")
	}
	dbPath := agentdb.Resolve(agentID, agentFolder(agentID))
	store, err := agentdb.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open agent db: %w", err)
	}
	defer store.Close()

	cfg, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load cfg: %w", err)
	}
	routerURL := ""
	if router, ok := cfg["router"].(map[string]any); ok {
		if u, ok := router["url"].(string); ok {
			routerURL = u
		}
	}
	return routerclient.NewFromAgentURL(routerURL), nil
}

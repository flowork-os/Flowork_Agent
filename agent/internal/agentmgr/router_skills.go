// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

func buildRouterClient(agentID string) (*routerclient.Client, error) {

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

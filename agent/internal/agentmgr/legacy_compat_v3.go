// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/code-progress.md

package agentmgr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/tools"
)

var AgentIDsFunc func() []string

func WargaListCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	var ids []string
	if AgentIDsFunc != nil {
		ids = AgentIDsFunc()
	}
	if len(ids) == 0 {
		ids = []string{defaultAgentID}
	}
	warga := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		warga = append(warga, map[string]any{
			"name":         id,
			"display_name": id,
			"role":         "warga",
			"active":       true,
		})
	}
	httpx.WriteJSON(w, map[string]any{"warga": warga, "count": len(warga)})
}

func WargaCapsCatalogCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}

	byCat := map[string][]string{}
	order := []string{}
	for _, c := range buildToolCatalog() {
		cat, _ := c["category"].(string)
		name, _ := c["tool"].(string)
		if _, seen := byCat[cat]; !seen {
			order = append(order, cat)
		}
		byCat[cat] = append(byCat[cat], name)
	}
	sort.Strings(order)
	catalog := make([]map[string]any, 0, len(order))
	for _, cat := range order {
		tools := byCat[cat]
		sort.Strings(tools)
		catalog = append(catalog, map[string]any{"category": cat, "tools": tools})
	}
	httpx.WriteJSON(w, map[string]any{
		"catalog": catalog,
		"roles":   []string{},
		"count":   len(catalog),
	})
}

func WargaCapsEffectiveCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	warga := strings.TrimSpace(r.URL.Query().Get("warga"))
	if warga == "" {
		warga = defaultAgentID
	}
	store, err := openAgentStore(warga)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	subs, err := store.ListSubscriptions()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list subs: " + err.Error()})
		return
	}

	subSource := map[string]string{}
	for _, s := range subs {
		subSource[s.ToolName] = s.Source
	}
	catalog := buildToolCatalog()
	caps := make([]map[string]any, 0, len(catalog))
	for _, c := range catalog {
		name, _ := c["tool"].(string)
		src, subscribed := subSource[name]
		isOverride := subscribed && src != "default" && src != ""
		caps = append(caps, map[string]any{
			"tool":        name,
			"enabled":     subscribed,
			"is_override": isOverride,
		})
	}
	httpx.WriteJSON(w, map[string]any{"caps": caps, "warga": warga})
}

func WargaCapsOverrideCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var body struct {
		Warga   string `json:"warga"`
		Tool    string `json:"tool"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	if body.Warga == "" {
		body.Warga = defaultAgentID
	}
	if body.Tool == "" {
		httpx.WriteJSON(w, map[string]any{"error": "tool required"})
		return
	}
	store, err := openAgentStore(body.Warga)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if body.Enabled {
		if err := store.SubscribeTool(body.Tool, "manual", "{}"); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "subscribe: " + err.Error()})
			return
		}
	} else {
		if err := store.UnsubscribeTool(body.Tool); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "unsubscribe: " + err.Error()})
			return
		}
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":      true,
		"warga":   body.Warga,
		"tool":    body.Tool,
		"enabled": body.Enabled,
	})
}

func WargaCapsSeedCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	catalog := buildToolCatalog()
	count := 0
	for _, c := range catalog {
		name, _ := c["tool"].(string)
		if name == "" {
			continue
		}
		if err := store.SubscribeTool(name, "default", "{}"); err == nil {
			count++
		}
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "seeded": count})
}

func buildToolCatalog() []map[string]any {
	summaries := tools.ListSummaries()
	out := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, map[string]any{
			"tool":        s.Name,
			"description": s.Description,
			"category":    extractCategory(s.Capability),
		})
	}
	return out
}

func extractCategory(capability string) string {
	if idx := strings.Index(capability, ":"); idx > 0 {
		return capability[:idx]
	}
	if capability == "" {
		return "misc"
	}
	return capability
}

func CommitsCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	limit := 100
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	entries, err := store.ListAudit("", "", "", limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list audit: " + err.Error()})
		return
	}
	commits := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		commits = append(commits, map[string]any{
			"date":    e.OccurredAt,
			"author":  fallbackActor(e.Actor),
			"subject": e.EventType + " — " + truncateString(e.DetailJSON, 160),
			"hash":    formatAuditHash(e.ID),
		})
	}
	httpx.WriteJSON(w, map[string]any{"commits": commits, "count": len(commits)})
}

func fallbackActor(actor string) string {
	if strings.TrimSpace(actor) == "" {
		return "system"
	}
	return actor
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func formatAuditHash(id int64) string {
	h := fmt.Sprintf("%07x", id)
	return h
}

var _ = agentdb.AuditEntry{}

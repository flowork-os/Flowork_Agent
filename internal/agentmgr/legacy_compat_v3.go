// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Phase 2 — Tool Registry (warga_caps.js) + Audit Log (commits.js)
//   reference GUI tab shim. Extend (legacy_compat.go + _v2.go locked).
//   Single-warga BY DESIGN — semua endpoint shim ke agent_id 'mr-flow'.
//
// legacy_compat_v3.go — 2 reference GUI tab compat:
//   /api/warga-caps/{catalog,warga,effective,override,seed}
//     → backend tools.ListSummaries + store.SubscribedSet + store.SubscribeTool
//   /api/commits → backend store.ListAudit
// Shape transform per endpoint di handler.

package agentmgr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/tools"
)

// =============================================================================
// /api/warga-caps/* (reference: warga_caps.js)
// =============================================================================

// WargaListCompatHandler — GET /api/warga-caps/warga
// Reference shape: {warga: [{name, display_name, role}]}
// Single-warga BY DESIGN → return 1 entry untuk mr-flow.
func WargaListCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	httpx.WriteJSON(w, map[string]any{
		"warga": []map[string]any{
			{
				"name":         defaultAgentID,
				"display_name": "Mr.Flow",
				"role":         "owner",
			},
		},
	})
}

// WargaCapsCatalogCompatHandler — GET /api/warga-caps/catalog
// Reference shape: {catalog: [{tool, description, category}]}
// Sumber data: tools.ListSummaries() global registry.
func WargaCapsCatalogCompatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	catalog := buildToolCatalog()
	httpx.WriteJSON(w, map[string]any{
		"catalog": catalog,
		"count":   len(catalog),
	})
}

// WargaCapsEffectiveCompatHandler — GET /api/warga-caps/effective?warga=mr-flow
// Reference shape: {caps: [{tool, enabled, is_override}]}
// Sumber: store.SubscribedSet (subscription state) + tools registry catalog.
// Field enabled = tool subscribed; is_override = true kalau row di-tulis user
// (semua subscribe explicit di-flag override; seed default reset ke false).
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
	// Map tool_name → source. Source != "default" treated as override.
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

// WargaCapsOverrideCompatHandler — POST /api/warga-caps/override
// Reference body: {warga, tool, enabled}
// Subscribe (enabled=true source='manual') atau unsubscribe (delete row).
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

// WargaCapsSeedCompatHandler — POST /api/warga-caps/seed
// Reset semua override → re-subscribe semua tool registry sebagai 'default'.
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

// buildToolCatalog — internal helper. tools.ListSummaries → reference shape
// {tool, description, category}.
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

// extractCategory — split capability "exec:shell" → "exec".
func extractCategory(capability string) string {
	if idx := strings.Index(capability, ":"); idx > 0 {
		return capability[:idx]
	}
	if capability == "" {
		return "misc"
	}
	return capability
}

// =============================================================================
// /api/commits (reference: commits.js) — adapt audit log → fake git log
// =============================================================================

// CommitsCompatHandler — GET /api/commits
// Reference shape: {commits: [{date, author, subject, hash}]}
// Sumber: store.ListAudit("", "", "", 100) → AuditEntry list.
// Map:
//   date    = e.OccurredAt
//   author  = e.Actor
//   subject = e.EventType + " — " + detail_summary
//   hash    = hex(e.ID) 7-char (kayak git short hash).
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

// keep agentdb import used (struct field type reference).
var _ = agentdb.AuditEntry{}

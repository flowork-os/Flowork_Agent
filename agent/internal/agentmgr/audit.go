// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/code-progress.md

package agentmgr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

func AuditLogHandler(w http.ResponseWriter, r *http.Request) {
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
	switch r.Method {
	case http.MethodGet:
		rows, err := store.ListAudit(
			r.URL.Query().Get("type"),
			r.URL.Query().Get("from"),
			r.URL.Query().Get("to"),
			parseLimitOr(r.URL.Query().Get("limit"), 100),
		)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body agentdb.AuditEntry
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.AppendAudit(body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

var WatchdogFireFunc func() (int, int)

func WatchdogTickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	if WatchdogFireFunc == nil {
		httpx.WriteJSON(w, map[string]any{"error": "watchdog engine not wired"})
		return
	}
	evaluated, fired := WatchdogFireFunc()
	httpx.WriteJSON(w, map[string]any{
		"ok":        true,
		"evaluated": evaluated,
		"fired":     fired,
	})
}

func WatchdogAlertsHandler(w http.ResponseWriter, r *http.Request) {
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
	rows, err := store.ListWatchdogAlerts(parseLimitOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

func parseLimitOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

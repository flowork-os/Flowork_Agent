// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 22 phase 1 wallet alert endpoints. Phase 2 (cron
//   integration with Section 18 scheduler, Telegram dispatcher,
//   evaluator yang fetch portfolio sebelum compare) → tambah file baru.
//
// wallet_alert.go — Section 22 phase 1: alert config CRUD + history.

package agentmgr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// WalletAlertFireFunc — wired di main.go ke walletalert.Engine.FireNow.
// Caller (WalletAlertTickHandler) panggil untuk manual sweep.
var WalletAlertFireFunc func() (int, int)

// WalletAlertTickHandler — POST /api/agents/wallet/alerts/tick (admin sweep)
func WalletAlertTickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	if WalletAlertFireFunc == nil {
		httpx.WriteJSON(w, map[string]any{"error": "wallet alert engine not wired"})
		return
	}
	evaluated, fired := WalletAlertFireFunc()
	httpx.WriteJSON(w, map[string]any{
		"ok":        true,
		"evaluated": evaluated,
		"fired":     fired,
	})
}

// WalletAlertsHandler — GET/POST/DELETE /api/agents/wallet/alerts?id=<agent>
func WalletAlertsHandler(w http.ResponseWriter, r *http.Request) {
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
		rows, err := store.ListWalletAlerts()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body agentdb.WalletAlertConfig
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.AddWalletAlert(body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("alert_id"), 10, 64)
		if id == 0 {
			httpx.WriteJSON(w, map[string]any{"error": "alert_id required"})
			return
		}
		if err := store.DeleteWalletAlert(id); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// WalletAlertsFiredHandler — GET /api/agents/wallet/alerts/fired?id=<agent>&limit=
func WalletAlertsFiredHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListWalletAlertsFired(limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

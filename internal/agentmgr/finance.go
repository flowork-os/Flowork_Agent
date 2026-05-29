// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 23 phase 1 finance endpoints. Phase 2 (auto-ingestion
//   dari Router X-Router-Cost-Usd response header, dormancy detector,
//   ratelimit budget enforcement) → tambah file baru.
//
// finance.go — Section 23 phase 1: ledger + budget endpoints.

package agentmgr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// FinanceLedgerHandler — GET/POST /api/agents/finance/ledger?id=<agent>
//   GET  — list ?category=&from=&to=&limit=
//   POST — body FinanceLedger (insert row)
func FinanceLedgerHandler(w http.ResponseWriter, r *http.Request) {
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
		category := r.URL.Query().Get("category")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		limit := 100
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		rows, err := store.ListLedger(category, from, to, limit)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body agentdb.FinanceLedger
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		id, err := store.AddLedger(body)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "id": id})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// FinanceSummaryHandler — GET /api/agents/finance/summary?id=&from=&to=
func FinanceSummaryHandler(w http.ResponseWriter, r *http.Request) {
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
	summary, err := store.SummaryLedger(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	var total float64
	for _, s := range summary {
		total += s.CostUSD
	}
	httpx.WriteJSON(w, map[string]any{
		"by_category": summary,
		"total_usd":   total,
	})
}

// FinanceBudgetHandler — GET/POST /api/agents/finance/budget?id=<agent>
func FinanceBudgetHandler(w http.ResponseWriter, r *http.Request) {
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
		rows, err := store.ListBudgets()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body agentdb.FinanceBudget
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if err := store.SetBudget(body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "metric_key": body.MetricKey})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

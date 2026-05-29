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
	"time"

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

// FinanceCheckBudgetHandler — GET /api/agents/finance/check_budget?id=&metric_key=
// Return {allowed, current_value, budget_value, warning_pct, exceeded}
// Caller (Mr.Flow pre-LLM-call wrapper) panggil ini → kalau allowed=false → block
// + log decision "budget_exceeded".
func FinanceCheckBudgetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	metricKey := strings.TrimSpace(r.URL.Query().Get("metric_key"))
	if agentID == "" || metricKey == "" {
		httpx.WriteJSON(w, map[string]any{"error": "id + metric_key required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	budgets, err := store.ListBudgets()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	var targetBudget *agentdb.FinanceBudget
	for i := range budgets {
		if budgets[i].MetricKey == metricKey {
			targetBudget = &budgets[i]
			break
		}
	}
	if targetBudget == nil {
		// No budget configured → always allowed.
		httpx.WriteJSON(w, map[string]any{
			"allowed":      true,
			"reason":       "no budget configured",
			"metric_key":   metricKey,
			"current_value": 0,
		})
		return
	}
	if !targetBudget.Enabled {
		httpx.WriteJSON(w, map[string]any{
			"allowed":      true,
			"reason":       "budget disabled",
			"metric_key":   metricKey,
			"current_value": 0,
			"budget_value":  targetBudget.BudgetValue,
		})
		return
	}

	// Sum cost_usd today (UTC).
	from := time.Now().UTC().Format("2006-01-02") + "T00:00:00Z"
	to := time.Now().UTC().Format(time.RFC3339)
	summary, err := store.SummaryLedger(from, to)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	var totalToday float64
	for _, s := range summary {
		totalToday += s.CostUSD
	}

	exceeded := totalToday >= targetBudget.BudgetValue
	atWarning := totalToday >= targetBudget.BudgetValue*targetBudget.WarningAtPct

	allowed := !exceeded
	reason := ""
	if exceeded {
		reason = "daily budget exceeded"
		// Log decision.
		inputs := map[string]any{
			"metric_key":    metricKey,
			"current_value": totalToday,
			"budget_value":  targetBudget.BudgetValue,
		}
		_, _ = store.LogDecision("budget_exceeded", reason, "fail", inputs, 0)
	} else if atWarning {
		reason = "at warning threshold"
	}

	httpx.WriteJSON(w, map[string]any{
		"allowed":       allowed,
		"reason":        reason,
		"metric_key":    metricKey,
		"current_value": totalToday,
		"budget_value":  targetBudget.BudgetValue,
		"warning_pct":   targetBudget.WarningAtPct,
		"exceeded":      exceeded,
		"at_warning":    atWarning,
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

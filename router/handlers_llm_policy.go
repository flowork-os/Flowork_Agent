// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Pricing → dok lock/gui/Pricing.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func ProviderChainsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, qerr := db.Query(`SELECT id, chain_name, providers_json, created_at FROM provider_chain_configs ORDER BY id`)
		if qerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var name, pj, ts string
			_ = rows.Scan(&id, &name, &pj, &ts)
			out = append(out, map[string]any{
				"id": id, "chain_name": name, "providers_json": pj, "created_at": ts,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
	case http.MethodPost:
		var body struct {
			ChainName     string `json:"chain_name"`
			ProvidersJSON string `json:"providers_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if body.ChainName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chain_name required"})
			return
		}
		if body.ProvidersJSON == "" {
			body.ProvidersJSON = "[]"
		}
		res, err := db.Exec(
			`INSERT INTO provider_chain_configs (chain_name, providers_json) VALUES (?, ?)
			 ON CONFLICT(chain_name) DO UPDATE SET providers_json = excluded.providers_json`,
			body.ChainName, body.ProvidersJSON,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func ProviderCallsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 {
			limit = n
		}
	}
	query := `SELECT id, caller, provider, model, input_tokens, output_tokens,
	                 cost_usd, latency_ms, status, occurred_at
	          FROM provider_call_log WHERE 1=1`
	args := []any{}
	if from := r.URL.Query().Get("from"); from != "" {
		query += ` AND occurred_at >= ?`
		args = append(args, from)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		query += ` AND occurred_at <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, qerr := db.Query(query, args...)
	if qerr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var inputT, outputT, latencyMS int64
		var costUSD float64
		var caller, provider, model, status, ts string
		_ = rows.Scan(&id, &caller, &provider, &model, &inputT, &outputT, &costUSD, &latencyMS, &status, &ts)
		out = append(out, map[string]any{
			"id": id, "caller": caller, "provider": provider, "model": model,
			"input_tokens": inputT, "output_tokens": outputT,
			"cost_usd": costUSD, "latency_ms": latencyMS,
			"status": status, "occurred_at": ts,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
}

func LocalAIModelsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, qerr := db.Query(
			`SELECT id, model_name, gguf_path, size_bytes, checksum, manifest_json, signature, loaded, added_at
			 FROM localai_models ORDER BY id`)
		if qerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, size int64
			var name, ggufPath, checksum, manifest, sig, addedAt string
			var loaded int
			_ = rows.Scan(&id, &name, &ggufPath, &size, &checksum, &manifest, &sig, &loaded, &addedAt)
			out = append(out, map[string]any{
				"id": id, "model_name": name, "gguf_path": ggufPath,
				"size_bytes": size, "checksum": checksum,
				"manifest_json": manifest, "signature": sig,
				"loaded": loaded != 0, "added_at": addedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
	case http.MethodPost:
		var body struct {
			ModelName    string `json:"model_name"`
			GGUFPath     string `json:"gguf_path"`
			SizeBytes    int64  `json:"size_bytes"`
			Checksum     string `json:"checksum"`
			ManifestJSON string `json:"manifest_json"`
			Signature    string `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if body.ModelName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "model_name required"})
			return
		}
		if body.ManifestJSON == "" {
			body.ManifestJSON = "{}"
		}
		res, err := db.Exec(
			`INSERT INTO localai_models (model_name, gguf_path, size_bytes, checksum, manifest_json, signature)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(model_name) DO UPDATE SET
			   gguf_path = excluded.gguf_path,
			   size_bytes = excluded.size_bytes,
			   checksum = excluded.checksum,
			   manifest_json = excluded.manifest_json,
			   signature = excluded.signature`,
			body.ModelName, body.GGUFPath, body.SizeBytes, body.Checksum, body.ManifestJSON, body.Signature,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("model_name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "model_name required"})
			return
		}
		res, derr := db.Exec(`DELETE FROM localai_models WHERE model_name = ?`, name)
		if derr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": derr.Error()})
			return
		}
		n, _ := res.RowsAffected()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func PricingRulesHandler(w http.ResponseWriter, r *http.Request) {
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, qerr := db.Query(
			`SELECT id, rule_name, provider, model, tier,
			        input_per_1m_usd, output_per_1m_usd, enabled, notes
			 FROM pricing_rules ORDER BY id`)
		if qerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var ruleName, provider, model, tier, notes string
			var inputPer, outputPer float64
			var enabled int
			_ = rows.Scan(&id, &ruleName, &provider, &model, &tier, &inputPer, &outputPer, &enabled, &notes)
			out = append(out, map[string]any{
				"id": id, "rule_name": ruleName, "provider": provider, "model": model,
				"tier": tier, "input_per_1m_usd": inputPer, "output_per_1m_usd": outputPer,
				"enabled": enabled != 0, "notes": notes,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
	case http.MethodPost:
		var body struct {
			RuleName       string  `json:"rule_name"`
			Provider       string  `json:"provider"`
			Model          string  `json:"model"`
			Tier           string  `json:"tier"`
			InputPer1MUSD  float64 `json:"input_per_1m_usd"`
			OutputPer1MUSD float64 `json:"output_per_1m_usd"`
			Notes          string  `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if body.Provider == "" || body.Model == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "provider + model required"})
			return
		}
		if body.Tier == "" {
			body.Tier = "default"
		}
		res, err := db.Exec(
			`INSERT INTO pricing_rules (rule_name, provider, model, tier,
			   input_per_1m_usd, output_per_1m_usd, notes)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(provider, model, tier) DO UPDATE SET
			   rule_name = excluded.rule_name,
			   input_per_1m_usd = excluded.input_per_1m_usd,
			   output_per_1m_usd = excluded.output_per_1m_usd,
			   notes = excluded.notes`,
			body.RuleName, body.Provider, body.Model, body.Tier,
			body.InputPer1MUSD, body.OutputPer1MUSD, body.Notes,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func PolicyBudgetsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, qerr := db.Query(
			`SELECT id, scope, scope_key, metric_key, budget_value,
			        reset_period, warning_pct, enabled, created_at
			 FROM policy_budgets ORDER BY id`)
		if qerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var scope, scopeKey, metricKey, resetPeriod, ts string
			var budgetValue, warningPct float64
			var enabled int
			_ = rows.Scan(&id, &scope, &scopeKey, &metricKey, &budgetValue,
				&resetPeriod, &warningPct, &enabled, &ts)
			out = append(out, map[string]any{
				"id": id, "scope": scope, "scope_key": scopeKey,
				"metric_key": metricKey, "budget_value": budgetValue,
				"reset_period": resetPeriod, "warning_pct": warningPct,
				"enabled": enabled != 0, "created_at": ts,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
	case http.MethodPost:
		var body struct {
			Scope       string  `json:"scope"`
			ScopeKey    string  `json:"scope_key"`
			MetricKey   string  `json:"metric_key"`
			BudgetValue float64 `json:"budget_value"`
			ResetPeriod string  `json:"reset_period"`
			WarningPct  float64 `json:"warning_pct"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		body.Scope = strings.TrimSpace(body.Scope)
		body.MetricKey = strings.TrimSpace(body.MetricKey)
		if body.Scope == "" || body.MetricKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "scope + metric_key required"})
			return
		}
		if body.ResetPeriod == "" {
			body.ResetPeriod = "daily"
		}
		if body.WarningPct <= 0 {
			body.WarningPct = 0.8
		}
		res, err := db.Exec(
			`INSERT INTO policy_budgets (scope, scope_key, metric_key, budget_value,
			   reset_period, warning_pct)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(scope, scope_key, metric_key) DO UPDATE SET
			   budget_value = excluded.budget_value,
			   reset_period = excluded.reset_period,
			   warning_pct = excluded.warning_pct`,
			body.Scope, body.ScopeKey, body.MetricKey, body.BudgetValue,
			body.ResetPeriod, body.WarningPct,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

var policyEngineRef interface {
	FireNow(ctx context.Context) (int, int)
}

func PolicyTickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if policyEngineRef == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "policy engine not wired"})
		return
	}
	evaluated, fired := policyEngineRef.FireNow(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"evaluated": evaluated,
		"fired":     fired,
	})
}

func PolicyViolationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil && n > 0 {
			limit = n
		}
	}
	rows, qerr := db.Query(
		`SELECT id, budget_id, fired_at, actual_value, action_taken
		 FROM policy_violations ORDER BY id DESC LIMIT ?`, limit)
	if qerr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": qerr.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, budgetID int64
		var actualValue float64
		var firedAt, action string
		_ = rows.Scan(&id, &budgetID, &firedAt, &actualValue, &action)
		out = append(out, map[string]any{
			"id": id, "budget_id": budgetID, "fired_at": firedAt,
			"actual_value": actualValue, "action_taken": action,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
}

var _ = sql.ErrNoRows
var _ = time.Now

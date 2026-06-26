// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func usageBreakdownRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/usage/")
	switch rest {
	case "today":
		usageTodayHandler(w, r)
		return
	case "chart":
		usageChartHandler(w, r)
		return
	case "history":
		usageHistoryHandler(w, r)
		return
	case "providers":
		usageProvidersHandler(w, r)
		return
	case "request-details":
		usageRequestDetailsHandler(w, r)
		return
	case "request-logs":
		usageRequestLogsHandler(w, r)
		return
	case "stats":
		usageStatsHandler(w, r)
		return
	case "stream":
		usageStreamHandler(w, r)
		return
	}

	if rest != "" {
		usageByConnectionHandler(w, r, rest)
		return
	}
	usageHandler(w, r)
}

func usageChartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 366 {
			days = n
		}
	}
	rows, err := d.Query(`
		SELECT day, SUM(requestCount), SUM(promptTokens), SUM(completionTokens), SUM(costUsd)
		FROM usageDaily
		WHERE day >= date('now', '-` + fmt.Sprintf("%d", days) + ` days')
		GROUP BY day
		ORDER BY day ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var series []map[string]any
	for rows.Next() {
		var day string
		var req, prompt, compl int
		var cost float64
		if err := rows.Scan(&day, &req, &prompt, &compl, &cost); err != nil {
			continue
		}
		series = append(series, map[string]any{
			"day":              day,
			"requestCount":     req,
			"promptTokens":     prompt,
			"completionTokens": compl,
			"totalTokens":      prompt + compl,
			"costUsd":          cost,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series": series,
		"days":   days,
	})
}

func usageHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	provider := r.URL.Query().Get("provider")
	q := `SELECT id, ts, provider, model, COALESCE(apiKeyId, ''), promptTokens, completionTokens, costUsd, latencyMs, status FROM usageHistory`
	args := []any{}
	if provider != "" {
		q += ` WHERE provider = ?`
		args = append(args, provider)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := d.Query(q, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int
		var ts, provider, model, apiKeyId, status string
		var prompt, compl int
		var cost float64
		var lat int
		if err := rows.Scan(&id, &ts, &provider, &model, &apiKeyId, &prompt, &compl, &cost, &lat, &status); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id":               id,
			"ts":               ts,
			"provider":         provider,
			"model":            model,
			"apiKeyId":         apiKeyId,
			"promptTokens":     prompt,
			"completionTokens": compl,
			"costUsd":          cost,
			"latencyMs":        lat,
			"status":           status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   out,
		"count":  len(out),
		"limit":  limit,
		"offset": offset,
	})
}

func usageProvidersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	rows, err := d.Query(`
		SELECT provider, COUNT(*) cnt, SUM(promptTokens), SUM(completionTokens),
		       SUM(costUsd), AVG(latencyMs), MAX(ts)
		FROM usageHistory
		GROUP BY provider
		ORDER BY cnt DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var provider, lastSeen string
		var cnt, prompt, compl int
		var cost, latAvg float64
		if err := rows.Scan(&provider, &cnt, &prompt, &compl, &cost, &latAvg, &lastSeen); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"provider":         provider,
			"requests":         cnt,
			"promptTokens":     prompt,
			"completionTokens": compl,
			"totalTokens":      prompt + compl,
			"costUsd":          cost,
			"avgLatencyMs":     latAvg,
			"lastSeen":         lastSeen,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "count": len(out)})
}

func usageRequestDetailsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	row := d.QueryRow(`SELECT id, ts, COALESCE(apiKeyId, ''), COALESCE(providerId, ''),
		COALESCE(model, ''), COALESCE(clientIp, ''), COALESCE(clientUA, ''),
		COALESCE(requestBody, ''), COALESCE(responseBody, ''), statusCode,
		COALESCE(error, ''), durationMs
		FROM requestDetails WHERE id = ?`, id)
	var rd struct {
		ID         int
		Ts, APIKey string
		ProviderID string
		Model      string
		ClientIP   string
		ClientUA   string
		ReqBody    string
		RespBody   string
		StatusCode int
		Error      string
		DurationMs int
	}
	if err := row.Scan(&rd.ID, &rd.Ts, &rd.APIKey, &rd.ProviderID, &rd.Model, &rd.ClientIP, &rd.ClientUA, &rd.ReqBody, &rd.RespBody, &rd.StatusCode, &rd.Error, &rd.DurationMs); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           rd.ID,
		"ts":           rd.Ts,
		"apiKeyId":     rd.APIKey,
		"providerId":   rd.ProviderID,
		"model":        rd.Model,
		"clientIp":     rd.ClientIP,
		"clientUA":     rd.ClientUA,
		"requestBody":  rd.ReqBody,
		"responseBody": rd.RespBody,
		"statusCode":   rd.StatusCode,
		"error":        rd.Error,
		"durationMs":   rd.DurationMs,
	})
}

func usageRequestLogsHandler(w http.ResponseWriter, r *http.Request) {
	consoleLogHandler(w, r)
}

func usageStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	var totalReq, totalPrompt, totalCompl, errCount int
	var totalCost float64
	_ = d.QueryRow(`SELECT COUNT(*), SUM(promptTokens), SUM(completionTokens), SUM(costUsd) FROM usageHistory`).
		Scan(&totalReq, &totalPrompt, &totalCompl, &totalCost)
	_ = d.QueryRow(`SELECT COUNT(*) FROM usageHistory WHERE status != 'ok' AND status != ''`).Scan(&errCount)
	var providerCount int
	_ = d.QueryRow(`SELECT COUNT(DISTINCT provider) FROM usageHistory`).Scan(&providerCount)
	writeJSON(w, http.StatusOK, map[string]any{
		"totalRequests":         totalReq,
		"totalPromptTokens":     totalPrompt,
		"totalCompletionTokens": totalCompl,
		"totalTokens":           totalPrompt + totalCompl,
		"totalCostUsd":          totalCost,
		"errorCount":            errCount,
		"providerCount":         providerCount,
	})
}

func usageStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	d, _ := store.Open()
	var lastID int
	_ = d.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM usageHistory`).Scan(&lastID)

	fmt.Fprintf(w, "event: ready\ndata: {\"lastId\":%d}\n\n", lastID)
	flusher.Flush()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := d.Query(`SELECT id, ts, provider, model, COALESCE(apiKeyId, ''),
				promptTokens, completionTokens, costUsd, latencyMs, status
				FROM usageHistory WHERE id > ? ORDER BY id ASC LIMIT 50`, lastID)
			if err != nil {
				continue
			}
			anyEmitted := streamUsageHistoryRows(w, rows, &lastID)
			if anyEmitted {
				flusher.Flush()
			}
		}
	}
}

func streamUsageHistoryRows(w http.ResponseWriter, rows *sql.Rows, lastID *int) bool {
	defer rows.Close()
	anyEmitted := false
	for rows.Next() {
		var id int
		var ts, provider, model, apiKey, status string
		var prompt, compl int
		var cost float64
		var lat int
		if err := rows.Scan(&id, &ts, &provider, &model, &apiKey, &prompt, &compl, &cost, &lat, &status); err != nil {
			continue
		}
		if id > *lastID {
			*lastID = id
		}
		fmt.Fprintf(w, "event: usage\ndata: {\"id\":%d,\"ts\":%q,\"provider\":%q,\"model\":%q,\"promptTokens\":%d,\"completionTokens\":%d,\"costUsd\":%v,\"latencyMs\":%d,\"status\":%q}\n\n",
			id, ts, provider, model, prompt, compl, cost, lat, status)
		_ = apiKey
		anyEmitted = true
	}
	return anyEmitted
}

func usageByConnectionHandler(w http.ResponseWriter, r *http.Request, connID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	row := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(promptTokens),0), COALESCE(SUM(completionTokens),0),
		COALESCE(SUM(costUsd),0), COALESCE(AVG(latencyMs),0), MAX(ts) FROM usageHistory WHERE provider = ?`, connID)
	var cnt, prompt, compl int
	var cost, lat float64
	var lastSeen sql.NullString
	if err := row.Scan(&cnt, &prompt, &compl, &cost, &lat, &lastSeen); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connectionId":     connID,
		"requests":         cnt,
		"promptTokens":     prompt,
		"completionTokens": compl,
		"totalTokens":      prompt + compl,
		"costUsd":          cost,
		"avgLatencyMs":     lat,
		"lastSeen":         lastSeen.String,
	})
}

// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	mitmCaptureMu      sync.RWMutex
	mitmCaptureEnabled bool
)

func MITMCaptureEnabled() bool {
	mitmCaptureMu.RLock()
	defer mitmCaptureMu.RUnlock()
	return mitmCaptureEnabled
}

func mitmCaptureToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	mitmCaptureMu.Lock()
	mitmCaptureEnabled = body.Enabled
	mitmCaptureMu.Unlock()

	d, _ := store.Open()
	_ = store.SaveTunnelState(d, &store.TunnelState{})
	_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES ('mitm:capture','` +
		map[bool]string{true: "true", false: "false"}[body.Enabled] +
		`', datetime('now')) ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": body.Enabled})
}

func mitmFullDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/mitm/full/")
	if id == "" || id == r.URL.Path {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	r.URL.RawQuery = "id=" + id
	usageRequestDetailsHandler(w, r)
}

func mitmRecentFullHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	d, _ := store.Open()
	rows, err := d.Query(`SELECT id, ts, COALESCE(providerId, ''), COALESCE(model, ''),
		statusCode, COALESCE(error, ''), durationMs,
		LENGTH(COALESCE(requestBody, '')) reqLen,
		LENGTH(COALESCE(responseBody, '')) respLen
		FROM requestDetails ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, status, dur, reqLen, respLen int
		var ts, providerID, model, errStr string
		if err := rows.Scan(&id, &ts, &providerID, &model, &status, &errStr, &dur, &reqLen, &respLen); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id":          id,
			"ts":          ts,
			"providerId":  providerID,
			"model":       model,
			"statusCode":  status,
			"error":       errStr,
			"durationMs":  dur,
			"reqBodyLen":  reqLen,
			"respBodyLen": respLen,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":           out,
		"count":          len(out),
		"captureEnabled": MITMCaptureEnabled(),
	})
}

func loadMITMCaptureState() {
	d, err := store.Open()
	if err != nil {
		return
	}
	var v string
	if err := d.QueryRow(`SELECT v FROM kv WHERE k = 'mitm:capture'`).Scan(&v); err == nil {
		mitmCaptureMu.Lock()
		mitmCaptureEnabled = v == "true"
		mitmCaptureMu.Unlock()
	}
}

func recordMITMRequest(providerID, model, clientIP, clientUA string, reqBody []byte, statusCode int, errMsg string, durationMs int64, respBody []byte) {
	d, err := store.Open()
	if err != nil {
		return
	}
	const maxBody = 256 * 1024
	trunc := func(b []byte) string {
		if len(b) > maxBody {
			return string(b[:maxBody]) + "\n…[truncated]"
		}
		return string(b)
	}
	_, _ = d.Exec(`INSERT INTO requestDetails (ts, providerId, model, clientIp, clientUA,
		requestBody, responseBody, statusCode, error, durationMs)
		VALUES (datetime('now'), ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		providerID, model, clientIP, clientUA,
		trunc(reqBody), trunc(respBody), statusCode, errMsg, durationMs)
}

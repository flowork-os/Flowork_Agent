// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Phase 1 — Doktrin Edukasi compat shim (Section 9). Multi-agent
//   karma/topology reference NOT applicable di Mr.Flow single-warga
//   plug-and-play (BY DESIGN). Phase 2 (multi-warga support kalau warga
//   baru spawn) → tambah file baru.
//
// legacy_compat_v2.go — Section 9 (Doktrin Edukasi) reference GUI tab
// shim. Extend (legacy_compat.go locked). Maps reference path:
//   /api/settings/educational-errors  →  /api/agents/edu-errors?id=mr-flow
// + shape transform: {data: [{error_code, title, message_template,
// evolution_hint}]} ↔ backend {items: [{code, title, explanation,
// remediation, ...}]}.

package agentmgr

import (
	"encoding/json"
	"net/http"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// EduErrorsCompatHandler — GET/PUT /api/settings/educational-errors
//
//	GET  → reference shape {data: [{error_code, title, message_template,
//	       evolution_hint}], count}
//	PUT  → body {error_code, message_template, evolution_hint}.
//	       Preserve title + category by re-reading existing entry.
func EduErrorsCompatHandler(w http.ResponseWriter, r *http.Request) {
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	switch r.Method {
	case http.MethodGet:
		items, err := store.ListEduErrors("", 500)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
			return
		}
		data := make([]map[string]any, 0, len(items))
		for _, e := range items {
			data = append(data, map[string]any{
				"error_code":       e.Code,
				"title":            e.Title,
				"message_template": e.Explanation,
				"evolution_hint":   e.Remediation,
				"category":         e.Category,
				"synced_at":        e.SyncedAt,
			})
		}
		httpx.WriteJSON(w, map[string]any{"data": data, "count": len(data)})

	case http.MethodPut, http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var body struct {
			ErrorCode       string `json:"error_code"`
			MessageTemplate string `json:"message_template"`
			EvolutionHint   string `json:"evolution_hint"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		if body.ErrorCode == "" {
			httpx.WriteJSON(w, map[string]any{"error": "error_code required"})
			return
		}
		// Preserve title + category dari existing entry (reference cuma
		// edit message + hint, title locked di seed source).
		existing, lookupErr := store.LookupEduError(body.ErrorCode)
		if lookupErr != nil {
			httpx.WriteJSON(w, map[string]any{"error": "lookup: " + lookupErr.Error()})
			return
		}
		merged := agentdb.EduError{
			Code:        body.ErrorCode,
			Category:    existing.Category,
			Title:       existing.Title,
			Explanation: body.MessageTemplate,
			Remediation: body.EvolutionHint,
		}
		if err := store.UpsertEduError(merged); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "upsert: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "error_code": body.ErrorCode})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

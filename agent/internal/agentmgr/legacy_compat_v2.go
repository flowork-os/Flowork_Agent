// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"net/http"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

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

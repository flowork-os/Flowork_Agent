// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: Connections HTTP API (Phase 1). Stable endpoint surface for the GUI.
//
// handlers.go — HTTP face of the Connections registry. Owner-gated (mounted behind
// the same auth middleware as the rest of /api). Install reuses the uniform .fwpack
// gerbang (kind:channel dispatch in plugin_handler.go); these handlers cover the
// rest of a connector's lifecycle: list · toggle · config · uninstall.
package connections

import (
	"encoding/json"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status != 0 {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(body)
}

// ListHandler — GET /api/connections → every installed connector + its status.
func ListHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 0, map[string]any{"connectors": List()})
}

// ToggleHandler — POST /api/connections/toggle {id, enabled}. Enable/disable a
// connector by its in-folder marker.
func ToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	if err := SetEnabled(strings.TrimSpace(body.ID), body.Enabled); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 0, map[string]any{"ok": true, "id": body.ID, "enabled": body.Enabled})
}

// UninstallHandler — POST /api/connections/uninstall {id}. Removes the connector's
// folder — and with it everything the connector owned (config + token).
func UninstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	if err := Uninstall(strings.TrimSpace(body.ID)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 0, map[string]any{"ok": true, "uninstalled": body.ID})
}

// ConfigHandler — GET /api/connections/config?id= → masked config;
// POST /api/connections/config {id, config:{...}} → merge into the connector's own
// folder. Credentials are masked on GET; the real value stays in the folder.
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		cfg, err := GetConfigMasked(id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 0, map[string]any{"id": id, "config": cfg})
	case http.MethodPost:
		var body struct {
			ID     string            `json:"id"`
			Config map[string]string `json:"config"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		if err := SetConfig(strings.TrimSpace(body.ID), body.Config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 0, map[string]any{"ok": true, "id": body.ID})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET or POST"})
	}
}

// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: MCP hub HTTP API (Phase 2). Owner-gated /api/mcp endpoints.
//
// handlers.go — HTTP face of the MCP connector hub. Owner-gated (behind the same
// auth middleware as the rest of /api): installing an MCP server + supplying its
// token is a high-risk, owner-only action.
package mcphub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status != 0 {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(body)
}

// ListHandler — GET /api/mcp → installed MCP connectors (+ live tools, env names).
func ListHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 0, map[string]any{"connectors": Default.List()})
}

// InstallHandler — POST /api/mcp/install {id, command, args, env}. Stores the server
// spec in the connector's own folder. The owner pastes the standard mcpServers shape.
func InstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		ID      string            `json:"id"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "decode: " + err.Error()})
		return
	}
	if err := Install(strings.TrimSpace(body.ID), SavedConfig{Command: strings.TrimSpace(body.Command), Args: body.Args, Env: body.Env}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 0, map[string]any{"ok": true, "id": body.ID})
}

// idAction handles {id}-only POSTs (enable/disable/uninstall).
func idAction(w http.ResponseWriter, r *http.Request, fn func(id string) error) {
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
	if err := fn(strings.TrimSpace(body.ID)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 0, map[string]any{"ok": true, "id": body.ID})
}

// EnableHandler — POST /api/mcp/enable {id}. Spawns the server + registers its tools.
func EnableHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, func(id string) error {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		return Default.Enable(ctx, id)
	})
}

// DisableHandler — POST /api/mcp/disable {id}. Unregisters tools + reaps the process.
func DisableHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, Default.Disable)
}

// UninstallHandler — POST /api/mcp/uninstall {id}. Removes the connector entirely.
func UninstallHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, Default.Uninstall)
}

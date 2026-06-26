// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

func ListHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 0, map[string]any{"connectors": Default.List()})
}

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

func EnableHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, func(id string) error {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		return Default.Enable(ctx, id)
	})
}

func DisableHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, Default.Disable)
}

func UninstallHandler(w http.ResponseWriter, r *http.Request) {
	idAction(w, r, Default.Uninstall)
}

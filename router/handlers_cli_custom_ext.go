// handlers_cli_custom_ext.go — CRUD CLI tool custom dari GUI, via seam routes_ext.go.
//   GET    /api/cli-tools/custom        → list custom (DB)
//   POST   /api/cli-tools/custom {Tool} → tambah (DB + live registry)
//   DELETE /api/cli-tools/custom/<id>   → hapus (DB; drop penuh saat restart)
// NON-frozen, deletable.
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/clitools"
)

func init() {
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/cli-tools/custom", cliCustomHandler)
		mux.HandleFunc("/api/cli-tools/custom/", cliCustomHandler)
	})
}

func cliCustomHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"data": clitools.LoadCustomCLITools()})
	case http.MethodPost:
		var t clitools.Tool
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "parse: " + err.Error()})
			return
		}
		t.ID = strings.TrimSpace(t.ID)
		if t.ID == "" || strings.TrimSpace(t.BinaryName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id & binaryName wajib"})
			return
		}
		if t.Format == "" {
			t.Format = clitools.FormatEnv
		}
		if err := clitools.RegisterCustomCLITool(t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": t.ID})
	case http.MethodDelete:
		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/cli-tools/custom/"))
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id wajib"})
			return
		}
		if err := clitools.DeleteCustomCLITool(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "note": "drop penuh dari registry saat restart"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

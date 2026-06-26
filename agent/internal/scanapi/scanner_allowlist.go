// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

package scanapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"flowork-gui/internal/floworkdb"
)

func ScannerAllowlistHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			kind := strings.TrimSpace(r.URL.Query().Get("kind"))
			list, err := store.ListAllowlist(kind)
			if err != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"allowlist": list, "count": len(list)})
		case http.MethodPost:
			var body struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
				Note  string `json:"note"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
				return
			}
			if err := store.AddAllowlist(body.Kind, body.Value, body.Note); err != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			tfWriteJSON(w, 0, map[string]any{"ok": true, "kind": body.Kind, "value": body.Value})
		default:
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET/POST only"})
		}
	}
}

func ScannerAllowlistCheckHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		value := strings.TrimSpace(r.URL.Query().Get("value"))
		ok, err := store.IsAllowed(kind, value)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"allowed": ok, "kind": kind, "value": value})
	}
}

func ScannerAllowlistDeleteHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		if err := store.RemoveAllowlist(body.Kind, body.Value); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "removed": body.Value})
	}
}

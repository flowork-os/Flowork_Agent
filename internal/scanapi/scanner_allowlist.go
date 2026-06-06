// scanner_allowlist.go — endpoint ALLOWLIST scanner (owner-editable, agent-locked).
//
//	GET  /api/scanner/allowlist?kind=exec|target   → list
//	POST /api/scanner/allowlist  {kind,value,note}  → add/update
//	POST /api/scanner/allowlist/delete {kind,value} → remove
//
// Loopback-only owner-local. AGENT ga pernah dikasih akses endpoint ini (ga ada
// di cap net:fetch agent) → cuma OWNER (GUI/CLI) yang bisa edit gerbang.

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

// ScannerAllowlistCheckHandler — GET ?kind=&value= → {allowed}. Gerbang IsAllowed
// (owner verifikasi scope sebelum scan; nanti dipake scan tool internal).
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

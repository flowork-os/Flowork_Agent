// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Tags → dok lock/gui/Tags.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func tagsHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListTags(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var t store.Tag
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpsertTag(d, &t); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func tagCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	id := r.URL.Path[len("/api/tags/"):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var t store.Tag
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		t.ID = id
		if err := store.UpsertTag(d, &t); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if err := store.DeleteTag(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

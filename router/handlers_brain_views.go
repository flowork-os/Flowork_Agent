// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func brainIngestRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		http.Error(w, "brain DB not available", http.StatusServiceUnavailable)
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 500)
	contribs, err := store.ListBrainContributions(d, true, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var added, deduped, skipped int
	var maxID int64
	for _, c := range contribs {
		if c.ID > maxID {
			maxID = c.ID
		}
		answer := strings.TrimSpace(c.Answer)
		if len(answer) < 20 {
			skipped++
			continue
		}
		content := "Q: " + strings.TrimSpace(c.Query) + "\n\nA: " + answer
		_, wasNew, err := brain.AddDrawer(r.Context(), content, "compounding", c.Agent, "compounding")
		if err != nil {
			skipped++
			continue
		}
		if wasNew {
			added++
		} else {
			deduped++
		}
	}
	if maxID > 0 {
		_, _ = store.MarkContributionsIngested(d, maxID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"processed": len(contribs), "added": added, "deduped": deduped, "skipped": skipped, "markedIngested": maxID > 0,
	})
}

func brainAddDrawerHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	switch r.Method {
	case http.MethodPost:
		var b struct {
			Content string `json:"content"`
			Wing    string `json:"wing"`
			Room    string `json:"room"`
			MemType string `json:"memType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(b.Content) == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		id, wasNew, err := brain.AddDrawer(r.Context(), b.Content, b.Wing, b.Room, b.MemType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "added": wasNew})
	case http.MethodPut:
		var b struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Wing    string `json:"wing"`
			Room    string `json:"room"`
			MemType string `json:"memType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := brain.UpdateDrawer(r.Context(), b.ID, b.Content, b.Wing, b.Room, b.MemType); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": b.ID})
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := brain.SoftDeleteDrawer(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func brainInitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	created, err := brain.EnsureSchema()
	if err != nil {
		http.Error(w, "init: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": created, "path": brain.DBPath(), "available": brain.Available(),
	})
}

func brainSearchDrawersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		http.Error(w, "query param required", http.StatusBadRequest)
		return
	}
	k := atoiDefault(r.URL.Query().Get("k"), 8)
	if k > 20 {
		k = 20
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	hits := []map[string]any{}
	if brain.Available() {
		if db, err := brain.Open(); err == nil {

			min := searchMinCosine()
			snips, _ := brain.SemanticRetrieveScored(r.Context(), db, query, brain.RetrieveOpts{Limit: k}, min)
			for _, sn := range snips {
				hits = append(hits, map[string]any{
					"wing": sn.Wing, "room": sn.Room, "content": sn.Content,
					"score": sn.Score, "drawer_id": sn.DrawerID,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "hits": hits, "count": len(hits), "min_score": searchMinCosine()})
}

func searchMinCosine() float64 {
	const def = 0.45
	if s := strings.TrimSpace(os.Getenv("FLOWORK_SEARCH_MINSCORE")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 {
			return f
		}
	}
	return def
}

func brainByTypeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "drawers": []any{}})
		return
	}
	memType := r.URL.Query().Get("type")
	if memType == "" {
		memType = "project"
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)
	drawers, err := brain.ListByType(r.Context(), memType, limit, 400)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "type": memType, "drawers": drawers})
}

func brainPersonasHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"available": false, "personas": []any{}})
			return
		}
		http.Error(w, "brain DB not available", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		personas, err := brain.ListPersonas(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "personas": personas})
	case http.MethodPost:
		var b struct {
			Name    string `json:"name"`
			Content string `json:"content"`
			Source  string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := brain.AddPersona(r.Context(), b.Name, b.Content, b.Source); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": b.Name})
	case http.MethodPut:
		var b struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := brain.UpdatePersona(r.Context(), b.Name, b.Content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": b.Name})
	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := brain.DeletePersona(r.Context(), name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func applyBrainPath(s *store.Settings) {
	if s != nil && s.Brain.DBPath != "" {
		brain.SetDBPath(s.Brain.DBPath)
	}
}

func brainStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, err := store.Open()
	if err != nil {
		http.Error(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	writeJSON(w, http.StatusOK, brain.GetStats(r.Context()))
}

func brainConfigHandler(w http.ResponseWriter, r *http.Request) {
	d, err := store.Open()
	if err != nil {
		http.Error(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s, err := store.LoadSettings(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, s.Brain)
	case http.MethodPut, http.MethodPatch:
		var cfg store.BrainConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		s, err := store.LoadSettings(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.Brain = cfg
		if err := store.SaveSettings(d, s); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		applyBrainPath(s)
		writeJSON(w, http.StatusOK, s.Brain)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func brainExploreHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	writeJSON(w, http.StatusOK, brain.Explore(r.Context()))
}

func brainConstitutionHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "entries": []any{}})
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := atoiDefault(r.URL.Query().Get("limit"), 100)
		entries, err := brain.ListConstitution(r.Context(), limit, 1200)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "entries": entries})
	case http.MethodPost:
		var b struct {
			Section   string  `json:"section"`
			Content   string  `json:"content"`
			Amplitude float64 `json:"amplitude"`
			Source    string  `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		id, err := brain.AddConstitution(r.Context(), b.Section, b.Content, b.Amplitude, b.Source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	case http.MethodPut:
		var b struct {
			ID        int64   `json:"id"`
			Content   string  `json:"content"`
			Amplitude float64 `json:"amplitude"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := brain.UpdateConstitution(r.Context(), b.ID, b.Content, b.Amplitude); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		id := int64(atoiDefault(r.URL.Query().Get("id"), 0))
		if id == 0 {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := brain.SoftDeleteConstitution(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func brainContributionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, err := store.Open()
	if err != nil {
		http.Error(w, "store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	pending := r.URL.Query().Get("pending") == "1"
	limit := atoiDefault(r.URL.Query().Get("limit"), 200)
	list, _ := store.ListBrainContributions(d, pending, limit)
	total, pend := store.CountBrainContributions(d)
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "pending": pend, "contributions": list,
	})
}

func brainContributionsIngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		MaxID int64 `json:"maxId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	n, err := store.MarkContributionsIngested(d, body.MaxID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"marked": n})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func brainTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Query string   `json:"query"`
		Wings []string `json:"wings,omitempty"`
		TopK  int      `json:"topK,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false, "path": brain.DBPath(),
			"snippets": []any{}, "skills": []any{},
		})
		return
	}
	db, err := brain.Open()
	if err != nil {
		http.Error(w, "brain open: "+err.Error(), http.StatusInternalServerError)
		return
	}
	topK := body.TopK
	if topK <= 0 {
		topK = 5
	}

	snips, _ := brain.SemanticRetrieve(r.Context(), db, body.Query, brain.RetrieveOpts{
		Limit: topK, Wings: body.Wings, MaxContentLen: 400,
	})
	skills := brain.SelectSkills(body.Query, 3)
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"query":     body.Query,
		"snippets":  snips,
		"skills":    skills,
	})
}

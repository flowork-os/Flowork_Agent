// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Brain (wing / Knowledge Graph) → dok lock/gui/Brain.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func brainWingHandler(w http.ResponseWriter, r *http.Request) {
	wing := r.URL.Query().Get("wing")

	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)
	if !brain.Available() {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "drawers": []any{}})
		return
	}

	if wing == "cognitive_graph_node" {
		db, err := brain.OpenRW()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var node struct {
				ID         string `json:"id"`
				Label      string `json:"label"`
				Type       string `json:"type"`
				Properties string `json:"properties"`
			}
			if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if node.ID == "" || node.Label == "" || node.Type == "" {
				http.Error(w, "missing required fields", http.StatusBadRequest)
				return
			}
			if node.Properties == "" {
				node.Properties = "{}"
			}
			_, err = db.ExecContext(r.Context(), `
				INSERT INTO cognitive_nodes (id, label, type, properties, source)
				VALUES (?, ?, ?, ?, 'manual_ui')
				ON CONFLICT(id) DO UPDATE SET
					label=excluded.label,
					type=excluded.type,
					properties=excluded.properties,
					last_accessed=CURRENT_TIMESTAMP`,
				node.ID, node.Label, node.Type, node.Properties)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = brain.SyncGraphToRAG(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			_, err = db.ExecContext(r.Context(), "DELETE FROM cognitive_nodes WHERE id = ?", id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = brain.SyncGraphToRAG(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	if wing == "cognitive_graph_edge" {
		db, err := brain.OpenRW()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var edge struct {
				FromID       string  `json:"from_id"`
				ToID         string  `json:"to_id"`
				RelationType string  `json:"relation_type"`
				Strength     float64 `json:"strength"`
			}
			if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if edge.FromID == "" || edge.ToID == "" || edge.RelationType == "" {
				http.Error(w, "missing required fields", http.StatusBadRequest)
				return
			}
			if edge.Strength <= 0 {
				edge.Strength = 1.0
			}
			_, _ = db.ExecContext(r.Context(), "INSERT OR IGNORE INTO cognitive_nodes (id, label, type) VALUES (?, ?, 'concept')", edge.FromID, edge.FromID)
			_, _ = db.ExecContext(r.Context(), "INSERT OR IGNORE INTO cognitive_nodes (id, label, type) VALUES (?, ?, 'concept')", edge.ToID, edge.ToID)

			_, err = db.ExecContext(r.Context(), `
				INSERT INTO cognitive_edges (from_id, to_id, relation_type, strength)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(from_id, to_id, relation_type) DO UPDATE SET
					strength=excluded.strength`,
				edge.FromID, edge.ToID, edge.RelationType, edge.Strength)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = brain.SyncGraphToRAG(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		case http.MethodDelete:
			fromID := r.URL.Query().Get("from_id")
			toID := r.URL.Query().Get("to_id")
			relType := r.URL.Query().Get("relation_type")
			if fromID == "" || toID == "" || relType == "" {
				http.Error(w, "from_id, to_id, and relation_type required", http.StatusBadRequest)
				return
			}
			_, err = db.ExecContext(r.Context(), "DELETE FROM cognitive_edges WHERE from_id = ? AND to_id = ? AND relation_type = ?", fromID, toID, relType)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = brain.SyncGraphToRAG(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if wing == "" {
		http.Error(w, "wing required", http.StatusBadRequest)
		return
	}
	if wing == "cognitive_graph_all" {
		db, err := brain.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		nRows, err := db.QueryContext(r.Context(), "SELECT id, label, type, properties FROM cognitive_nodes")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type jsonNode struct {
			ID         string `json:"id"`
			Label      string `json:"label"`
			Type       string `json:"type"`
			Properties string `json:"properties"`
		}
		var nodes []jsonNode
		for nRows.Next() {
			var n jsonNode
			if err := nRows.Scan(&n.ID, &n.Label, &n.Type, &n.Properties); err != nil {
				nRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			nodes = append(nodes, n)
		}
		nRows.Close()

		eRows, err := db.QueryContext(r.Context(), "SELECT from_id, to_id, relation_type, strength FROM cognitive_edges")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type jsonEdge struct {
			Source       string  `json:"source"`
			Target       string  `json:"target"`
			RelationType string  `json:"relation_type"`
			Strength     float64 `json:"strength"`
		}
		var edges []jsonEdge
		for eRows.Next() {
			var e jsonEdge
			if err := eRows.Scan(&e.Source, &e.Target, &e.RelationType, &e.Strength); err != nil {
				eRows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			edges = append(edges, e)
		}
		eRows.Close()

		writeJSON(w, http.StatusOK, map[string]any{
			"nodes": nodes,
			"edges": edges,
		})
		return
	}
	if wing == "cognitive_graph_stats" {
		db, err := brain.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var nodes, edges int64
		_ = db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM cognitive_nodes").Scan(&nodes)
		_ = db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM cognitive_edges").Scan(&edges)
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"nodes":     nodes,
			"edges":     edges,
		})
		return
	}
	if wing == "cognitive_graph_dream" {
		num, err := brain.RunDreamCycle(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"digested":  num,
		})
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 100)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)
	roomLike := r.URL.Query().Get("room_like")
	drawers, err := brain.ListByWing(r.Context(), wing, roomLike, limit, offset, 1200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true, "wing": wing, "count": len(drawers), "offset": offset, "drawers": drawers,
	})
}

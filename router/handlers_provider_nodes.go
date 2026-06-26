// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Providers → dok lock/gui/Providers.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type ProviderNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	APIKey    string `json:"apiKey,omitempty"`
	Format    string `json:"format,omitempty"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updatedAt"`
}

const provNodeKVPrefix = "provider-node:"

func providerNodesRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/provider-nodes")
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case rest == "":
		providerNodesListUpsert(w, r)
	case rest == "validate":
		providerNodeValidate(w, r)
	default:
		providerNodeCRUD(w, r, rest)
	}
}

func providerNodesListUpsert(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		rows, err := d.Query(`SELECT v FROM kv WHERE k LIKE ? ORDER BY k`, provNodeKVPrefix+"%")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var out []ProviderNode
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				var n ProviderNode
				if json.Unmarshal([]byte(v), &n) == nil {
					out = append(out, n)
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "count": len(out)})
	case http.MethodPost:
		var n ProviderNode
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if n.URL == "" {
			http.Error(w, "url required", http.StatusBadRequest)
			return
		}
		if n.ID == "" {
			n.ID = "node-" + strings.ReplaceAll(strings.ToLower(n.Name), " ", "-")
			if n.ID == "node-" {
				n.ID = "node-" + time.Now().Format("150405")
			}
		}
		if n.Format == "" {
			n.Format = "openai"
		}
		n.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		saveProviderNode(d, &n)
		writeJSON(w, http.StatusCreated, n)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func providerNodeCRUD(w http.ResponseWriter, r *http.Request, id string) {
	d, _ := store.Open()
	key := provNodeKVPrefix + id
	switch r.Method {
	case http.MethodGet:
		var v string
		if d.QueryRow(`SELECT v FROM kv WHERE k=?`, key).Scan(&v) != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var n ProviderNode
		_ = json.Unmarshal([]byte(v), &n)
		writeJSON(w, http.StatusOK, n)
	case http.MethodPut:
		var n ProviderNode
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		n.ID = id
		n.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		saveProviderNode(d, &n)
		writeJSON(w, http.StatusOK, n)
	case http.MethodDelete:
		_, _ = d.Exec(`DELETE FROM kv WHERE k=?`, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func providerNodeValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL    string `json:"url"`
		APIKey string `json:"apiKey"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	valid, status, detail := probeProvider(r.Context(), body.URL, body.APIKey, body.Format)
	writeJSON(w, http.StatusOK, map[string]any{"valid": valid, "statusCode": status, "detail": detail})
}

func saveProviderNode(d *sql.DB, n *ProviderNode) {
	b, _ := json.Marshal(n)
	_, _ = d.Exec(`INSERT INTO kv (k,v,updatedAt) VALUES (?,?,datetime('now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		provNodeKVPrefix+n.ID, string(b))
}

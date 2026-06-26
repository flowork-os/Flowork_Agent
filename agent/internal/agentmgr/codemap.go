// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/codemap"
	"flowork-gui/internal/httpx"
)

func CodemapIndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(agentID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid or missing agent id"})
		return
	}
	var body struct {
		FilePath string `json:"file_path"`
		Layer    string `json:"layer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	if body.FilePath == "" {
		httpx.WriteJSON(w, map[string]any{"error": "file_path required"})
		return
	}
	if !strings.HasSuffix(body.FilePath, ".go") {
		httpx.WriteJSON(w, map[string]any{"error": "phase 1 only .go files"})
		return
	}
	sharedRoot := filepath.Join(agentFolder(agentID), "workspace")
	target := filepath.Join(sharedRoot, body.FilePath)
	if rel, rerr := filepath.Rel(sharedRoot, target); rerr != nil || strings.HasPrefix(rel, "..") {
		httpx.WriteJSON(w, map[string]any{"error": "file_path escapes workspace"})
		return
	}
	content, err := os.ReadFile(target)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	nodes, err := codemap.ParseGo(target, content)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "parse: " + err.Error()})
		return
	}

	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	relPath := relPathTo(sharedRoot, target)
	_ = store.DeleteCodemapNodesByFile(relPath)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, n := range nodes {
		_, _ = store.UpsertCodemapNode(agentdb.CodemapNode{
			NodeType:     n.Type,
			Name:         n.Name,
			FilePath:     relPath,
			LineStart:    n.LineStart,
			LineEnd:      n.LineEnd,
			Layer:        body.Layer,
			Signature:    n.Signature,
			SizeLOC:      n.SizeLOC,
			LastModified: now,
			IndexedAt:    now,
		})
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":          true,
		"file_path":   relPath,
		"nodes_count": len(nodes),
	})
}

func CodemapNodesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListCodemapNodes(
		r.URL.Query().Get("node_type"),
		r.URL.Query().Get("layer"),
		r.URL.Query().Get("search"),
		parseLimitOr(r.URL.Query().Get("limit"), 100),
	)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

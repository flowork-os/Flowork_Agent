// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 27 phase 1 endpoints. POST /index single .go file
//   only (phase 2 walk dir + JS + edges). GET /nodes filter.
//
// codemap.go — Section 27 phase 1 endpoints.

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

// CodemapIndexHandler — POST /api/agents/codemap/index?id=<agent>
// Body {file_path, layer}. file_path relative ke shared workspace.
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
	// Clear existing rows for this file.
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

// CodemapNodesHandler — GET /api/agents/codemap/nodes?id=&node_type=&layer=&search=&limit=
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

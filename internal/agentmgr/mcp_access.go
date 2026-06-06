// mcp_access.go — per-agent MCP access (opt-OUT). MCP tools are on by default for
// every agent (they live in the engine tool registry, found via tool_search). An
// agent can UNCHECK MCP connectors it doesn't need; the excluded set is kept in the
// agent's OWN folder (mcp_excluded.json — isolated, no shared table) and the
// tool_search results are filtered for that agent. Owner's rule: flexible per user.
package agentmgr

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/mcphub"
)

const mcpExcludedFile = "mcp_excluded.json"

// mcpExcludedSet reads the connector ids an agent has unchecked.
func mcpExcludedSet(agentID string) map[string]bool {
	out := map[string]bool{}
	if !reID.MatchString(agentID) {
		return out
	}
	raw, err := os.ReadFile(filepath.Join(agentFolder(agentID), mcpExcludedFile))
	if err != nil {
		return out
	}
	var ids []string
	_ = json.Unmarshal(raw, &ids)
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// setMCPExcluded persists the agent's unchecked connector ids in its own folder.
func setMCPExcluded(agentID string, ids []string) error {
	blob, _ := json.Marshal(ids)
	return os.WriteFile(filepath.Join(agentFolder(agentID), mcpExcludedFile), blob, 0o644)
}

// hiddenMCPToolNames returns the registry tool names hidden for this agent (the
// tools belonging to the connectors it unchecked). Used to filter tool_search.
func hiddenMCPToolNames(agentID string) map[string]bool {
	excl := mcpExcludedSet(agentID)
	if len(excl) == 0 {
		return nil
	}
	hidden := map[string]bool{}
	for connID := range excl {
		for _, t := range mcphub.Default.ToolsFor(connID) {
			hidden[t] = true
		}
	}
	return hidden
}

// AgentMCPHandler serves the per-agent MCP checklist:
//
//	GET  /api/agents/mcp?id=<agent>            → {connectors:[{id, enabled}]}
//	POST /api/agents/mcp?id=<agent> {excluded} → persist the unchecked set
//
// "enabled" here = checked for THIS agent (not in its excluded set).
func AgentMCPHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	if !reID.MatchString(agentID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid agent id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		excl := mcpExcludedSet(agentID)
		type row struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
			Tools   int    `json:"tools"`
		}
		rows := []row{}
		for _, c := range mcphub.Default.List() {
			rows = append(rows, row{ID: c.ID, Enabled: !excl[c.ID], Tools: len(c.Tools)})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		httpx.WriteJSON(w, map[string]any{"connectors": rows})
	case http.MethodPost:
		var body struct {
			Excluded []string `json:"excluded"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "decode: " + err.Error()})
			return
		}
		if err := setMCPExcluded(agentID, body.Excluded); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "excluded": body.Excluded})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "GET or POST"})
	}
}

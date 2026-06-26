// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: CLI Tools → dok lock/gui/CLI Tools.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/clitools"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func cliToolsRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/cli-tools")
	rest = strings.TrimPrefix(rest, "/")
	switch {
	case rest == "":
		cliToolsListHandler(w, r)
	case rest == "all-statuses":
		cliToolsListHandler(w, r)
	case rest == "cowork-mcp-registry":
		coworkMCPRegistryHandler(w, r)
	case rest == "cowork-mcp-tools":
		coworkMCPToolsHandler(w, r)
	case rest == "antigravity-mitm/alias":
		antigravityAliasHandler(w, r)
	case strings.HasSuffix(rest, "-settings") || rest == "antigravity-mitm":
		toolID := strings.TrimSuffix(rest, "-settings")
		cliToolSettingsHandler(w, r, toolID)
	default:
		http.Error(w, "unknown cli-tools sub-route: "+rest, http.StatusNotFound)
	}
}

func cliToolsListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	statuses := clitools.DetectAll()

	d, _ := store.Open()
	for _, s := range statuses {
		_ = store.UpsertCLIToolState(d, &store.CLIToolState{
			ToolID:          s.ID,
			Installed:       s.Installed,
			HasCredentials:  s.SettingsExists,
			BinaryPath:      s.BinaryPath,
			CredentialsPath: s.SettingsPath,
			Status: func() string {
				if s.ErrorMessage != "" {
					return "error"
				}
				if s.HasFlowRouter {
					return "connected"
				}
				if s.Installed {
					return "installed"
				}
				return "missing"
			}(),
			Notes: s.ErrorMessage,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  statuses,
		"count": len(statuses),
	})
}

func cliToolSettingsHandler(w http.ResponseWriter, r *http.Request, toolID string) {
	switch r.Method {
	case http.MethodGet:
		st, err := clitools.Detect(toolID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, st)
	case http.MethodPost:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		env := body

		if v, ok := body["env"]; ok {
			if mm, ok := v.(map[string]any); ok {
				env = mm
			}
		} else if _, hasBase := body["baseUrl"]; hasBase {

			str := func(k string) string { s, _ := body[k].(string); return s }
			env = clitools.BuildConnectEnv(toolID, str("baseUrl"), str("apiKey"), str("model"))
		}
		updated, err := clitools.WriteEnv(toolID, env)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"toolId":   toolID,
			"settings": updated,
		})
	case http.MethodDelete:
		if err := clitools.ResetEnv(toolID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"toolId":  toolID,
			"message": "settings env keys reset",
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func antigravityAliasHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	const key = "antigravity:alias"
	switch r.Method {
	case http.MethodGet:
		var v string
		_ = d.QueryRow(`SELECT v FROM kv WHERE k=?`, key).Scan(&v)
		if v == "" {
			v = "{}"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(v))
	case http.MethodPost, http.MethodPut:
		body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if len(body) == 0 {
			body = []byte("{}")
		}
		_, _ = d.Exec(`INSERT INTO kv (k,v,updatedAt) VALUES (?,?,datetime('now'))
			ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`, key, string(body))
		writeJSON(w, http.StatusOK, map[string]any{"saved": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func coworkMCPRegistryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	registry := []map[string]any{
		{"id": "playwright", "name": "Playwright MCP", "description": "Browser automation via Playwright", "transport": "stdio", "command": "npx", "args": []string{"@playwright/mcp"}},
		{"id": "filesystem", "name": "Filesystem MCP", "description": "Read/write local filesystem (sandboxed)", "transport": "stdio", "command": "npx", "args": []string{"@modelcontextprotocol/server-filesystem"}},
		{"id": "github", "name": "GitHub MCP", "description": "GitHub repo/issue/PR ops", "transport": "stdio", "command": "npx", "args": []string{"@modelcontextprotocol/server-github"}},
		{"id": "sqlite", "name": "SQLite MCP", "description": "Query SQLite DBs", "transport": "stdio", "command": "npx", "args": []string{"@modelcontextprotocol/server-sqlite"}},
		{"id": "memory", "name": "Memory MCP", "description": "Long-term memory key-value", "transport": "stdio", "command": "npx", "args": []string{"@modelcontextprotocol/server-memory"}},
		{"id": "fetch", "name": "Fetch MCP", "description": "HTTP fetch tool", "transport": "stdio", "command": "npx", "args": []string{"@modelcontextprotocol/server-fetch"}},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  registry,
		"count": len(registry),
	})
}

func coworkMCPToolsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	servers, _ := store.ListMCPServers(d)
	var tools []map[string]any
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		srvTools, err := mcpListToolsLive(r.Context(), &s)
		if err != nil {
			tools = append(tools, map[string]any{
				"server": s.ID, "name": s.Name, "transport": s.Transport, "error": err.Error(),
			})
			continue
		}
		for _, t := range srvTools {
			t["server"] = s.ID
			tools = append(tools, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  tools,
		"count": len(tools),
	})
}

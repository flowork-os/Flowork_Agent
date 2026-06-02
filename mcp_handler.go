// mcp_handler.go — FASE 7: endpoint config MCP buat GUI (copy-paste ke AI
// eksternal: VS Code/Cursor/Claude Desktop). Path binary ke-resolve otomatis
// biar owner ga usah ngetik manual.
//
//	GET /api/mcp/config → {config, binary_path, binary_exists, build_cmd, self_url}

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"flowork-gui/internal/agentdb"
)

func mcpConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	root := agentdb.ProjectRoot()
	bin := filepath.Join(root, "bin", "flowork-mcp")
	exists := false
	if st, err := os.Stat(bin); err == nil && !st.IsDir() {
		exists = true
	}
	selfURL := "http://127.0.0.1:1987"
	if v := os.Getenv("FLOWORK_SELF_URL"); v != "" {
		selfURL = v
	}

	// Config mcpServers (format umum: Claude Desktop/Code, Cursor, VS Code MCP).
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"flowork": map[string]any{
				"command": bin,
				"env":     map[string]any{"FLOWORK_SELF_URL": selfURL},
			},
		},
	}
	pretty, _ := json.MarshalIndent(cfg, "", "  ")

	_ = json.NewEncoder(w).Encode(map[string]any{
		"config":        string(pretty),
		"binary_path":   bin,
		"binary_exists": exists,
		"build_cmd":     "go build -o bin/flowork-mcp ./cmd/flowork-mcp",
		"self_url":      selfURL,
	})
}

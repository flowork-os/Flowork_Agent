// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: MCP Servers → dok lock/gui/MCP Servers.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/mcpcatalog"
)

func mcpCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	plugins := mcpcatalog.Catalog()
	writeJSON(w, http.StatusOK, map[string]any{
		"plugins": plugins,
		"count":   len(plugins),
	})
}

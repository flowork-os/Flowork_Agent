// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package handlers

import "net/http"

func init() { Register(&copilotHandler{}) }

type copilotHandler struct{}

func (c *copilotHandler) Name() string { return "copilot" }

func (c *copilotHandler) Handle(w http.ResponseWriter, r *http.Request) {

	r.Header.Del("editor-version")
	r.Header.Del("editor-plugin-version")
	r.Header.Del("copilot-integration-id")
	rerouteToRouter(w, r, r.URL.Path)
}

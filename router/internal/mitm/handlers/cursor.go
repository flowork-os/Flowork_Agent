// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package handlers

import "net/http"

func init() { Register(&cursorHandler{}) }

type cursorHandler struct{}

func (c *cursorHandler) Name() string { return "cursor" }

func (c *cursorHandler) Handle(w http.ResponseWriter, r *http.Request) {
	r.Header.Del("x-cursor-checksum")
	r.Header.Del("x-cursor-session-id")
	r.Header.Del("x-ghost-mode")
	rerouteToRouter(w, r, "/v1/chat/completions")
}

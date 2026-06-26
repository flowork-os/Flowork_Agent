// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "cursor", To: "openai"}, translator.DirResponse, CursorToOpenAI)
}

func CursorToOpenAI(body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		if k == "cursor_metadata" || k == "analytics" {
			continue
		}
		out[k] = v
	}
	return out
}

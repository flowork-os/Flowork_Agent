// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package request

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "antigravity", To: "openai"}, translator.DirRequest, AntigravityToOpenAI)
}

func AntigravityToOpenAI(body map[string]any) map[string]any {
	inner, ok := body["request"].(map[string]any)
	if !ok {

		return GeminiToOpenAI(body)
	}
	out := GeminiToOpenAI(inner)
	if m, ok := body["model"].(string); ok && m != "" {
		out["model"] = m
	}
	return out
}

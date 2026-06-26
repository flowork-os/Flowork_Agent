// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package request

import (
	"github.com/flowork-os/flowork_Router/internal/translator"
	"github.com/flowork-os/flowork_Router/internal/translator/helpers"
)

func init() {
	translator.Register(translator.Pair{From: "claude", To: "openai"}, translator.DirRequest, ClaudeToOpenAI)
}

func ClaudeToOpenAI(body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		if k == "system" || k == "messages" {
			continue
		}
		out[k] = v
	}
	msgs := []map[string]any{}
	if sys := helpers.FlattenAnthropicSystem(body["system"]); sys != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": sys})
	}
	if arr, ok := body["messages"].([]any); ok {
		for _, raw := range arr {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			role, _ := m["role"].(string)
			content := helpers.FlattenAnthropicContent(m["content"])
			msgs = append(msgs, map[string]any{"role": role, "content": content})
		}
	}
	out["messages"] = msgs
	return out
}

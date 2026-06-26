// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package request

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "openai", To: "commandcode"}, translator.DirRequest, OpenAIToCommandCode)
}

func OpenAIToCommandCode(body map[string]any) map[string]any {
	out := map[string]any{
		"model": body["model"],
	}
	var prompt string
	var history []map[string]any
	if msgs, ok := body["messages"].([]any); ok {
		for i, raw := range msgs {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			if i == len(msgs)-1 && role == "user" {
				prompt = content
				continue
			}
			history = append(history, map[string]any{"role": role, "content": content})
		}
	}
	out["prompt"] = prompt
	out["history"] = history
	return out
}

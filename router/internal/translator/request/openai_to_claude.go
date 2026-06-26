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
	translator.Register(translator.Pair{From: "openai", To: "claude"}, translator.DirRequest, OpenAIToClaude)
}

func OpenAIToClaude(body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		if k == "messages" {
			continue
		}
		out[k] = v
	}
	if mt, ok := body["max_tokens"]; ok {
		out["max_tokens"] = mt
	} else if m, _ := body["model"].(string); m != "" {
		out["max_tokens"] = helpers.MaxTokensForModel(m)
	}

	systemParts := []string{}
	anthrMsgs := []map[string]any{}
	if msgs, ok := body["messages"].([]any); ok {
		for _, raw := range msgs {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			switch role {
			case "system":
				if content != "" {
					systemParts = append(systemParts, content)
				}
			case "user", "assistant":
				anthrMsgs = append(anthrMsgs, map[string]any{"role": role, "content": content})
			case "tool":

				anthrMsgs = append(anthrMsgs, map[string]any{
					"role": "user",
					"content": []map[string]any{{
						"type":        "tool_result",
						"tool_use_id": m["tool_call_id"],
						"content":     content,
					}},
				})
			}
		}
	}
	if len(systemParts) > 0 {
		out["system"] = joinStr(systemParts, "\n\n")
	}
	out["messages"] = anthrMsgs
	return out
}

func joinStr(parts []string, sep string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += sep
		}
		s += p
	}
	return s
}

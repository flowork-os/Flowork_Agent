// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package request

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "gemini", To: "openai"}, translator.DirRequest, GeminiToOpenAI)
}

func GeminiToOpenAI(body map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range body {
		if k == "contents" || k == "systemInstruction" {
			continue
		}
		out[k] = v
	}
	msgs := []map[string]any{}
	if sys, ok := body["systemInstruction"].(map[string]any); ok {
		if txt := joinParts(sys["parts"]); txt != "" {
			msgs = append(msgs, map[string]any{"role": "system", "content": txt})
		}
	}
	if contents, ok := body["contents"].([]any); ok {
		for _, raw := range contents {
			c, _ := raw.(map[string]any)
			if c == nil {
				continue
			}
			role, _ := c["role"].(string)
			if role == "model" {
				role = "assistant"
			}
			msgs = append(msgs, map[string]any{"role": role, "content": joinParts(c["parts"])})
		}
	}
	out["messages"] = msgs
	return out
}

func joinParts(parts any) string {
	arr, ok := parts.([]any)
	if !ok {
		return ""
	}
	out := ""
	for _, p := range arr {
		if m, ok := p.(map[string]any); ok {
			if t, _ := m["text"].(string); t != "" {
				out += t
			}
		}
	}
	return out
}
